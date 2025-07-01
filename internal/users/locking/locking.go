// Package userslocking implements locking of the local user and group files
// (/etc/passwd, /etc/groups, /etc/shadow, /etc/gshadow) via the libc lckpwdf()
// function.
//
// It is recommended by systemd to hold this lock when picking a new UID/GID to
// avoid races, even if the new user/group is not added to the local user/group
// files. See https://github.com/systemd/systemd/blob/main/docs/UIDS-GIDS.md.
package userslocking

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	writeLockImpl   = writeLock
	writeUnlockImpl = writeUnlock

	// maxWait is the maximum wait time for a lock to happen.
	// We mimic the libc behavior, in case we don't get SIGALRM'ed.
	maxWait = 16 * time.Second
)

var (
	// ErrLock is the error when locking the database fails.
	ErrLock = errors.New("failed to lock the system's user database")

	// ErrUnlock is the error when unlocking the database fails.
	ErrUnlock = errors.New("failed to unlock the system's user database")

	// ErrLockTimeout is the error when unlocking the database fails because of timeout.
	ErrLockTimeout = fmt.Errorf("%w: timeout", ErrLock)
)

type UserDBLock struct {
	mu   sync.Mutex
	cond *sync.Cond
	held bool
}

// NewUserDBLock creates a new UserDBLock.
func NewUserDBLock() *UserDBLock {
	lock := &UserDBLock{}
	lock.cond = sync.NewCond(&lock.mu)
	return lock
}

// Lock acquires the user database lock.
// Returns an error if already held.
func (l *UserDBLock) Lock() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for l.held {
		l.cond.Wait()
	}

	if err := writeLockInternal(); err != nil {
		return err
	}

	l.held = true

	return nil
}

// TryLock attempts to acquire the user database lock without blocking.
func (l *UserDBLock) TryLock() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.held {
		return fmt.Errorf("%w: lock already held", ErrLock)
	}

	if err := writeLockInternal(); err != nil {
		return err
	}

	l.held = true

	return nil
}

// Unlock releases the user database lock.
func (l *UserDBLock) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.held {
		return fmt.Errorf("%w: lock not held", ErrUnlock)
	}

	if err := writeUnlockImpl(); err != nil {
		return err
	}

	l.held = false
	l.cond.Signal()

	return nil
}

// IsHeld returns true if the lock is currently held.
func (l *UserDBLock) IsHeld() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}

type userDBLockKey struct{}

// WithUserDBLock creates and acquires a UserDBLock,
// storing it in the returned context.
func WithUserDBLock(parent context.Context) (context.Context, error) {
	lock := NewUserDBLock()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	ctx := context.WithValue(parent, userDBLockKey{}, lock)
	return ctx, nil
}

// GetUserDBLock retrieves the UserDBLock from context, if present.
func GetUserDBLock(ctx context.Context) *UserDBLock {
	val := ctx.Value(userDBLockKey{})
	if lock, ok := val.(*UserDBLock); ok {
		return lock
	}
	return nil
}

func writeLockInternal() error {
	done := make(chan error)
	writeLockImpl := writeLockImpl

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
