// Package userutils provides functions related to system users and groups.
package userutils

import (
	"errors"
	"sync/atomic"

	"github.com/ubuntu/authd/internal/testsdetection"
)

var (
	writeLockShadowPasswordImpl   = writeLockShadowPassword
	writeUnlockShadowPasswordImpl = writeUnlockShadowPassword

	overrideLocked atomic.Bool
)

var (
	// ErrLock is the error when locking the database fails.
	ErrLock = errors.New("failed to lock the shadow password database")

	// ErrUnlock is the error when unlocking the database fails.
	ErrUnlock = errors.New("failed to lock the shadow password database")
)

// WriteLockShadowPassword locks for writing the shadow password database by
// using the standard libc lckpwdf() function.
// While the database is locked read operations can happen, but no other process
// is allowed to write.
// Note that this call will block all the other processes trying to access the
// database in write mode, while it will return an error if called while the
// lock is already hold by this process.
func WriteLockShadowPassword() error {
	return writeLockShadowPasswordImpl()
}

// WriteUnlockShadowPassword unlocks for writing the shadow password database by
// using the standard libc ulckpwdf() function.
// As soon as this function is called all the other waiting processes will be
// allowed to take the lock.
func WriteUnlockShadowPassword() error {
	return writeUnlockShadowPasswordImpl()
}

// OverrideShadowPasswordLocking is a function to override the locking functions
// for testing purposes.
// It simulates the real behavior but without actual file locking.
func OverrideShadowPasswordLocking() {
	testsdetection.MustBeTesting()

	writeLockShadowPasswordImpl = func() error {
		if !overrideLocked.CompareAndSwap(false, true) {
			return ErrLock
		}
		return nil
	}

	writeUnlockShadowPasswordImpl = func() error {
		if !overrideLocked.CompareAndSwap(true, false) {
			return ErrUnlock
		}
		return nil
	}
}

// RestoreShadowPasswordLocking restores the locking overridden done by
// [OverrideShadowPasswordLocking].
func RestoreShadowPasswordLocking() {
	testsdetection.MustBeTesting()

	writeLockShadowPasswordImpl = writeLockShadowPassword
	writeUnlockShadowPasswordImpl = writeUnlockShadowPassword
}
