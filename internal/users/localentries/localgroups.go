// Package localentries provides functions to retrieve passwd and group entries and to update the groups of a user.
package localentries

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/ubuntu/authd/internal/sliceutils"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

var defaultOptions = options{
	groupPath:    "/etc/group",
	gpasswdCmd:   []string{"gpasswd"},
	getUsersFunc: getPasswdUsernames,
}

type options struct {
	groupPath    string
	gpasswdCmd   []string
	getUsersFunc func() ([]string, error)
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

	localGroupsMu.Lock()
	defer localGroupsMu.Unlock()
	groupsToAdd := sliceutils.Difference(newGroups, currentGroups)
	log.Debugf(context.TODO(), "Adding to groups: %v", groupsToAdd)
	groupsToRemove := sliceutils.Difference(oldGroups, newGroups)
	// Only remove user from groups which they are part of
	groupsToRemove = sliceutils.Intersection(groupsToRemove, currentGroups)
	log.Debugf(context.TODO(), "Removing from groups: %v", groupsToRemove)

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

// getPasswdUsernames gets the passwd entries and returns their usernames.
func getPasswdUsernames() ([]string, error) {
	var usernames []string
	entries, err := GetPasswdEntries()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		usernames = append(usernames, e.Name)
	}

	return usernames, nil
}

// existingLocalGroups returns which groups from groupPath the user is part of.
func existingLocalGroups(user, groupPath string) (groups []string, err error) {
	defer decorate.OnError(&err, "could not fetch existing local group")

	localGroupsMu.RLock()
	defer localGroupsMu.RUnlock()
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

// CleanUser removes the user from all local groups.
func CleanUser(user string, args ...Option) (err error) {
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

	localGroupsMu.Lock()
	defer localGroupsMu.Unlock()
	for _, group := range groups {
		args := opts.gpasswdCmd[1:]
		args = append(args, "--delete", user, group)
		if err := runGPasswd(opts.gpasswdCmd[0], args...); err != nil {
			return err
		}
	}

	return nil
}

// Clean removes all unexistent users from the local groups.
func Clean(args ...Option) (err error) {
	defer decorate.OnError(&err, "could not clean local groups completely")

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	localGroupsMu.Lock()
	defer localGroupsMu.Unlock()

	// Add the existingUsers to a map to speed up search
	existingUsers := make(map[string]struct{})
	usernames, err := opts.getUsersFunc()
	if err != nil {
		return err
	}
	for _, username := range usernames {
		existingUsers[username] = struct{}{}
	}
	// If no username was returned, something probably went wrong during the getpwent call and we should stop,
	// otherwise we would remove all users from the local groups.
	if len(existingUsers) == 0 {
		return errors.New("no existing users found, local groups won't be cleaned")
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
			log.Infof(context.TODO(), "ignoring gpasswd error: %s", out)
			return nil
		}
		return fmt.Errorf("%q returned: %v\nOutput: %s", strings.Join(cmd.Args, " "), err, out)
	}
	return nil
}
