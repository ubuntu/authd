// Package localentries provides functions to retrieve passwd and group entries and to update the groups of a user.
package localentries

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ubuntu/authd/internal/sliceutils"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// GroupFile is the default local group file.
const GroupFile = "/etc/group"

var defaultOptions = options{
	groupPath:  GroupFile,
	gpasswdCmd: []string{"gpasswd"},
}

type options struct {
	groupPath  string
	gpasswdCmd []string
}

// Option represents an optional function to override UpdateLocalGroups default values.
type Option func(*options)

var localGroupsMu = &sync.RWMutex{}

// Update synchronizes for the given user the local group list with the current group list from UserInfo.
func Update(username string, newGroups []string, oldGroups []string, args ...Option) (err error) {
	log.Debugf(context.TODO(), "Updating local groups for user %q, new groups: %v, old groups: %v", username, newGroups, oldGroups)
	defer decorate.OnError(&err, "could not update local groups for user %q", username)

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	currentGroups, err := existingLocalGroups(username, opts.groupPath)
	if err != nil {
		return err
	}

	currentGroupsNames := sliceutils.Map(currentGroups, func(g types.GroupEntry) string {
		return g.Name
	})

	localGroupsMu.Lock()
	defer localGroupsMu.Unlock()
	groupsToAdd := sliceutils.Difference(newGroups, currentGroupsNames)
	log.Debugf(context.TODO(), "Adding to local groups: %v", groupsToAdd)
	groupsToRemove := sliceutils.Difference(oldGroups, newGroups)
	// Only remove user from groups which they are part of
	groupsToRemove = sliceutils.Intersection(groupsToRemove, currentGroupsNames)
	log.Debugf(context.TODO(), "Removing from local groups: %v", groupsToRemove)

	// Do all this in a goroutine as we don't want to hang.
	for _, g := range groupsToRemove {
		args := opts.gpasswdCmd[1:]
		args = append(args, "--delete", username, g)
		if err := runGPasswd(opts.gpasswdCmd[0], args...); err != nil {
			return err
		}
	}
	for _, g := range groupsToAdd {
		args := opts.gpasswdCmd[1:]
		args = append(args, "--add", username, g)
		if err := runGPasswd(opts.gpasswdCmd[0], args...); err != nil {
			return err
		}
	}

	return nil
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

// existingLocalGroups returns which groups from groupPath the user is part of.
func existingLocalGroups(user, groupPath string) (groups []types.GroupEntry, err error) {
	defer decorate.OnError(&err, "could not fetch existing local group for user %q", user)

	groups, err = parseLocalGroups(groupPath)
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(groups, func(g types.GroupEntry) bool {
		return !slices.Contains(g.Users, user)
	}), nil
}

// runGPasswd is a wrapper to cmdName ignoring exit code 3, meaning that the group doesn't exist.
// Note: it’s the same return code for user not existing, but it’s something we are in control of as we
// are responsible for the user itself and parsing the output is not desired.
func runGPasswd(cmdName string, args ...string) error {
	cmd := exec.Command(cmdName, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState.ExitCode() == 3 {
			log.Noticef(context.TODO(), "gpasswd exited with code 3 (group or user does not exist); ignoring: %s", out)
			return nil
		}
		return fmt.Errorf("%q returned: %v\nOutput: %s", strings.Join(cmd.Args, " "), err, out)
	}
	return nil
}
