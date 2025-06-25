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
	"sync"

	"github.com/ubuntu/authd/internal/fileutils"
	"github.com/ubuntu/authd/internal/sliceutils"
	"github.com/ubuntu/authd/internal/testsdetection"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// GroupFile is the default local group file.
const GroupFile = "/etc/group"

var defaultOptions = options{
	groupInputPath:  GroupFile,
	groupOutputPath: GroupFile,
}

type options struct {
	groupInputPath  string
	groupOutputPath string
}

// Option represents an optional function to override UpdateLocalGroups default values.
type Option func(*options)

// GroupsWithLock is a struct that holds the current user groups and provides methods to
// retrieve and update them while ensuring that the system's user database is locked
// to prevent concurrent modifications.
type GroupsWithLock struct {
	// mu is a mutex that protects the refCount and entries fields.
	mu sync.RWMutex
	// refCount is used to track how many times the GroupsWithLock instance has been returned by GetGroupsWithLock.
	refCount uint64
	// entries holds the current group entries.
	entries []types.GroupEntry
	// options for testing purposes.
	options options
}

// defaultGroupsWithLock is used as the instance for locked groups when
// no test options are provided.
var defaultGroupsWithLock = &GroupsWithLock{}

// GetGroupsWithLock gets a GroupsWithLock instance with a lock on the system's user database.
// It returns a cleanup function that should be called to unlock system's user database.
func GetGroupsWithLock(args ...Option) (groups *GroupsWithLock, cleanup func() error, err error) {
	defer decorate.OnError(&err, "could not lock local groups")

	if err := userslocking.WriteRecLock(); err != nil {
		return nil, nil, err
	}

	cleanupUnlocked := func() error {
		if groups.refCount == 0 {
			return fmt.Errorf("groups were already unlocked")
		}

		groups.refCount--
		if groups.refCount == 0 {
			groups.entries = nil
		}
		return userslocking.WriteRecUnlock()
	}

	cleanup = func() error {
		groups.mu.Lock()
		defer groups.mu.Unlock()

		return cleanupUnlocked()
	}

	groups = defaultGroupsWithLock
	testingMode := len(args) != 0

	if testingMode {
		testsdetection.MustBeTesting()
		groups = &GroupsWithLock{}
	}

	groups.mu.Lock()
	defer groups.mu.Unlock()

	groups.refCount++
	if groups.refCount > 1 {
		return groups, cleanup, nil
	}

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	groups.options = opts
	groups.entries, err = parseLocalGroups(opts.groupInputPath)
	if err != nil {
		return nil, nil, errors.Join(err, cleanupUnlocked())
	}

	return groups, cleanup, nil
}

func (g *GroupsWithLock) mustLock() (cleanup func()) {
	g.mu.Lock()
	cleanup = g.mu.Unlock

	if g.refCount == 0 {
		defer cleanup()
		panic("locked groups are not locked!")
	}

	return cleanup
}

func (g *GroupsWithLock) mustRLock() (cleanup func()) {
	g.mu.RLock()
	cleanup = g.mu.RUnlock

	if g.refCount == 0 {
		defer cleanup()
		panic("locked groups are not locked!")
	}

	return cleanup
}

// GetEntries returns a copy of the current group entries.
func (g *GroupsWithLock) GetEntries() (entries []types.GroupEntry) {
	unlock := g.mustRLock()
	defer unlock()

	return types.DeepCopyGroupEntries(g.entries)
}

// SaveEntries saves the provided group entries to the local group file.
func (g *GroupsWithLock) SaveEntries(entries []types.GroupEntry) (err error) {
	defer decorate.OnError(&err, "could not save groups")

	unlock := g.mustLock()
	defer unlock()

	return g.saveLocalGroups(entries)
}

