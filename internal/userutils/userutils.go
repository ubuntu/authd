// Package userutils provides functions related to system users and groups.
package userutils

import (
	"errors"
	"fmt"
	"os"
)

var groupFile = "/etc/group"

// LockGroupFile creates a lock file at /etc/group.lock, the same location
// used by tools like gpasswd and groupmod to prevent concurrent modifications
// to the /etc/group file.
// The lock file contains the PID of the process that created it.
func LockGroupFile() (err error) {
	lockFile, err := os.OpenFile(groupFile+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer func() {
		closeErr := lockFile.Close()
		err = errors.Join(err, closeErr)
	}()

	if _, err := lockFile.WriteString(fmt.Sprintf("%d", os.Getpid())); err != nil {
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}

	return nil
}

// UnlockGroupFile removes the lock file created by LockGroupFile.
func UnlockGroupFile() error {
	if err := os.Remove(groupFile + ".lock"); err != nil {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}
	return nil
}
