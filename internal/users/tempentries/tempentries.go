// Package tempentries provides a temporary user and group records.
package tempentries

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/localentries"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// Avoid to loop forever if we can't find an UID for the user, it's just better
// to fail after a limit is reached than hang or crash.
const maxIDGenerateIterations = 256

// NoDataFoundError is the error returned when no entry is found in the database.
type NoDataFoundError = db.NoDataFoundError

// IDGenerator is the interface that must be implemented by the ID generator.
type IDGenerator interface {
	GenerateUID() (uint32, error)
	GenerateGID() (uint32, error)
}

// TemporaryRecords is the in-memory temporary user and group records.
type TemporaryRecords struct {
	*idTracker
	*preAuthUserRecords
	*temporaryGroupRecords

	idGenerator   IDGenerator
	lockedRecords atomic.Pointer[temporaryRecordsLocked]
}

// temporaryRecordsLocked is the structure that allows to safely perform write
// changes to [TemporaryRecords] entries while the local entries database is
// locked.
type temporaryRecordsLocked struct {
	tr *TemporaryRecords

	locksMu sync.RWMutex
	locks   uint64

	cachedLocalPasswd []types.UserEntry
	cachedLocalGroup  []types.GroupEntry
}

func (l *temporaryRecordsLocked) mustBeLocked() (cleanup func()) {
	// While all the [temporaryRecordsLocked] operations are happening
	// we need to keep a read lock on it, to prevent it being unlocked
	// while some action is still ongoing.
	l.locksMu.RLock()
	cleanup = l.locksMu.RUnlock

	if l.locks == 0 {
		defer cleanup()
		panic("locked groups are not locked!")
	}

	return cleanup
}

// RegisterUser registers a temporary user with a unique UID.
//
// Returns the generated UID and a cleanup function that should be called to
// remove the temporary user once the user is added to the database.
func (l *temporaryRecordsLocked) RegisterUser(name string) (uid uint32, cleanup func(), err error) {
	unlock := l.mustBeLocked()
	defer unlock()

	return l.tr.registerUser(name)
}

// RegisterPreAuthUser registers a temporary user with a unique UID in our NSS
// handler (in memory, not in the database).
//
// The temporary user record is removed when [RegisterUser] is called with the
// same username.
//
// This method is called when a user logs in for the first time via SSH, in
// which case sshd checks if the user exists on the system (before
// authentication), and denies the login if the user does not exist.
// We pretend that the user exists by creating this temporary user record,
// which is converted into a permanent user record when [RegisterUser] is called
// after the user authenticated successfully.
//
// Returns the generated UID.
func (l *temporaryRecordsLocked) RegisterPreAuthUser(loginName string) (uid uint32, err error) {
	unlock := l.mustBeLocked()
	defer unlock()

	return l.tr.registerPreAuthUser(loginName)
}

// RegisterGroupForUser registers a temporary group with a unique GID in
// memory for the provided UID.
//
// Returns the generated GID and a cleanup function that should be called to
// remove the temporary group once the group was added to the database.
func (l *temporaryRecordsLocked) RegisterGroupForUser(uid uint32, name string) (gid uint32, cleanup func(), err error) {
	unlock := l.mustBeLocked()
	defer unlock()

	return l.tr.registerGroupForUser(uid, name)
}

func (l *temporaryRecordsLocked) lock() (err error) {
	defer decorate.OnError(&err, "could not lock temporary records")

	l.locksMu.Lock()
	defer l.locksMu.Unlock()

	if l.locks != 0 {
		l.locks++
		return nil
	}

	log.Debug(context.Background(), "Locking temporary records ")

	l.cachedLocalPasswd, err = localentries.GetPasswdEntries()
	if err != nil {
		return fmt.Errorf("failed to get passwd entries: %w", err)
	}
	l.cachedLocalGroup, err = localentries.GetGroupEntries()
	if err != nil {
		return fmt.Errorf("failed to get group entries: %w", err)
	}

	if err := userslocking.WriteRecLock(); err != nil {
		return err
	}

	l.locks++

	return nil
}

func (l *temporaryRecordsLocked) unlock() (err error) {
	defer decorate.OnError(&err, "could not unlock temporary records")

	l.locksMu.Lock()
	defer l.locksMu.Unlock()

	if l.locks == 0 {
		return errors.New("temporary records are already unlocked")
	}

	if l.locks != 1 {
		l.locks--
		return nil
	}

	log.Debug(context.Background(), "Unlocking temporary records")

	if err := userslocking.WriteRecUnlock(); err != nil {
		return err
	}

	l.cachedLocalPasswd = nil
	l.cachedLocalGroup = nil
	l.locks--

	return nil
}

