// Package tempentries provides a temporary user and group records.
package tempentries

import (
	"context"
	"errors"
	"fmt"

	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

// NoDataFoundError is the error returned when no entry is found in the cache.
type NoDataFoundError = cache.NoDataFoundError

// IDGenerator is the interface that must be implemented by the ID generator.
type IDGenerator interface {
	GenerateUID() (uint32, error)
	GenerateGID() (uint32, error)
}

// TemporaryRecords is the in-memory temporary user and group records.
type TemporaryRecords struct {
	*temporaryUserRecords
	*preAuthUserRecords
	*temporaryGroupRecords

	idGenerator IDGenerator
}

// NewTemporaryRecords creates a new TemporaryRecords.
func NewTemporaryRecords(idGenerator IDGenerator) *TemporaryRecords {
	return &TemporaryRecords{
		idGenerator:           idGenerator,
		temporaryUserRecords:  newTemporaryUserRecords(idGenerator),
		preAuthUserRecords:    newPreAuthUserRecords(idGenerator),
		temporaryGroupRecords: newTemporaryGroupRecords(idGenerator),
	}
}

// UserByID returns the user information for the given user ID.
func (r *TemporaryRecords) UserByID(uid uint32) (types.UserEntry, error) {
	user, err := r.temporaryUserRecords.userByID(uid)
	if errors.Is(err, NoDataFoundError{}) {
		user, err = r.preAuthUserRecords.userByID(uid)
	}
	return user, err
}

// UserByName returns the user information for the given user name.
func (r *TemporaryRecords) UserByName(name string) (types.UserEntry, error) {
	user, err := r.temporaryUserRecords.userByName(name)
	if errors.Is(err, NoDataFoundError{}) {
		user, err = r.preAuthUserRecords.userByName(name)
	}
	return user, err
}

// RegisterUser registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// Returns the generated UID and a cleanup function that should be called to remove the temporary user once the user was
// added to the database.
func (r *TemporaryRecords) RegisterUser(name string) (uid uint32, cleanup func(), err error) {
	r.temporaryUserRecords.registerMu.Lock()
	defer r.temporaryUserRecords.registerMu.Unlock()

	// Check if there is a temporary  user with the same login name.
	_, err = r.temporaryUserRecords.userByName(name)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return 0, nil, fmt.Errorf("could not check if temporary user %q already exists: %w", name, err)
	}
	if err == nil {
		return 0, nil, fmt.Errorf("user %q already exists", name)
	}

	// Check if there is a pre-auth user with the same login name.
	user, err := r.preAuthUserRecords.userByLogin(name)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return 0, nil, fmt.Errorf("could not check if pre-auth user %q already exists: %w", name, err)
	}
	if err == nil {
		// There is a pre-auth user with the same login name. Now that the user authenticated successfully, we can
		// replace the pre-auth user with a temporary user.
		return r.replacePreAuthUser(user, name)
	}

	// Generate a UID until we find a unique one
	for {
		uid, err = r.idGenerator.GenerateUID()
		if err != nil {
			return 0, nil, err
		}

		// To avoid races where a user with this UID is created by some NSS source after we checked, we register this
		// UID in our NSS handler and then check if another user with the same UID exists in the system. This way we
		// can guarantee that the UID is unique, under the assumption that other NSS sources don't add users with a UID
		// that we already registered (if they do, there's nothing we can do about it).
		var tmpID string
		tmpID, cleanup, err = r.temporaryUserRecords.addTemporaryUser(uid, name)
		if err != nil {
			return 0, nil, fmt.Errorf("could not add temporary user record: %w", err)
		}

		unique, err := r.temporaryUserRecords.uniqueNameAndUID(name, uid, tmpID)
		if err != nil {
			err = fmt.Errorf("checking UID and name uniqueness: %w", err)
			cleanup()
			return 0, nil, err
		}
		if unique {
			break
		}

		// If the UID is not unique, remove the temporary user and generate a new one in the next iteration.
		cleanup()
	}

	log.Debugf(context.Background(), "Added temporary record for user %q with UID %d", name, uid)
	return uid, cleanup, nil
}

// replacePreAuthUser replaces a pre-auth user with a temporary user with the same name and UID.
func (r *TemporaryRecords) replacePreAuthUser(user types.UserEntry, name string) (uid uint32, cleanup func(), err error) {
	var tmpID string
	tmpID, cleanup, err = r.addTemporaryUser(user.UID, name)
	if err != nil {
		return 0, nil, fmt.Errorf("could not add temporary user record: %w", err)
	}

	// Remove the pre-auth user from the pre-auth user records.
	r.deletePreAuthUser(user.UID)

	// Check if the UID and name are unique.
	unique, err := r.temporaryUserRecords.uniqueNameAndUID(name, user.UID, tmpID)
	if err != nil {
		err = fmt.Errorf("checking UID and name uniqueness: %w", err)
		cleanup()
		return 0, nil, err
	}
	if !unique {
		err = fmt.Errorf("UID (%d) or name (%q) from pre-auth user are not unique", user.UID, name)
		cleanup()
		return 0, nil, err
	}

	return user.UID, cleanup, nil
}
