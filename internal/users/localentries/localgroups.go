// Package localentries provides functions to retrieve passwd and group entries and to update the groups of a user.
package localentries

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/ubuntu/authd/internal/fileutils"
	"github.com/ubuntu/authd/internal/sliceutils"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// GetGroupEntries returns a copy of the current group entries.
func GetGroupEntries(ctx context.Context) (entries []types.GroupEntry, err error) {
	defer decorate.OnError(&err, "could not get groups")

	unlock := GetUserDBLocked(ctx).lockGroupFile()
	defer unlock()

	return getGroupEntriesWithContext(ctx)
}

func getGroupEntriesWithContext(ctx context.Context) (entries []types.GroupEntry, err error) {
	entries, err = GetUserDBLocked(ctx).GetLocalGroupEntries()
	if err != nil {
		return nil, err
	}

	return types.DeepCopyGroupEntries(entries), nil
}

// SaveGroupEntries saves the provided group entries to the local group file.
func SaveGroupEntries(ctx context.Context, entries []types.GroupEntry) (err error) {
	defer decorate.OnError(&err, "could not save groups")

	unlock := GetUserDBLocked(ctx).lockGroupFile()
	defer unlock()

	return saveLocalGroups(ctx, entries)
}

// UpdateGroups updates the local groups for a user, adding them to the groups in
// newGroups which they are not already part of, and removing them from the
// groups in oldGroups which are not in newGroups.
func UpdateGroups(ctx context.Context, username string, newGroups []string, oldGroups []string) (err error) {
	log.Debugf(ctx, "Updating local groups for user %q, new groups: %v, old groups: %v", username, newGroups, oldGroups)
	defer decorate.OnError(&err, "could not update local groups for user %q", username)

	unlock := GetUserDBLocked(ctx).lockGroupFile()
	defer unlock()

	if len(newGroups) == 0 && len(oldGroups) == 0 {
		return nil
	}

	allGroups, err := getGroupEntriesWithContext(ctx)
	if err != nil {
		return err
	}

	userGroups := userLocalGroups(allGroups, username)
	currentGroupsNames := sliceutils.Map(userGroups, func(g types.GroupEntry) string {
		return g.Name
	})

	groupsToAdd := sliceutils.Difference(newGroups, currentGroupsNames)
	log.Debugf(ctx, "Adding %q to local groups: %v", username, groupsToAdd)
	groupsToRemove := sliceutils.Difference(oldGroups, newGroups)
	// Only remove user from groups which they are part of
	groupsToRemove = sliceutils.Intersection(groupsToRemove, currentGroupsNames)
	log.Debugf(ctx, "Removing %q from local groups: %v", username, groupsToRemove)

	if len(groupsToRemove) == 0 && len(groupsToAdd) == 0 {
		return nil
	}

	getGroupByName := func(name string) *types.GroupEntry {
		idx := slices.IndexFunc(allGroups, func(g types.GroupEntry) bool { return g.Name == name })
		if idx == -1 {
			return nil
		}
		return &allGroups[idx]
	}

	for _, g := range groupsToRemove {
		group := getGroupByName(g)
		if group == nil {
			continue
		}
		group.Users = slices.DeleteFunc(group.Users, func(u string) bool {
			return u == username
		})
	}
	for _, g := range groupsToAdd {
		group := getGroupByName(g)
		if group == nil {
			continue
		}
		group.Users = append(group.Users, username)
	}

	return saveLocalGroups(ctx, allGroups)
}

