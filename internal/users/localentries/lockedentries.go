package localentries

import (
	"context"
	"errors"
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
}

var defaultOptions = options{
	groupInputPath:  GroupFile,
	groupOutputPath: GroupFile,
}

// Option represents an optional function to override UpdateLocalGroups default values.
type Option func(*options)

// WithLock is a struct that holds the current users and groups while
// ensuring that the system's user database is locked to prevent concurrent
// modifications.
type WithLock struct {
	// mu is a mutex that protects the refCount and entries fields.
	mu sync.Mutex
	// refCount is used to track how many times the GroupsWithLock instance has
	// been returned by NewWithLock.
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

// defaultEntriesWithLock is used as the instance for locked groups when
// no test options are provided.
var defaultEntriesWithLock = &WithLock{}

// NewWithLock gets a [WithLock] instance with a lock on the system's user database.
// It returns a cleanup function that should be called to unlock system's user database.
func NewWithLock(args ...Option) (entries *WithLock, cleanup func() error, err error) {
	defer decorate.OnError(&err, "could not lock local groups")

	if err := userslocking.WriteRecLock(); err != nil {
		return nil, nil, err
	}

	cleanupWithoutLock := func() error {
		if entries.refCount == 0 {
			return fmt.Errorf("groups were already unlocked")
		}

		entries.refCount--
		if entries.refCount != 0 {
			return userslocking.WriteRecUnlock()
		}

		log.Debug(context.Background(), "Unlocking local entries")
		entries.userEntries = nil
		entries.localGroupEntries = nil
		entries.groupEntries = nil

		return userslocking.WriteRecUnlock()
	}

	cleanup = func() error {
		entries.mu.Lock()
		defer entries.mu.Unlock()

		return cleanupWithoutLock()
	}

	entries = defaultEntriesWithLock
	testingMode := args != nil

	if testingMode {
		testsdetection.MustBeTesting()
		entries = &WithLock{}
	}

	entries.mu.Lock()
	defer entries.mu.Unlock()

	log.Debug(context.Background(), "Locking local entries")

	entries.refCount++
	if entries.refCount > 1 {
		return entries, cleanup, nil
	}

	opts := defaultOptions
	for _, arg := range args {
		arg(&opts)
	}

	entries.options = opts
	if err != nil {
		return nil, nil, errors.Join(err, cleanupWithoutLock())
	}

	return entries, cleanup, nil
}

// MustBeLocked ensures wether the entries are locked.
func (l *WithLock) MustBeLocked() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.refCount == 0 {
		panic("locked entries are not locked!")
	}
}

func (l *WithLock) mustBeLocked() {
	if l.refCount == 0 {
		panic("locked entries are not locked!")
	}
}

// GetUserEntries gets the user Entries.
func (l *WithLock) GetUserEntries() (entries []types.UserEntry, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	if l.userEntries != nil {
		return l.userEntries, nil
	}

	l.userEntries, err = GetPasswdEntries()
	return l.userEntries, err
}

// GetGroupEntries gets the group Entries.
func (l *WithLock) GetGroupEntries() (entries []types.GroupEntry, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	if l.groupEntries != nil {
		return l.groupEntries, nil
	}

	l.groupEntries, err = GetGroupEntries()
	return l.groupEntries, err
}

// GetLocalGroupEntries gets the local group Entries.
func (l *WithLock) GetLocalGroupEntries() (entries []types.GroupEntry, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	if l.localGroupEntries != nil {
		return l.localGroupEntries, nil
	}

	l.localGroupEntries, err = parseLocalGroups(l.options.groupInputPath)
	return l.localGroupEntries, err
}

// updateLocalGroupEntriesCache updates the local group Entries.
func (l *WithLock) updateLocalGroupEntriesCache(entries []types.GroupEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.mustBeLocked()

	l.localGroupEntries = types.DeepCopyGroupEntries(entries)
}

// lockGroupFile locks the read/write operation on the group file and returns
// an unlock function.
func (l *WithLock) lockGroupFile() (unlock func()) {
	l.localGroupsMu.Lock()
	return l.localGroupsMu.Unlock
}
