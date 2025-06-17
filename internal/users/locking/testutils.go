package userslocking

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/log"
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
			return fmt.Errorf("%w: already locked", ErrLock)
		}

		log.Debug(context.Background(), "TestOverride: Local entries locked!")
		return nil
	}

	writeUnlockImpl = func() error {
		if !overrideLocked.CompareAndSwap(true, false) {
			return fmt.Errorf("%w: already unlocked", ErrUnlock)
		}

		log.Debug(context.Background(), "TestOverride: Local entries unlocked!")
		return nil
	}
}

// Z_ForTests_OverrideLockingWithCleanup is a function to override the locking
// functions for testing purposes.
// It simulates the real behavior but without actual file locking.
// This implicitly calls [Z_ForTests_RestoreLocking] once the test is completed.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_OverrideLockingWithCleanup(t *testing.T) {
	t.Helper()

	testsdetection.MustBeTesting()

	Z_ForTests_OverrideLocking()
	t.Cleanup(Z_ForTests_RestoreLocking)
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
