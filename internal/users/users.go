// Package users support all common action on the system for user handling.
package users

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/ubuntu/decorate"
)

// UserInfo is the user information returned by the broker.
type UserInfo struct {
	Name  string
	UID   int
	Gecos string
	Dir   string
	Shell string

	Groups []GroupInfo
}

// GroupInfo is the group information returned by the broker.
type GroupInfo struct {
	Name string
	GID  *int
}

type options struct {
	groupPath    string
	gpasswdCmd   []string
	getUsersFunc func() []string
}

// Option represents an optional function to override UpdateLocalGroups default values.
type Option func(*options)

// UpdateLocalGroups synchronizes for the given user the local group list with the current group list from UserInfo.
func (u *UserInfo) UpdateLocalGroups(args ...Option) (err error) {
	defer decorate.OnError(&err, "could not update local groups for user %q", u.Name)

	if u.Name == "" {
		return errors.New("empty user name")
	}

	for _, g := range u.Groups {
		if g.Name == "" && g.GID == nil {
			return errors.New("empty group provided")
		}
	}

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	currentLocalGroups, err := existingLocalGroups(u.Name, opts.groupPath)
	if err != nil {
		return err
	}

	groupsToAdd, groupsToRemove := computeGroupOperation(u.Groups, currentLocalGroups)

	for _, g := range groupsToRemove {
		args := opts.gpasswdCmd[1:]
		args = append(args, "--delete", u.Name, g)
		if err := runGPasswd(opts.gpasswdCmd[0], args...); err != nil {
			return err
		}
	}
	for _, g := range groupsToAdd {
		args := opts.gpasswdCmd[1:]
		args = append(args, "--add", u.Name, g)
		if err := runGPasswd(opts.gpasswdCmd[0], args...); err != nil {
			return err
		}
	}

	return nil
}

// existingLocalGroups returns which groups from groupPath the user is part of.
func existingLocalGroups(user, groupPath string) (groups []string, err error) {
	defer decorate.OnError(&err, "could not fetch existing local group")

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
			return nil, fmt.Errorf("malformed entry in group file (should have 4 separators): %q", t)
		}

		n := elems[0]
		users := strings.Split(elems[3], ",")
		if !slices.Contains(users, user) {
			continue
		}

		groups = append(groups, n)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

// computeGroupOperation returns which local groups to add and which to remove comparing with the existing group state.
// Only local groups (with no GID) are considered from GroupInfo.
func computeGroupOperation(newGroupsInfo []GroupInfo, currentLocalGroups []string) (groupsToAdd []string, groupsToRemove []string) {
	newGroups := make(map[string]struct{})
	for _, grp := range newGroupsInfo {
		if grp.GID != nil {
			continue
		}
		newGroups[grp.Name] = struct{}{}
	}

	currGroups := make(map[string]struct{})
	for _, g := range currentLocalGroups {
		currGroups[g] = struct{}{}
	}

	for g := range newGroups {
		// already in current group file
		if _, ok := currGroups[g]; ok {
			continue
		}
		groupsToAdd = append(groupsToAdd, g)
	}

	for g := range currGroups {
		// was in that group but not anymore
		if _, ok := newGroups[g]; ok {
			continue
		}
		groupsToRemove = append(groupsToRemove, g)
	}

	return groupsToAdd, groupsToRemove
}

// CleanUserFromLocalGroups removes the user from all local groups.
func CleanUserFromLocalGroups(user string, args ...Option) (err error) {
	defer decorate.OnError(&err, "could not clean user %q from local groups", user)

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	// Get the list of local groups the user belong to
	groups, err := existingLocalGroups(user, opts.groupPath)
	if err != nil {
		return err
	}

	for _, group := range groups {
		args := opts.gpasswdCmd[1:]
		args = append(args, "--delete", user, group)
		if err := runGPasswd(opts.gpasswdCmd[0], args...); err != nil {
			return err
		}
	}

	return nil
}

// CleanLocalGroups removes all unexistent users from the local groups.
func CleanLocalGroups(args ...Option) (err error) {
	defer decorate.OnError(&err, "could not clean local groups completely")

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	// Add the existingUsers to a map to speed up search
	existingUsers := make(map[string]struct{})
	for _, username := range opts.getUsersFunc() {
		existingUsers[username] = struct{}{}
	}
	// If no username was returned, something probably went wrong during the getpwent call and we should stop,
	// otherwise we would remove all users from the local groups.
	if len(existingUsers) == 0 {
		return fmt.Errorf("no existing users found, local groups won't be cleaned")
	}

	// Get the list of local groups
	f, err := os.Open(opts.groupPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Format of a line composing the group file is:
	// group_name:password:group_id:user1,…,usern
	var delOps [][]string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		if t == "" {
			continue
		}
		elems := strings.Split(t, ":")
		if len(elems) != 4 {
			return fmt.Errorf("malformed entry in group file (should have 4 separators): %q", t)
		}

		groupName := elems[0]
		users := strings.Split(elems[3], ",")
		for _, user := range users {
			if _, ok := existingUsers[user]; ok {
				continue
			}

			// User doesn't exist anymore, remove it from the group
			args := opts.gpasswdCmd[1:]
			delOps = append(delOps, append(args, "--delete", user, groupName))
		}
	}
	if scanner.Err() != nil {
		return scanner.Err()
	}
	f.Close()

	// Execute the deletion operations
	for _, op := range delOps {
		if cmdErr := runGPasswd(opts.gpasswdCmd[0], op...); cmdErr != nil {
			err = errors.Join(err, cmdErr)
		}
	}

	return err
}

// runGPasswd is a wrapper to cmdName ignoring exit code 3, meaning that the group doesn't exist.
// Note: it’s the same return code for user not existing, but it’s something we are in control of as we
// are responsible for the user itself and parsing the output is not desired.
func runGPasswd(cmdName string, args ...string) error {
	cmd := exec.Command(cmdName, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState.ExitCode() == 3 {
			slog.Info(fmt.Sprintf("ignoring gpasswd error: %s", out))
			return nil
		}
		return fmt.Errorf("%q returned: %v\nOutput: %s", strings.Join(cmd.Args, " "), err, out)
	}
	return nil
}