func parseLocalGroups(groupPath string) (groups []types.GroupEntry, invalidEntries []invalidEntry, err error) {
	defer decorate.OnError(&err, "could not fetch existing local group")

	log.Debugf(context.Background(), "Reading groups from %q", groupPath)

	f, err := os.Open(groupPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	// Format of a line composing the group file is:
	// group_name:password:group_id:user1,…,usern
	scanner := bufio.NewScanner(f)
	for lineNum := 0; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			invalidEntries = append(invalidEntries,
				invalidEntry{lineNum: lineNum, line: line})
			continue
		}

		elems := strings.SplitN(line, ":", 4)
		if len(elems) < 4 {
			log.Warningf(context.Background(),
				"Malformed entry in group file (should have 4 separators, got %d): %q",
				len(elems), line)
			invalidEntries = append(invalidEntries,
				invalidEntry{lineNum: lineNum, line: line})
			continue
		}

		name, passwd, gidValue, usersValue := elems[0], elems[1], elems[2], elems[3]

		gid, err := strconv.ParseUint(gidValue, 10, 0)
		if err != nil || gid > math.MaxUint32 {
			log.Warningf(context.Background(),
				"Failed parsing entry %q, unexpected GID value: %v", line, err)
			invalidEntries = append(invalidEntries,
				invalidEntry{lineNum: lineNum, line: line})
			continue
		}

		var users []string
		if usersValue != "" {
			users = strings.Split(usersValue, ",")
		}

		groups = append(groups, types.GroupEntry{
			Name:   name,
			Passwd: passwd,
			GID:    uint32(gid),
			Users:  users,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	if err := types.ValidateGroupEntries(groups); err != nil {
		log.Warningf(context.Background(),
			"The group file %q contains at least one invalid entry: %v",
			groupPath, err)
	}

	return groups, invalidEntries, nil
}

func groupFileTemporaryPath(groupPath string) string {
	return fmt.Sprintf("%s+", groupPath)
}

func groupFileBackupPath(groupPath string) string {
	return fmt.Sprintf("%s-", groupPath)
}

func formatGroupEntries(ctx context.Context, groups []types.GroupEntry) string {
	groupLines := sliceutils.Map(groups, func(group types.GroupEntry) string {
		return group.String()
	})

	for _, entry := range GetUserDBLocked(ctx).localGroupInvalidEntries {
		groupLines = slices.Insert(groupLines,
			min(entry.lineNum, len(groupLines)-1), entry.line)
	}

	// Add final new line to the group file.
	groupLines = append(groupLines, "")

	return strings.Join(groupLines, "\n")
}

func saveLocalGroups(ctx context.Context, groups []types.GroupEntry) (err error) {
	lockedEntries := GetUserDBLocked(ctx)
	inputPath := lockedEntries.options.inputGroupPath
	groupPath := lockedEntries.options.outputGroupPath

	defer decorate.OnError(&err, "could not write local groups to %q", groupPath)

	currentGroups, err := getGroupEntriesWithContext(ctx)
	if err != nil {
		return err
	}

	if slices.EqualFunc(currentGroups, groups, types.GroupEntry.Equals) {
		log.Debugf(ctx, "Nothing to do, groups are equal")
		return nil
	}

	if err := validateChangedGroups(ctx, currentGroups, groups); err != nil {
		log.Debugf(ctx, "New groups are not valid: %v", err)
		return err
	}

	backupPath := groupFileBackupPath(groupPath)
	groupsEntries := formatGroupEntries(ctx, groups)

	log.Debugf(ctx, "Saving group entries %#v to %q", groups, groupPath)
	if len(groupsEntries) > 0 {
		log.Debugf(ctx, "Group file content:\n%s", groupsEntries)
	}

	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warningf(ctx, "Failed to remove group file backup: %v", err)
	}

	log.Debugf(ctx, "Backing up %q to %q", inputPath, backupPath)
	if err := fileutils.CopyFile(inputPath, backupPath); err != nil {
		log.Warningf(ctx, "Failed make a backup for the group file: %v", err)
	}

	tempPath := groupFileTemporaryPath(groupPath)
	//nolint:gosec // G306 /etc/group should indeed have 0644 permissions
	if err := os.WriteFile(tempPath, []byte(groupsEntries), 0644); err != nil {
		return fmt.Errorf("error writing %s: %w", tempPath, err)
	}

	if err := fileutils.Lrename(tempPath, groupPath); err != nil {
		return fmt.Errorf("error renaming %s to %s: %w", tempPath, groupPath, err)
	}

	lockedEntries.updateLocalGroupEntriesCache(groups)
	return nil
}

func validateChangedGroups(ctx context.Context, currentGroups, newGroups []types.GroupEntry) error {
	changedGroups := sliceutils.DifferenceFunc(newGroups, currentGroups,
		types.GroupEntry.Equals)
	if len(changedGroups) == 0 {
		log.Debugf(ctx, "No new groups added to validate")
		return nil
	}

	log.Debugf(ctx, "Groups added or modified: %#v", changedGroups)

	if err := types.ValidateGroupEntries(changedGroups); err != nil {
		// One of the group that has been changed is not valid.
		return fmt.Errorf("changed groups are not valid: %w", err)
	}

	if err := types.ValidateGroupEntries(newGroups); err == nil {
		// The groups we got are all good, no need to proceed further!
		return nil
	}

	validCurrentGroups := types.GetValidGroupEntries(currentGroups)

	// So, now we know that:
	//  1) the changed groups alone are good
	//  2) the whole set of the new groups are not good
	// So let's try to check if the changed groups are compatible with the
	// current valid groups that we have.
	validGroupsWithChanged := append(validCurrentGroups, changedGroups...)
	return types.ValidateGroupEntries(validGroupsWithChanged)
}

// userLocalGroups returns all groups the user is part of.
func userLocalGroups(entries []types.GroupEntry, user string) (userGroups []types.GroupEntry) {
	return slices.DeleteFunc(slices.Clone(entries), func(g types.GroupEntry) bool {
		return !slices.Contains(g.Users, user)
	})
}
