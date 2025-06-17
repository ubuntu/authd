package tempentries

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

const (
	// MaxPreAuthUsers is the maximum number of pre-auth users that can be registered. If this limit is reached,
	// RegisterPreAuthUser will return an error and disable login for new users via SSH until authd is restarted.
	//
	// This value must be significantly smaller (less than half) than the number of UIDs which can be generated (as
	// defined by UID_MIN and UID_MAX in the config file), otherwise finding a unique UID by trial and error can take
	// too long.
	MaxPreAuthUsers = 4096

	// UserPrefix is the prefix used as login name by the pre-auth temporary users.
	UserPrefix = "authd-pre-auth-user"
)

type preAuthUser struct {
	// name is the generated random name of the pre-auth user (which is returned by UserByID).
	name string
	// loginName is the name of the user who the pre-auth user record is created for.
	loginName string
	uid       uint32
}

type preAuthUserRecords struct {
	idGenerator IDGenerator
	registerMu  sync.Mutex
	rwMu        sync.RWMutex
	users       map[uint32]preAuthUser
	uidByName   map[string]uint32
	uidByLogin  map[string]uint32
	numUsers    int
}

func newPreAuthUserRecords(idGenerator IDGenerator) *preAuthUserRecords {
	return &preAuthUserRecords{
		idGenerator: idGenerator,
		registerMu:  sync.Mutex{},
		rwMu:        sync.RWMutex{},
		users:       make(map[uint32]preAuthUser),
		uidByName:   make(map[string]uint32),
		uidByLogin:  make(map[string]uint32),
	}
}

// UserByID returns the user information for the given user ID.
func (r *preAuthUserRecords) userByID(uid uint32) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	user, ok := r.users[uid]
	if !ok {
		return types.UserEntry{}, db.NewUIDNotFoundError(uid)
	}

	return preAuthUserEntry(user), nil
}

// UserByName returns the user information for the given user name.
func (r *preAuthUserRecords) userByName(name string) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	uid, ok := r.uidByName[name]
	if !ok {
		return types.UserEntry{}, db.NewUserNotFoundError(name)
	}

	return r.userByID(uid)
}

func (r *preAuthUserRecords) userByLogin(loginName string) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	uid, ok := r.uidByLogin[loginName]
	if !ok {
		return types.UserEntry{}, db.NewUserNotFoundError(loginName)
	}

	return r.userByID(uid)
}

func preAuthUserEntry(user preAuthUser) types.UserEntry {
	return types.UserEntry{
		Name: user.name,
		UID:  user.uid,
		// The UID is also the GID of the user private group (see https://wiki.debian.org/UserPrivateGroups#UPGs)
		GID:   user.uid,
		Gecos: user.loginName,
		Dir:   "/nonexistent",
		Shell: "/usr/sbin/nologin",
	}
}

// RegisterPreAuthUser registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// The temporary user record is removed when UpdateUser is called with the same username.
//
// This method is called when a user logs in for the first time via SSH, in which case sshd checks if the user exists on
// the system (before authentication), and denies the login if the user does not exist. We pretend that the user exists
// by creating this temporary user record, which is converted into a permanent user record when UpdateUser is called
// after the user authenticated successfully.
//
// Returns the generated UID.
func (r *preAuthUserRecords) RegisterPreAuthUser(loginName string) (uid uint32, err error) {
	// To mitigate DoS attacks, we limit the length of the name to 256 characters.
	if len(loginName) > 256 {
		return 0, errors.New("username is too long (max 256 characters)")
	}

	r.registerMu.Lock()
	defer r.registerMu.Unlock()

	if r.numUsers >= MaxPreAuthUsers {
		return 0, errors.New("maximum number of pre-auth users reached, login for new users via SSH is disabled until authd is restarted")
	}

	// Check if there is already a pre-auth user for that name
	user, err := r.userByLogin(loginName)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return 0, fmt.Errorf("could not check if pre-auth user %q already exists: %w", loginName, err)
	}
	if err == nil {
		// A pre-auth user is already registered for this name, so we return the already generated UID.
		return user.UID, nil
	}

	// Generate a UID until we find a unique one
	for {
		uid, err := r.idGenerator.GenerateUID()
		if err != nil {
			return 0, err
		}

		// To avoid races where a user with this UID is created by some NSS source after we checked, we register this
		// UID in our NSS handler and then check if another user with the same UID exists in the system. This way we
		// can guarantee that the UID is unique, under the assumption that other NSS sources don't add users with a UID
		// that we already registered (if they do, there's nothing we can do about it).
		tmpName, cleanup, err := r.addPreAuthUser(uid, loginName)
		if err != nil {
			return 0, fmt.Errorf("could not add pre-auth user record: %w", err)
		}

		unique, err := r.isUniqueUID(uid, tmpName)
		if err != nil {
			cleanup()
			return 0, fmt.Errorf("could not check if UID %d is unique: %w", uid, err)
		}
		if unique {
			log.Debugf(context.Background(), "Added temporary record for user %q with UID %d", loginName, uid)
			return uid, nil
		}

		// If the UID is not unique, remove the temporary user and generate a new one in the next iteration.
		cleanup()
	}
}

// isUniqueUID returns true if the given UID is unique in the system. It returns false if the UID is already assigned to
// a user by any NSS source (except the given temporary user).
func (r *preAuthUserRecords) isUniqueUID(uid uint32, tmpName string) (bool, error) {
	entries, err := localentries.GetPasswdEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.UID == uid && entry.Name != tmpName {
			return false, nil
		}
	}

	groupEntries, err := localentries.GetGroupEntries()
	if err != nil {
		return false, fmt.Errorf("failed to get group entries: %w", err)
	}
	for _, group := range groupEntries {
		if group.GID == uid {
			// A group with the same ID already exists, so we can't use that ID as the GID of the temporary user
			log.Debugf(context.Background(), "ID %d already in use by group %q", uid, group.Name)
			return false, fmt.Errorf("group with GID %d already exists", uid)
		}
	}

	return true, nil
}

// addPreAuthUser adds a temporary user with a random name and the given UID. We use a random name here to avoid
// creating user records with attacker-controlled names.
//
// It returns the generated name and a cleanup function to remove the temporary user record.
func (r *preAuthUserRecords) addPreAuthUser(uid uint32, loginName string) (name string, cleanup func(), err error) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	// Generate a 64 character (32 bytes in hex) random ID which we store in the name field of the temporary user
	// record to be able to identify it in isUniqueUID.
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	name = fmt.Sprintf("%s-%x", UserPrefix, bytes)

	user := preAuthUser{name: name, uid: uid, loginName: loginName}
	r.users[uid] = user
	r.uidByName[name] = uid
	r.uidByLogin[loginName] = uid
	r.numUsers++

	cleanup = func() { r.deletePreAuthUser(uid) }

	return name, cleanup, nil
}

// deletePreAuthUser deletes the temporary user with the given UID.
func (r *preAuthUserRecords) deletePreAuthUser(uid uint32) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	user, ok := r.users[uid]
	if !ok {
		// We ignore the case that the pre-auth user does not exist, because it can happen that the same user is
		// registered multiple times (because multiple SSH sessions are opened for the same user) and the cleanup
		// function is called multiple times.
		return
	}
	delete(r.users, uid)
	delete(r.uidByName, user.name)
	delete(r.uidByLogin, user.loginName)
	r.numUsers--
	log.Debugf(context.Background(), "Removed temporary record for user %q with UID %d", user.name, uid)
}
