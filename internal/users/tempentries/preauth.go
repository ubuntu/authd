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

	// MaxPreAuthUserNameLength is the maximum length of the pre-auth user name.
	MaxPreAuthUserNameLength = 256

	// UserPrefix is the prefix used as login name by the pre-auth temporary users.
	UserPrefix = "authd-pre-auth-user"
)

// NoDataFoundError is the error returned when no entry is found in the database.
type NoDataFoundError = db.NoDataFoundError

// IDGenerator is the interface that must be implemented by the ID generator.
type IDGenerator interface {
	GenerateUID() (uint32, error)
	GenerateGID() (uint32, error)
}

type preAuthUser struct {
	// name is the generated random name of the pre-auth user (which is returned by UserByID).
	name string
	// loginName is the name of the user who the pre-auth user record is created for.
	loginName string
	uid       uint32
}

type PreAuthUserRecords struct {
	idGenerator IDGenerator
	rwMu        sync.RWMutex
	users       map[uint32]preAuthUser
	uidByName   map[string]uint32
	uidByLogin  map[string]uint32
}

func NewPreAuthUserRecords(idGenerator IDGenerator) *PreAuthUserRecords {
	return &PreAuthUserRecords{
		idGenerator: idGenerator,
		users:       make(map[uint32]preAuthUser),
		uidByName:   make(map[string]uint32),
		uidByLogin:  make(map[string]uint32),
	}
}

// UserByID returns the user information for the given user ID.
func (r *PreAuthUserRecords) UserByID(uid uint32) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	return r.userByIDWithoutLock(uid)
}

func (r *PreAuthUserRecords) userByIDWithoutLock(uid uint32) (types.UserEntry, error) {
	user, ok := r.users[uid]
	if !ok {
		return types.UserEntry{}, db.NewUIDNotFoundError(uid)
	}

	return preAuthUserEntry(user), nil
}

// UserByName returns the user information for the given user name.
func (r *PreAuthUserRecords) UserByName(name string) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	uid, ok := r.uidByName[name]
	if !ok {
		return types.UserEntry{}, db.NewUserNotFoundError(name)
	}

	return r.userByIDWithoutLock(uid)
}

func (r *PreAuthUserRecords) UserByLogin(loginName string) (types.UserEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	uid, ok := r.uidByLogin[loginName]
	if !ok {
		return types.UserEntry{}, db.NewUserNotFoundError(loginName)
	}

	return r.userByIDWithoutLock(uid)
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
func (r *PreAuthUserRecords) RegisterPreAuthUser(loginName string) (uid uint32, err error) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	// To mitigate DoS attacks, we limit the length of the name to 256 characters.
	if len(loginName) > MaxPreAuthUserNameLength {
		return 0, fmt.Errorf("username is too long (maximum %d characters): %q", MaxPreAuthUserNameLength, loginName)
	}

	if len(r.users) >= MaxPreAuthUsers {
		return 0, errors.New("maximum number of pre-auth users reached, login for new users via SSH is disabled until authd is restarted")
	}

	// Check if there is already a pre-auth user for that name
	if uid, ok := r.uidByLogin[loginName]; ok {
		return uid, nil
	}

	// Check if the user already exists on the system (e.g. in /etc/passwd).
	_, err = localentries.GetPasswdByName(loginName)
	if err != nil && !errors.Is(err, localentries.ErrUserNotFound) {
		return 0, fmt.Errorf("could not check if user %q exists on the system: %w", loginName, err)
	}
	if err == nil {
		// The user already exists on the system, so we cannot create a new user with the same name.
		return 0, fmt.Errorf("user %q already exists on the system (but not in the authd database)", loginName)
	}

	// Generate a unique UID for the pre-auth user.
	uid, err = r.idGenerator.GenerateUID()
	if err != nil {
		return 0, fmt.Errorf("failed to generate unique UID for pre-auth user: %w", err)
	}

	var name string
	for {
		// Generate a 64 character (32 bytes in hex) random ID which we store in the
		// name field of the temporary user record to be able to identify it.
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return 0, fmt.Errorf("failed to generate random name: %w", err)
		}
		name = fmt.Sprintf("%s-%x", UserPrefix, bytes)

		if _, ok := r.uidByName[name]; ok {
			log.Debugf(context.Background(), "Generated user %q was not unique", name)
			continue
		}

		break
	}

	log.Debugf(context.Background(),
		"Added temporary record for user %q with UID %d as %q", loginName, uid, name)

	user := preAuthUser{name: name, uid: uid, loginName: loginName}
	r.users[uid] = user
	r.uidByName[name] = uid
	r.uidByLogin[loginName] = uid

	return uid, nil
}

// deletePreAuthUser deletes the temporary user with the given UID.
//
// This must be called with the mutex locked.
func (r *PreAuthUserRecords) deletePreAuthUser(uid uint32) bool {
	user, ok := r.users[uid]
	if !ok {
		// We ignore the case that the pre-auth user does not exist, because it can happen that the same user is
		// registered multiple times (because multiple SSH sessions are opened for the same user) and the cleanup
		// function is called multiple times.
		return false
	}
	delete(r.users, uid)
	delete(r.uidByName, user.name)
	delete(r.uidByLogin, user.loginName)
	log.Debugf(context.Background(), "Removed temporary record for user %q with UID %d", user.name, uid)
	return true
}
