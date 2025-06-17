// Package userslocking implements locking of the local user and group files
// (/etc/passwd, /etc/groups, /etc/shadow, /etc/gshadow) via the libc lckpwdf()
// function.
//
// It is recommended by systemd to hold this lock when picking a new UID/GID to
// avoid races, even if the new user/group is not added to the local user/group
// files. See https://github.com/systemd/systemd/blob/main/docs/UIDS-GIDS.md.
package userslocking

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	writeLockImpl   = writeLock
	writeUnlockImpl = writeUnlock

	writeLocksCount   uint64
	writeLocksCountMu sync.RWMutex

	// maxWait is the maximum wait time for a lock to happen.
	// We mimic the libc behavior, in case we don't get SIGALRM'ed.
	maxWait = 16 * time.Second
)

var (
	// ErrLock is the error when locking the database fails.
	ErrLock = errors.New("failed to lock the shadow password database")

	// ErrUnlock is the error when unlocking the database fails.
	ErrUnlock = errors.New("failed to unlock the shadow password database")

	// ErrLockTimeout is the error when unlocking the database fails because of timeout.
	ErrLockTimeout = fmt.Errorf("%w: timeout", ErrLock)
)

// WriteLock locks for writing the the local user entries database by using
// the standard libc lckpwdf() function.
// While the database is locked read operations can happen, but no other process
// is allowed to write.
// Note that this call will block all the other processes trying to access the
// database in write mode, while it will return an error if called while the
// lock is already hold by this process.
func WriteLock() error {
	writeLocksCountMu.RLock()
	defer writeLocksCountMu.RUnlock()

	if writeLocksCount > 0 {
		return fmt.Errorf("%w: mixing recursive and normal locking", ErrLock)
	}

	return writeLockInternal()
}

func writeLockInternal() error {
	done := make(chan error)
	go func() {
		done <- writeLockImpl()
	}()

	select {
	// lckpwdf when called from cgo doesn't behave exactly the same, likely
	// because alarms are handled by go runtime, so do it manually here by
	// failing if "lock not obtained within 15 seconds" as per lckpwdf.3.
	// Keep this in sync with what lckpwdf does, adding an extra second.
	case <-time.After(maxWait):
		return ErrLockTimeout
	case err := <-done:
		return err
	}
}

// WriteUnlock unlocks for writing the local user entries database by using
// the standard libc ulckpwdf() function.
// As soon as this function is called all the other waiting processes will be
// allowed to take the lock.
func WriteUnlock() error {
	writeLocksCountMu.RLock()
	defer writeLocksCountMu.RUnlock()

	if writeLocksCount > 0 {
		return fmt.Errorf("%w: mixing recursive and normal locking", ErrUnlock)
	}

	return writeUnlockImpl()
}

// WriteRecLock is like [WriteLock] but it allows recursive locks, so that if
// called when authd already has shadow password lock, then we increase the
// locking reference count without returning an error.
func WriteRecLock() error {
	writeLocksCountMu.Lock()
	defer writeLocksCountMu.Unlock()

	if writeLocksCount == 0 {
		if err := writeLockInternal(); err != nil {
			return err
		}
	}

	writeLocksCount++
	return nil
}

// WriteRecUnlock is like [WriteUnlock] but it reduces the reference count
// of the recursive lock added by [WriteRecLock].
func WriteRecUnlock() error {
	writeLocksCountMu.Lock()
	defer writeLocksCountMu.Unlock()

	if writeLocksCount == 0 {
		return fmt.Errorf("%w: no locks found", ErrUnlock)
	}

	if writeLocksCount > 1 {
		writeLocksCount--
		return nil
	}

	if err := writeUnlockImpl(); err != nil {
		return err
	}

	writeLocksCount--
	return nil
}