// Update updates the local groups for a user, adding them to the groups in
// newGroups which they are not already part of, and removing them from the
// groups in oldGroups which are not in newGroups.
func (g *GroupsWithLock) Update(username string, newGroups []string, oldGroups []string) (err error) {
	log.Debugf(context.TODO(), "Updating local groups for user %q, new groups: %v, old groups: %v", username, newGroups, oldGroups)
	defer decorate.OnError(&err, "could not update local groups for user %q", username)

	unlock := g.mustLock()
	defer unlock()

	allGroups := types.DeepCopyGroupEntries(g.entries)
	userGroups := g.userLocalGroups(username)
	currentGroupsNames := sliceutils.Map(userGroups, func(g types.GroupEntry) string {
		return g.Name
	})

	groupsToAdd := sliceutils.Difference(newGroups, currentGroupsNames)
	log.Debugf(context.TODO(), "Adding %q to local groups: %v", username, groupsToAdd)
	groupsToRemove := sliceutils.Difference(oldGroups, newGroups)
	// Only remove user from groups which they are part of
	groupsToRemove = sliceutils.Intersection(groupsToRemove, currentGroupsNames)
	log.Debugf(context.TODO(), "Removing %q from local groups: %v", username, groupsToRemove)

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

	return g.saveLocalGroups(allGroups)
}

func parseLocalGroups(groupPath string) (groups []types.GroupEntry, err error) {
	defer decorate.OnError(&err, "could not fetch existing local group")

	log.Debugf(context.Background(), "Reading groups from %q", groupPath)

	f, err := os.Open(groupPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Format of a line composing the group file is:
	// group_name:password:group_id:user1,…,usern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if t == "" {
			continue
		}
		elems := strings.Split(t, ":")
		if len(elems) != 4 {
			return nil, fmt.Errorf("malformed entry in group file (should have 4 separators, got %d): %q", len(elems), t)
		}

		name, passwd, gidValue, usersValue := elems[0], elems[1], elems[2], elems[3]

		gid, err := strconv.ParseUint(gidValue, 10, 0)
		if err != nil || gid > math.MaxUint32 {
			return nil, fmt.Errorf("failed parsing entry %q, unexpected GID value", t)
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
		return nil, err
	}

	if err := types.ValidateGroupEntries(groups); err != nil {
		return nil, err
	}

	return groups, nil
}

func groupFileTemporaryPath(groupPath string) string {
	return fmt.Sprintf("%s+", groupPath)
}

func groupFileBackupPath(groupPath string) string {
	return fmt.Sprintf("%s-", groupPath)
}

func formatGroupEntries(groups []types.GroupEntry) string {
	groupLines := sliceutils.Map(groups, func(group types.GroupEntry) string {
		return group.String()
	})

	// Add final new line to the group file.
	groupLines = append(groupLines, "")

	return strings.Join(groupLines, "\n")
}

func (g *GroupsWithLock) saveLocalGroups(groups []types.GroupEntry) (err error) {
	inputPath := g.options.groupInputPath
	groupPath := g.options.groupOutputPath

	defer decorate.OnError(&err, "could not write local groups to %q", groupPath)

	if slices.EqualFunc(g.entries, groups, types.GroupEntry.Equals) {
		log.Debugf(context.TODO(), "Nothing to do, groups are equal")
		return nil
	}

	if err := types.ValidateGroupEntries(groups); err != nil {
		return err
	}

	backupPath := groupFileBackupPath(groupPath)
	groupsEntries := formatGroupEntries(groups)

	log.Debugf(context.TODO(), "Saving group entries %#v to %q", groups, groupPath)
	if len(groupsEntries) > 0 {
		log.Debugf(context.TODO(), "Group file content:\n%s", groupsEntries)
	}

	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warningf(context.Background(), "Failed to remove group file backup: %v", err)
	}

	log.Debugf(context.Background(), "Backing up %q to %q", inputPath, backupPath)
	if err := fileutils.CopyFile(inputPath, backupPath); err != nil {
		log.Warningf(context.Background(), "Failed make a backup for the group file: %v", err)
	}

	tempPath := groupFileTemporaryPath(groupPath)
	//nolint:gosec // G306 /etc/group should indeed have 0644 permissions
	if err := os.WriteFile(tempPath, []byte(groupsEntries), 0644); err != nil {
		return fmt.Errorf("error writing %s: %w", tempPath, err)
	}

	if err := fileutils.Lrename(tempPath, groupPath); err != nil {
		return fmt.Errorf("error renaming %s to %s: %w", tempPath, groupPath, err)
	}

	g.entries = types.DeepCopyGroupEntries(groups)
	return nil
}

// userLocalGroups returns all groups the user is part of.
func (g *GroupsWithLock) userLocalGroups(user string) (userGroups []types.GroupEntry) {
	return slices.DeleteFunc(slices.Clone(g.entries), func(g types.GroupEntry) bool {
		return !slices.Contains(g.Users, user)
	})
}