// NewTemporaryRecords creates a new TemporaryRecords.
func NewTemporaryRecords(idGenerator IDGenerator) *TemporaryRecords {
	return &TemporaryRecords{
		idGenerator:           idGenerator,
		idTracker:             newIDTracker(),
		preAuthUserRecords:    newPreAuthUserRecords(idGenerator),
		temporaryGroupRecords: newTemporaryGroupRecords(idGenerator),
	}
}

// UserByID returns the user information for the given user ID.
func (r *TemporaryRecords) UserByID(uid uint32) (types.UserEntry, error) {
	return r.preAuthUserRecords.userByID(uid)
}

// UserByName returns the user information for the given user name.
func (r *TemporaryRecords) UserByName(name string) (types.UserEntry, error) {
	return r.preAuthUserRecords.userByName(name)
}

// LockForChanges locks the underneath system user entries databases and returns
// a [temporaryRecordsLocked] that allows to perform write-modifications to the
// user records in a way that is safe for the environment, and preventing races
// with other NSS modules.
//
//nolint:revive,nolintlint  // [temporaryRecordsLocked] is not a type we want to be able to use outside of this package
func (r *TemporaryRecords) LockForChanges() (tra *temporaryRecordsLocked, unlock func() error, err error) {
	defer decorate.OnError(&err, "failed to lock for changes")

	lockedRecords := &temporaryRecordsLocked{tr: r}

	for {
		if r.lockedRecords.CompareAndSwap(nil, lockedRecords) {
			break
		}

		// We've old locked records, let's just return these once we're sure!
		l := r.lockedRecords.Load()
		if l == nil {
			continue
		}
		if err := l.lock(); err != nil {
			return nil, nil, err
		}
		return l, l.unlock, nil
	}

	if err := lockedRecords.lock(); err != nil {
		return nil, nil, err
	}

	return lockedRecords, lockedRecords.unlock, nil
}

func (r *TemporaryRecords) registerUser(name string) (uid uint32, cleanup func(), err error) {
	defer decorate.OnError(&err, "failed to register user %q", name)

	if !r.isUniqueSystemUserName(name) {
		return 0, nil, fmt.Errorf("user %q already exists", name)
	}

	// Check if there is a pre-auth user with the same login name.
	user, err := r.preAuthUserRecords.userByLogin(name)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return 0, nil, fmt.Errorf("could not check if pre-auth user %q already exists: %w", name, err)
	}
	if err == nil {
		// There is a pre-auth user with the same login name.

		// Remove the pre-checked user as last thing, so that the user UID is exposed as the
		// ones we manage in authd while we're updating the users, although there's no risk
		// that someone else takes the UID here, since we're locked.
		cleanup := func() {
			r.preAuthUserRecords.rwMu.Lock()
			defer r.preAuthUserRecords.rwMu.Unlock()

			r.idTracker.forgetUser(name)
			if r.deletePreAuthUser(user.UID) {
				r.forgetID(user.UID)
			}
		}

		// Now that the user authenticated successfully, we don't really need to check again
		// if the UID is unique, since that's something we did while registering it, and we're
		// currently locked, so nothing else can add another user with such ID, but we do to
		// double check it, just in case.
		if !r.isUniqueSystemID(user.UID) {
			cleanup()
			return 0, nil, fmt.Errorf("UID (%d) or name (%q) from pre-auth user are not unique", user.UID, name)
		}

		return user.UID, cleanup, nil
	}

	// Generate a UID until we find a unique one
	for range maxIDGenerateIterations {
		uid, err = r.idGenerator.GenerateUID()
		if err != nil {
			return 0, nil, err
		}

		if unique := r.maybeTrackUniqueID(uid); !unique {
			// If the UID is not unique, generate a new one in the next iteration.
			continue
		}

		if tracked, currentUID := r.idTracker.trackUser(name, uid); !tracked {
			// If the loginName is already set for a different UID, it means
			// that another concurrent request won the race, so let's just
			// use that one instead.
			r.idTracker.forgetID(uid)
			return currentUID, func() {}, nil
		}

		log.Debugf(context.Background(), "Generated UID %d for user UID %s", uid, name)
		return uid, func() {
			r.idTracker.forgetID(uid)
			r.idTracker.forgetUser(name)
		}, nil
	}

	return 0, nil, fmt.Errorf("failed to find a valid UID after %d attempts",
		maxIDGenerateIterations)
}

