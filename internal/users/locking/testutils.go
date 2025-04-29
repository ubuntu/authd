package userslocking

import (
	"sync/atomic"

	"github.com/ubuntu/authd/internal/testsdetection"
)

var overrideLocked atomic.Bool

// Z_ForTests_OverrideLocking is a function to override the locking functions
// for testing purposes.
// It simulates the real behavior but without actual file locking.
// Use [Z_ForTests_RestoreLocking] once done with it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_OverrideLocking() {
	testsdetection.MustBeTesting()

	writeLockImpl = func() error {
		if !overrideLocked.CompareAndSwap(false, true) {
			return ErrLock
		}
		return nil
	}

	writeUnlockImpl = func() error {
		if !overrideLocked.CompareAndSwap(true, false) {
			return ErrUnlock
		}
		return nil
	}
}

// Z_ForTests_RestoreLocking restores the locking overridden done by
// [Z_ForTests_OverrideLocking].
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_RestoreLocking() {
	testsdetection.MustBeTesting()

	writeLockImpl = writeLock
	writeUnlockImpl = writeUnlock
}
