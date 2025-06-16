package tempentries

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

type userRecord struct {
	name  string
	uid   uint32
	gecos string
}

type temporaryUserRecords struct {
	idGenerator IDGenerator
	registerMu  sync.Mutex
	rwMu        sync.RWMutex
	users       map[uint32]userRecord
	uidByName   map[string]uint32
}

func newTemporaryUserRecords(idGenerator IDGenerator) *temporaryUserRecords {
	return &temporaryUserRecords{
		idGenerator: idGenerator,
		registerMu:  sync.Mutex{},
		rwMu:        sync.RWMutex{},
		users:       make(map[uint32]userRecord),
		uidByName:   make(map[string]uint32),
	}
}

// UserByID returns the user information for the given user ID.
func (r *temporaryUserRecords) userByID(uid uint32) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	user, ok := r.users[uid]
	if !ok {
		return types.UserEntry{}, NoDataFoundError{}
	}

	return userEntry(user), nil
}

// UserByName returns the user information for the given user name.
func (r *temporaryUserRecords) userByName(name string) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	uid, ok := r.uidByName[name]
	if !ok {
		return types.UserEntry{}, NoDataFoundError{}
	}

	return r.userByID(uid)
}

func userEntry(user userRecord) types.UserEntry {
	return types.UserEntry{
		Name: user.name,
		UID:  user.uid,
		// The UID is also the GID of the user private group (see https://wiki.debian.org/UserPrivateGroups#UPGs)
		GID:   user.uid,
		Gecos: user.gecos,
		Dir:   "/nonexistent",
		Shell: "/usr/sbin/nologin",
	}
}

// uniqueNameAndUID returns true if the given UID is unique in the system. It returns false if the UID is already assigned to
// a user by any NSS source (except the given temporary user).
func (r *temporaryUserRecords) uniqueNameAndUID(name string, uid uint32, tmpID string) (bool, error) {
	entries, err := localentries.GetPasswdEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name == name && entry.UID != uid {
			// A user with the same name already exists, we can't register this temporary user.
			log.Debugf(context.Background(), "Name %q already in use by UID %d", name, entry.UID)
			return false, fmt.Errorf("user %q already exists", name)
		}

		if entry.UID == uid && entry.Gecos != tmpID {
			log.Debugf(context.Background(), "UID %d already in use by user %q, generating a new one", uid, entry.Name)
			return false, nil
		}
	}

	groupEntries, err := localentries.GetGroupEntries()
	if err != nil {
		return false, fmt.Errorf("failed to get group entries: %w", err)
	}
	for _, group := range groupEntries {
		if group.GID == uid {
			// A group with the same ID already exists, so we can't use that ID as the GID of the temporary user.
			log.Debugf(context.Background(), "ID %d already in use by group %q", uid, group.Name)
			return false, fmt.Errorf("group with GID %d already exists", uid)
		}
	}

	return true, nil
}

// addTemporaryUser adds a temporary user with a random name and the given UID. It returns the generated name.
// If the UID is already registered, it returns a errUserAlreadyExists.
func (r *temporaryUserRecords) addTemporaryUser(uid uint32, name string) (tmpID string, cleanup func(), err error) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	// Generate a 64 character (32 bytes in hex) random ID which we store in the gecos field of the temporary user
	// record to be able to identify it in isUniqueUID.
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	tmpID = fmt.Sprintf("authd-temp-user-%x", bytes)

	r.users[uid] = userRecord{name: name, uid: uid, gecos: tmpID}
	r.uidByName[name] = uid

	cleanup = func() { r.deleteTemporaryUser(uid) }

	return tmpID, cleanup, nil
}

// deleteTemporaryUser deletes the temporary user with the given UID.
func (r *temporaryUserRecords) deleteTemporaryUser(uid uint32) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	user, ok := r.users[uid]
	if !ok {
		log.Warningf(context.Background(), "Can't delete temporary user with UID %d, it does not exist", uid)
		return
	}
	delete(r.users, uid)
	delete(r.uidByName, user.name)

	log.Debugf(context.Background(), "Removed temporary record for user %q with UID %d", user.name, uid)
}