func (r *TemporaryRecords) registerPreAuthUser(loginName string) (uid uint32, err error) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	// Check if there is already a pre-auth user for that name
	if uid, ok := r.preAuthUserRecords.uidByLogin[loginName]; ok {
		return uid, nil
	}

	if !r.isUniqueSystemUserName(loginName) {
		log.Errorf(context.Background(), "User already exists on the system: %+v", loginName)
		return 0, fmt.Errorf("user %q already exists on the system", loginName)
	}

	for range maxIDGenerateIterations {
		uid, err := r.preAuthUserRecords.generatePreAuthUserID(loginName)
		if err != nil {
			return 0, err
		}

		if unique := r.maybeTrackUniqueID(uid); !unique {
			// If the UID is not unique, generate a new one in the next iteration.
			continue
		}

		if tracked, currentUID := r.idTracker.trackUser(loginName, uid); !tracked {
			// If the loginName is already set for a different UID, it means
			// that another concurrent request won the race, so let's just
			// use that one instead.
			r.idTracker.forgetID(uid)
			uid = currentUID
		}

		if err := r.addPreAuthUser(uid, loginName); err != nil {
			r.idTracker.forgetUser(loginName)
			r.idTracker.forgetID(uid)
			return 0, fmt.Errorf("could not add pre-auth user record: %w", err)
		}

		return uid, nil
	}

	return 0, fmt.Errorf("failed to find a valid UID after %d attempts",
		maxIDGenerateIterations)
}

func (r *TemporaryRecords) passwdEntries() []types.UserEntry {
	l := r.lockedRecords.Load()
	if l == nil {
		testsdetection.MustBeTesting()

		entries, err := localentries.GetPasswdEntries()
		if err != nil {
			panic(fmt.Sprintf("Failed get local passwd: %v", err))
		}
		return entries
	}

	return l.cachedLocalPasswd
}

func (r *TemporaryRecords) groupEntries() []types.GroupEntry {
	l := r.lockedRecords.Load()
	if l == nil {
		testsdetection.MustBeTesting()

		entries, err := localentries.GetGroupEntries()
		if err != nil {
			panic(fmt.Sprintf("Failed get local groups: %v", err))
		}
		return entries
	}

	return l.cachedLocalGroup
}

func (r *TemporaryRecords) registerGroupForUser(uid uint32, name string) (gid uint32, cleanup func(), err error) {
	defer decorate.OnError(&err, "failed to register group %q for user ID %d", name, uid)

	if slices.ContainsFunc(r.groupEntries(), func(g types.GroupEntry) bool {
		return g.Name == name
	}) {
		return 0, nil, fmt.Errorf("group %q already exists", name)
	}

	r.temporaryGroupRecords.mu.Lock()
	defer r.temporaryGroupRecords.mu.Unlock()

	groupCleanup := func() {
		r.temporaryGroupRecords.mu.Lock()
		defer r.temporaryGroupRecords.mu.Unlock()

		if r.releaseTemporaryGroup(gid) {
			r.idTracker.forgetID(gid)
		}
	}

	if gid = r.getTemporaryGroup(name); gid != 0 {
		return gid, groupCleanup, nil
	}

	for range maxIDGenerateIterations {
		gid, err = r.temporaryGroupRecords.generateGroupID()
		if err != nil {
			return 0, nil, err
		}

		if gid == uid {
			// Generated GID matches current user UID, try again...
			continue
		}

		if unique := r.maybeTrackUniqueID(gid); !unique {
			// If the GID is not unique, generate a new one in the next iteration.
			continue
		}

		r.addTemporaryGroup(gid, name)

		return gid, groupCleanup, nil
	}

	return 0, nil, fmt.Errorf("failed to find a valid GID after %d attempts",
		maxIDGenerateIterations)
}

func (r *TemporaryRecords) maybeTrackUniqueID(id uint32) (unique bool) {
	defer func() {
		if !unique {
			log.Debugf(context.TODO(), "ID %d is not unique in this system", id)
		}
	}()

	if !r.isUniqueSystemID(id) {
		return false
	}

	return r.idTracker.trackID(id)
}

func (r *TemporaryRecords) isUniqueSystemID(id uint32) bool {
	if idx := slices.IndexFunc(r.passwdEntries(), func(p types.UserEntry) (found bool) {
		return p.UID == id
	}); idx != -1 {
		log.Debugf(context.Background(), "ID %d already in use by user %q",
			id, r.passwdEntries()[idx].Name)
		return false
	}

	if idx := slices.IndexFunc(r.groupEntries(), func(g types.GroupEntry) (found bool) {
		return g.GID == id
	}); idx != -1 {
		log.Debugf(context.Background(), "ID %d already in use by user %q",
			id, r.groupEntries()[idx].Name)
		return false
	}

	return true
}

func (r *TemporaryRecords) isUniqueSystemUserName(name string) bool {
	return !slices.ContainsFunc(r.passwdEntries(), func(p types.UserEntry) bool {
		return p.Name == name
	})
}
