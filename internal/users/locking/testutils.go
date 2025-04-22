package userslocking

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/log"
)

var (
	overrideLocked atomic.Bool

	overriddenMu              sync.Mutex
	overriddenWriteLockImpl   []func() error
	overriddenWriteUnlockImpl []func() error
)

// Z_ForTests_OverrideLocking is a function to override the locking functions
// for testing purposes.
// It simulates the real behavior but without actual file locking.
// Use [Z_ForTests_RestoreLocking] once done with it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_OverrideLocking() {
	testsdetection.MustBeTesting()

	overriddenMu.Lock()
	defer overriddenMu.Unlock()

	overriddenWriteLockImpl = append(overriddenWriteLockImpl, writeLockImpl)
	writeLockImpl = func() error {
		if !overrideLocked.CompareAndSwap(false, true) {
			return fmt.Errorf("%w: already locked", ErrLock)
		}

		log.Debug(context.Background(), "TestOverride: Local entries locked!")
		return nil
	}

	overriddenWriteUnlockImpl = append(overriddenWriteUnlockImpl, writeUnlockImpl)
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

// Z_ForTests_OverrideLockingAsLockedExternally simulates a scenario where the
// user database is locked by an external process.
//
// When called, it marks the user database as locked, causing any subsequent
// locking attempts by authd (via [WriteLock]) to block until the provided
// context is cancelled.
//
// This does not use real file locking. The lock can be released either
// by cancelling the context or by calling [WriteUnlock]. After the test,
// [Z_ForTests_RestoreLocking] is called automatically to restore normal behavior.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_OverrideLockingAsLockedExternally(t *testing.T, ctx context.Context) {
	t.Helper()

	testsdetection.MustBeTesting()

	overriddenMu.Lock()
	defer overriddenMu.Unlock()

	t.Cleanup(Z_ForTests_RestoreLocking)

	// This channel is used to synchronize the lock and unlock operations.
	// It uses a buffer of size 1 so that it can be locked exactly once
	// and then blocks until the unlock operation is called.
	lockCh := make(chan struct{}, 1)

	overriddenWriteLockImpl = append(overriddenWriteLockImpl, writeLockImpl)
	writeLockImpl = func() error {
		for {
			select {
			case lockCh <- struct{}{}:
				log.Debug(ctx, "TestOverrideExternallyLocked: Local entries external lock released!")
			case <-time.After(maxWait):
				return ErrLockTimeout
			}

			if overrideLocked.CompareAndSwap(false, true) {
				log.Debug(ctx, "TestOverrideExternallyLocked: Local entries locked!")
				break
			}
		}
		return nil
	}

	overriddenWriteUnlockImpl = append(overriddenWriteUnlockImpl, writeUnlockImpl)
	writeUnlockImpl = func() error {
		if !overrideLocked.CompareAndSwap(true, false) {
			return ErrUnlock
		}

		<-lockCh
		log.Debug(ctx, "TestOverrideExternallyLocked: Local entries unlocked!")
		return nil
	}

	done := atomic.Bool{}
	writeUnlockImpl := writeUnlockImpl
	cleanup := func() {
		if !done.CompareAndSwap(false, true) {
			return
		}
		if !overrideLocked.Load() {
			return
		}
		err := writeUnlockImpl()
		require.NoError(t, err, "Unlocking should be allowed")
	}

	t.Cleanup(cleanup)

	err := writeLockImpl()
	require.NoError(t, err, "Locking should be allowed")

	go func() {
		<-ctx.Done()
		cleanup()
	}()
}

// Z_ForTests_RestoreLocking restores the locking overridden done by
// [Z_ForTests_OverrideLocking] or [Z_ForTests_OverrideLockingAsLockedExternally].
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_RestoreLocking() {
	testsdetection.MustBeTesting()

	if overrideLocked.Load() {
		panic("Lock has not been released before restoring!")
	}

	overriddenMu.Lock()
	defer overriddenMu.Unlock()

	popLast := func(l []func() error) ([]func() error, func() error) {
		n := len(l) - 1
		v, l := l[n], l[:n]
		return l, v
	}

	overriddenWriteLockImpl, writeLockImpl = popLast(overriddenWriteLockImpl)
	overriddenWriteUnlockImpl, writeUnlockImpl = popLast(overriddenWriteUnlockImpl)
}
