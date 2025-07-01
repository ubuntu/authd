package localentries

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"slices"
	"strconv"
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
	// inputGroupPath is the path used to read the group file. Defaults to
	// [GroupFile], but can be overwritten in tests.
	inputGroupPath string
	// outputGroupPath is the path used to write the group file. Defaults to
	// [GroupFile], but can be overwritten in tests.
	outputGroupPath string

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

	inputGroupPath:  GroupFile,
	outputGroupPath: GroupFile,

	writeLockFunc:   userslocking.WriteLock,
	writeUnlockFunc: userslocking.WriteUnlock,
}

// Option represents an optional function to override [UserDBLocked] default values.
type Option func(*options)

type invalidEntry struct {
	// lineNum is the line number in the group file where the invalid line was found.
	lineNum int
	// line is the content of the invalid line.
	line string
}

// UserDBLocked is a struct that holds the current users and groups while
// ensuring that the system's user database is locked to prevent concurrent
// modifications.
type UserDBLocked struct {
	// mu is a mutex that protects the refCount and entries fields.
	mu sync.Mutex
	// refCount is used to track how many times the GroupsWithLock instance has
	// been returned by [NewUserDBLocked].
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
	// localGroupInvalidEntries holds the current group invalid entries.
	localGroupInvalidEntries []invalidEntry

	// options to set the local entries context.
	options options
}

type userDBLockKey struct{}

// GetUserDBLocked retrieves the [WithLock] from context, if present.
func GetUserDBLocked(ctx context.Context) *UserDBLocked {
	if l, ok := ctx.Value(userDBLockKey{}).(*UserDBLocked); ok {
		l.MustBeLocked()
		return l
	}
	return nil
}

// ContextUserDBLocked gets a context instance with a lock on the system's
// user database.
// It returns an unlock function that should be called to unlock it.
//
// It can called safely multiple times and will return always a new context that
// is always bound to the same instance of [UserDBLocked] with increased
// reference counting (that is released through the unlock function).
// Use [GetUserDBLocked] to retrieve it.
func ContextUserDBLocked(parent context.Context, args ...Option) (ctx context.Context, unlock func() error, err error) {
	defer decorate.OnError(&err, "could not lock local groups")

	var entries *UserDBLocked
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
	ctx = context.WithValue(parent, userDBLockKey{}, entries)

	entries.mu.Lock()
	defer entries.mu.Unlock()

	if entries.refCount != 0 {
		entries.refCount++
		return ctx, unlock, nil
	}

	log.Debug(context.Background(), "Locking local entries")

	if err := opts.writeLockFunc(); err != nil {
		return nil, nil, err
	}

	entries.options = opts
	entries.refCount++

	return ctx, unlock, nil
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

	l.localGroupEntries, l.localGroupInvalidEntries, err = parseLocalGroups(
		l.options.inputGroupPath)
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

// IsUniqueUserName returns if a user exists for the given name.
func (l *UserDBLocked) IsUniqueUserName(name string) (unique bool, err error) {
	l.MustBeLocked()

	users, err := l.GetUserEntries()
	if err != nil {
		return false, err
	}

	// Let's try to check first the (potentially) cached entries.
	if slices.ContainsFunc(users, func(u types.UserEntry) bool {
		return u.Name == name
	}) {
		return false, nil
	}

	// In case we found no user, we need to still double check NSS by name.
	_, err = user.Lookup(name)
	if err == nil {
		return false, nil
	}

	var userErr user.UnknownUserError
	if !errors.As(err, &userErr) {
		return false, err
	}

	return true, nil
}

// IsUniqueGroupName returns if a user exists for the given UID.
func (l *UserDBLocked) IsUniqueGroupName(group string) (unique bool, err error) {
	l.MustBeLocked()

	// Let's try to check first the (potentially) cached entries.
	groups, err := l.GetGroupEntries()
	if err != nil {
		return false, err
	}

	if slices.ContainsFunc(groups, func(g types.GroupEntry) bool {
		return g.Name == group
	}) {
		return false, nil
	}

	// In case we found no user, we need to still double check NSS by name.
	_, err = user.LookupGroup(group)
	if err == nil {
		return false, nil
	}

	var groupErr user.UnknownGroupError
	if !errors.As(err, &groupErr) {
		return false, err
	}

	return true, nil
}

// IsUniqueUID returns if a user exists for the given UID.
func (l *UserDBLocked) IsUniqueUID(uid uint32) (unique bool, err error) {
	l.MustBeLocked()

	users, err := l.GetUserEntries()
	if err != nil {
		return false, err
	}

	// Let's try to check first the (potentially) cached entries.
	if slices.ContainsFunc(users, func(u types.UserEntry) bool {
		return u.UID == uid
	}) {
		return false, nil
	}

	// In case we found no user, we need to still double check NSS by ID.
	_, err = user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err == nil {
		return false, nil
	}

	var userErr user.UnknownUserIdError
	if !errors.As(err, &userErr) {
		return false, err
	}

	// Also check that the UID is not used by a group.
	return l.IsUniqueGID(uid)
}

// IsUniqueGID returns if a user exists for the given UID.
func (l *UserDBLocked) IsUniqueGID(gid uint32) (unique bool, err error) {
	l.MustBeLocked()

	// Let's try to check first the (potentially) cached entries.
	groups, err := l.GetGroupEntries()
	if err != nil {
		return false, err
	}

	if slices.ContainsFunc(groups, func(g types.GroupEntry) bool {
		return g.GID == gid
	}) {
		return false, nil
	}

	// Then the (potentially) cached user entries, for user local groups.
	users, err := l.GetUserEntries()
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(users, func(u types.UserEntry) bool {
		return u.GID == gid
	}) {
		return false, nil
	}

	// In case we found no user, we need to still double check NSS by ID.
	_, err = user.LookupGroupId(strconv.FormatUint(uint64(gid), 10))
	if err == nil {
		return false, nil
	}

	var groupErr user.UnknownGroupIdError
	if !errors.As(err, &groupErr) {
		return false, err
	}

	return true, nil
}
