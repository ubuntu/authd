package localentries

import (
	"context"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/testsdetection"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// GroupFile is the default local group fill.
const GroupFile = "/etc/group"

type options struct {
	// groupInputPath is the path used to read the group file. Defaults to
	// [GroupFile], but can be overwritten in tests.
	groupInputPath string
	// groupOutputPath is the path used to write the group file. Defaults to
	// [GroupFile], but can be overwritten in tests.
	groupOutputPath string

	// These are the lock and unlock functions to be used that can be overridden
	// for testing purposes.
	writeLockFunc   func() error
	writeUnlockFunc func() error

	// userDBLocked is the userDBLocked instance to use and that can be
	// replaced for testing purposes.
	userDBLocked *UserDBLocked
}

var defaultOptions = options{
	// userDBLocked is used as the instance for locked groups when
	// no test options are provided.
	userDBLocked: &UserDBLocked{},

	groupInputPath:  GroupFile,
	groupOutputPath: GroupFile,

	writeLockFunc:   userslocking.WriteLock,
	writeUnlockFunc: userslocking.WriteUnlock,
}

// Option represents an optional function to override [NewUserDBLocked] default values.
type Option func(*options)

// UserDBLocked is a struct that holds the current users and groups while
// ensuring that the system's user database is locked to prevent concurrent
// modifications.
type UserDBLocked struct {
	// mu is a mutex that protects the refCount and entries fields.
	mu sync.Mutex
	// refCount is used to track how many times the GroupsWithLock instance has
	// been returned by [NewWithLock].
	refCount uint64

	// localGroupsMu is the mutex that protects us globally from concurrent
	// reads and writes on the group file.
	// We need this to ensure that we don't write to the file while we're
	// parsing it to prevent that we may do concurrent writes on it.
	// The mutex is tied to the lock instance since it's where different file
	// paths can be defined (through options), and avoids us to have a shared
	// global mutex when the locked instances are different from the default.
	localGroupsMu sync.Mutex

	// userEntries holds the current group entries.
	userEntries []types.UserEntry
	// groupEntries holds the current group entries.
	groupEntries []types.GroupEntry
	// localGroupEntries holds the current group entries.
	localGroupEntries []types.GroupEntry

	// options to set the local entries context.
	options options
}

// NewUserDBLocked gets a [UserDBLocked] instance with a lock on the system's user database.
// It returns an unlock function that should be called to unlock system's user database.
func NewUserDBLocked(args ...Option) (entries *UserDBLocked, unlock func() error, err error) {
	defer decorate.OnError(&err, "could not lock local groups")

	unlock = func() error {
		entries.mu.Lock()
		defer entries.mu.Unlock()

		if entries.refCount == 0 {
			return fmt.Errorf("groups were already unlocked")
		}

		entries.refCount--
		if entries.refCount != 0 {
			return nil
		}

		log.Debug(context.Background(), "Unlocking local entries")
		entries.userEntries = nil
		entries.localGroupEntries = nil
		entries.groupEntries = nil

		return entries.options.writeUnlockFunc()
	}

	opts := defaultOptions
	testingMode := args != nil

	if testingMode {
		testsdetection.MustBeTesting()
		opts.userDBLocked = &UserDBLocked{}

		for _, arg := range args {
			arg(&opts)
		}
	}

	entries = opts.userDBLocked

	entries.mu.Lock()
	defer entries.mu.Unlock()

	if entries.refCount != 0 {
		entries.refCount++
		return entries, unlock, nil
	}

	log.Debug(context.Background(), "Locking local entries")

	if err := opts.writeLockFunc(); err != nil {
		return nil, nil, err
	}

	entries.options = opts
	entries.refCount++

	return entries, unlock, nil
}

// MustBeLocked ensures wether the entries are locked.
func (l *UserDBLocked) MustBeLocked() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()
}

func (l *UserDBLocked) mustBeLocked() {
	if l.refCount == 0 {
		panic("locked entries are not locked!")
	}
}

// GetUserEntries gets the user entries.
func (l *UserDBLocked) GetUserEntries() (entries []types.UserEntry, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	if l.userEntries != nil {
		return l.userEntries, nil
	}

	l.userEntries, err = GetPasswdEntries()
	return l.userEntries, err
}

// GetGroupEntries gets the group entries.
func (l *UserDBLocked) GetGroupEntries() (entries []types.GroupEntry, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	if l.groupEntries != nil {
		return l.groupEntries, nil
	}

	l.groupEntries, err = GetGroupEntries()
	return l.groupEntries, err
}

// GetLocalGroupEntries gets the local group entries.
func (l *UserDBLocked) GetLocalGroupEntries() (entries []types.GroupEntry, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	if l.localGroupEntries != nil {
		return l.localGroupEntries, nil
	}

	l.localGroupEntries, err = parseLocalGroups(l.options.groupInputPath)
	return l.localGroupEntries, err
}

// updateLocalGroupEntriesCache updates the local group entries.
func (l *UserDBLocked) updateLocalGroupEntriesCache(entries []types.GroupEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	l.localGroupEntries = types.DeepCopyGroupEntries(entries)
}

// lockGroupFile locks the read/write operation on the group file and returns
// an unlock function.
func (l *UserDBLocked) lockGroupFile() (unlock func()) {
	l.MustBeLocked()

	l.localGroupsMu.Lock()
	return l.localGroupsMu.Unlock
}
