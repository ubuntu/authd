// Package tempentries provides a temporary user and group records.
package tempentries

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/localentries"
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
	lockedRecords atomic.Pointer[TemporaryRecordsWithLock]
}

// TemporaryRecordsWithLock is the structure that allows to safely perform write
// changes to [TemporaryRecords] entries while the local entries database is
// locked.
type TemporaryRecordsWithLock struct {
	tr      *TemporaryRecords
	entries *localentries.UserDBLocked
}

// RegisterUser registers a temporary user with a unique UID.
//
// Returns the generated UID and a cleanup function that should be called to
// remove the temporary user once the user is added to the database.
func (l *TemporaryRecordsWithLock) RegisterUser(name string) (uid uint32, cleanup func(), err error) {
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
func (l *TemporaryRecordsWithLock) RegisterPreAuthUser(loginName string) (uid uint32, err error) {
	return l.tr.registerPreAuthUser(loginName)
}

// RegisterGroupForUser registers a temporary group with a unique GID in
// memory for the provided UID.
//
// Returns the generated GID and a cleanup function that should be called to
// remove the temporary group once the group was added to the database.
func (l *TemporaryRecordsWithLock) RegisterGroupForUser(uid uint32, name string) (gid uint32, cleanup func(), err error) {
	return l.tr.registerGroupForUser(uid, name)
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
func (r *TemporaryRecords) LockForChanges(entries *localentries.UserDBLocked) (tra *TemporaryRecordsWithLock) {
	entries.MustBeLocked()
	lockedRecords := &TemporaryRecordsWithLock{tr: r, entries: entries}

	for {
		if r.lockedRecords.CompareAndSwap(nil, lockedRecords) {
			break
		}

		// We've old locked records, let's just return these once we're sure!
		l := r.lockedRecords.Load()
		if l == nil {
			continue
		}
		return l
	}

	return lockedRecords
}

func (r *TemporaryRecords) registerUser(name string) (uid uint32, cleanup func(), err error) {
	defer decorate.OnError(&err, "failed to register user %q", name)

	unique, err := r.isUniqueSystemUserName(name)
	if err != nil {
		return 0, nil, err
	}
	if !unique {
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
		unique, err := r.isUniqueSystemID(user.UID)
		if err != nil {
			cleanup()
			return 0, nil, err
		}
		if !unique {
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

		unique, err := r.maybeTrackUniqueID(uid)
		if err != nil {
			return 0, nil, err
		}
		if !unique {
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

	unique, err := r.isUniqueSystemUserName(loginName)
	if err != nil {
		return 0, err
	}
	if !unique {
		log.Errorf(context.Background(), "User already exists on the system: %+v", loginName)
		return 0, fmt.Errorf("user %q already exists on the system", loginName)
	}

	for range maxIDGenerateIterations {
		uid, err := r.preAuthUserRecords.generatePreAuthUserID(loginName)
		if err != nil {
			return 0, err
		}

		unique, err := r.maybeTrackUniqueID(uid)
		if err != nil {
			return 0, err
		}
		if !unique {
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

func (r *TemporaryRecords) userEntries() ([]types.UserEntry, error) {
	l := r.lockedRecords.Load()
	if l == nil {
		testsdetection.MustBeTesting()

		le, cleanup, err := localentries.NewUserDBLocked()
		if err != nil {
			panic(fmt.Sprintf("Failed get lock local entries: %v", err))
		}
		defer func() {
			if err := cleanup(); err != nil {
				panic(fmt.Sprintf("Failed get cleanup locked entries: %v", err))
			}
		}()

		l = &TemporaryRecordsWithLock{entries: le}
	}

	return l.entries.GetUserEntries()
}

func (r *TemporaryRecords) groupEntries() ([]types.GroupEntry, error) {
	l := r.lockedRecords.Load()
	if l == nil {
		testsdetection.MustBeTesting()

		le, cleanup, err := localentries.NewUserDBLocked()
		if err != nil {
			panic(fmt.Sprintf("Failed get lock local entries: %v", err))
		}
		defer func() {
			if err := cleanup(); err != nil {
				panic(fmt.Sprintf("Failed get cleanup locked entries: %v", err))
			}
		}()

		l = &TemporaryRecordsWithLock{entries: le}
	}

	return l.entries.GetGroupEntries()
}

func (r *TemporaryRecords) registerGroupForUser(uid uint32, name string) (gid uint32, cleanup func(), err error) {
	defer decorate.OnError(&err, "failed to register group %q for user ID %d", name, uid)

	groupEntries, err := r.groupEntries()
	if err != nil {
		return 0, nil, err
	}

	if slices.ContainsFunc(groupEntries, func(g types.GroupEntry) bool {
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

		unique, err := r.maybeTrackUniqueID(gid)
		if err != nil {
			return 0, nil, err
		}
		if !unique {
			// If the GID is not unique, generate a new one in the next iteration.
			continue
		}

		r.addTemporaryGroup(gid, name)

		return gid, groupCleanup, nil
	}

	return 0, nil, fmt.Errorf("failed to find a valid GID after %d attempts",
		maxIDGenerateIterations)
}

func (r *TemporaryRecords) maybeTrackUniqueID(id uint32) (unique bool, err error) {
	defer func() {
		if !unique {
			log.Debugf(context.TODO(), "ID %d is not unique in this system", id)
		}
	}()

	unique, err = r.isUniqueSystemID(id)
	if err != nil {
		return false, err
	}
	if !unique {
		return false, nil
	}

	return r.idTracker.trackID(id), nil
}

func (r *TemporaryRecords) isUniqueSystemID(id uint32) (unique bool, err error) {
	userEntries, err := r.userEntries()
	if err != nil {
		return false, err
	}

	if idx := slices.IndexFunc(userEntries, func(p types.UserEntry) (found bool) {
		return p.UID == id
	}); idx != -1 {
		log.Debugf(context.Background(), "ID %d already in use by user %q",
			id, userEntries[idx].Name)
		return false, nil
	}

	groupEntries, err := r.groupEntries()
	if err != nil {
		return false, err
	}

	if idx := slices.IndexFunc(groupEntries, func(g types.GroupEntry) (found bool) {
		return g.GID == id
	}); idx != -1 {
		log.Debugf(context.Background(), "ID %d already in use by user %q",
			id, groupEntries[idx].Name)
		return false, nil
	}

	return true, nil
}

func (r *TemporaryRecords) isUniqueSystemUserName(name string) (unique bool, err error) {
	userEntries, err := r.userEntries()
	if err != nil {
		return false, err
	}

	return !slices.ContainsFunc(userEntries, func(p types.UserEntry) bool {
		return p.Name == name
	}), nil
}
