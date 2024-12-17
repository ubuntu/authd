package users

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/users/localentries"
)

type temporaryRecords struct {
	mutex            sync.Mutex
	numPreAuthUsers  int
	users            map[uint32]temporaryUser
	usersByLoginName map[string]uint32
	groups           map[uint32]string
}

func newTemporaryRecords() temporaryRecords {
	return temporaryRecords{
		users:            make(map[uint32]temporaryUser),
		usersByLoginName: make(map[string]uint32),
		groups:           make(map[uint32]string),
	}
}

type temporaryUser struct {
	// name is the generated random name of the temporary user (which is returned by UserByID).
	name string
	// loginName is the name of the user who the temporary user record is created for.
	loginName string
	preAuth   bool
}

// RegisterUserPreAuth registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// The temporary user record is removed when UpdateUser is called with the same username.
func (m *Manager) RegisterUserPreAuth(name string) (uint32, error) {
	m.temporaryRecords.mutex.Lock()
	defer m.temporaryRecords.mutex.Unlock()

	if m.temporaryRecords.numPreAuthUsers >= maxPreAuthUsers {
		return 0, errors.New("maximum number of pre-auth users reached, login for new users via SSH is disabled until authd is restarted")
	}

	uid, _, err := m.registerUser(name, true)
	if err != nil {
		return 0, fmt.Errorf("could not register user %q: %w", name, err)
	}

	m.temporaryRecords.numPreAuthUsers++

	return uid, nil
}

// registerUser registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// The caller must lock m.temporaryEntriesMu for writing before calling this function, to avoid that multiple parallel
// calls can register the same UID multiple times.
//
// Returns the generated UID and a cleanup function that should be called to remove the temporary user once the user was
// added to the database.
func (m *Manager) registerUser(name string, preAuth bool) (uid uint32, cleanup func(), err error) {
	// Check if there is already a temporary user for that name
	uid, ok := m.temporaryRecords.usersByLoginName[name]
	if ok {
		user := m.temporaryRecords.users[uid]
		if !user.preAuth {
			// This should never happen, non-preauth temporary users should be removed before the
			// m.temporaryEntriesMu lock is released and this function is called again.
			return 0, nil, fmt.Errorf("temporary record for user %q already exists", name)
		}

		// A pre-auth user is already registered for this name. To avoid that we generate multiple UIDs for the same
		// user, we return the already generated UID.
		cleanup = func() {
			m.deleteTemporaryUser(uid)
			m.temporaryRecords.numPreAuthUsers--
		}
		return uid, cleanup, nil
	}

	for {
		uid, err = m.GenerateUID()
		if err != nil {
			return 0, nil, err
		}

		// To avoid races where a user with this UID is created by some NSS source after we checked, we register this
		// UID in our NSS handler and then check if another user with the same UID exists in the system. This way we
		// can guarantee that the UID is unique, under the assumption that other NSS sources don't add users with a UID
		// that we already registered (if they do, there's nothing we can do about it).
		var tmpName string
		tmpName, cleanup, err = m.addTemporaryUser(uid, name, preAuth)
		if errors.Is(err, errUserAlreadyExists) {
			log.Debugf(context.Background(), "UID %d already in use, generating a new one", uid)
			continue
		}
		if err != nil {
			return 0, nil, fmt.Errorf("could not add temporary user record: %w", err)
		}

		if unique := m.isUniqueUID(uid, tmpName); unique {
			log.Debugf(context.Background(), "Added temporary record for user %q with UID %d", name, uid)
			break
		}

		// If the UID is not unique, remove the temporary user and generate a new one in the next iteration.
		cleanup()
	}

	return uid, cleanup, nil
}

// isUniqueUID returns true if the given UID is unique in the system. It returns false if the UID is already assigned to
// a user by any NSS source (except the given temporary user).
func (m *Manager) isUniqueUID(uid uint32, tmpName string) bool {
	for _, entry := range localentries.GetPasswdEntries() {
		if entry.UID == uid && entry.Name != tmpName {
			log.Debugf(context.Background(), "UID %d already in use by user %q, generating a new one", uid, entry.Name)
			return false
		}
	}

	return true
}

var errUserAlreadyExists = errors.New("user already exists")

// addTemporaryUser adds a temporary user with a random name and the given UID. It returns the generated name.
// If the UID is already registered, it returns a errUserAlreadyExists.
func (m *Manager) addTemporaryUser(uid uint32, loginName string, preAuth bool) (name string, cleanup func(), err error) {
	// Check if the UID is already registered.
	_, err = m.UserByID(uid)
	if err == nil {
		return "", nil, errUserAlreadyExists
	}
	if !errors.Is(err, NoDataFoundError{}) {
		return "", nil, err
	}

	// Generate a 64 character (32 bytes in hex) random name.
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	if preAuth {
		name = fmt.Sprintf("authd-preauth-user-%x", bytes)
	} else {
		name = fmt.Sprintf("authd-temp-user-%x", bytes)
	}

	m.temporaryRecords.users[uid] = temporaryUser{name: name, loginName: loginName, preAuth: preAuth}
	m.temporaryRecords.usersByLoginName[loginName] = uid

	cleanup = func() { m.deleteTemporaryUser(uid) }

	return name, cleanup, nil
}

// deleteTemporaryUser deletes the temporary user with the given UID.
func (m *Manager) deleteTemporaryUser(uid uint32) {
	user, ok := m.temporaryRecords.users[uid]
	if !ok {
		// We ignore the case that the temporary user does not exist, because it might happen that the same user is
		// registered multiple times (by UserPreCheck) and the cleanup function is called multiple times.
		return
	}
	delete(m.temporaryRecords.users, uid)
	delete(m.temporaryRecords.usersByLoginName, user.loginName)
	log.Debugf(context.Background(), "Removed temporary record for user %q with UID %d", user.loginName, uid)
}

// registerGroup registers a temporary group with a unique GID in our NSS handler (in memory, not in the database).
//
// The caller must lock m.temporaryEntriesMu for writing before calling this function, to avoid that multiple parallel
// calls can register the same GID multiple times.
//
// Returns the generated GID and a cleanup function that should be called to remove the temporary group once the group
// was added to the database.
func (m *Manager) registerGroup(name string) (gid uint32, cleanup func() error, err error) {
	for {
		gid, err = m.GenerateGID()
		if err != nil {
			return 0, nil, err
		}

		// To avoid races where a group with this GID is created by some NSS source after we checked, we register this
		// GID in our NSS handler and then check if another group with the same GID exists in the system. This way we
		// can guarantee that the GID is unique, under the assumption that other NSS sources don't add groups with a GID
		// that we already registered (if they do, there's nothing we can do about it).
		var tmpName string
		tmpName, cleanup, err = m.addTemporaryGroup(gid)
		if errors.Is(err, errGroupAlreadyExists) {
			log.Debugf(context.Background(), "GID %d already in use, generating a new one", gid)
			continue
		}
		if err != nil {
			return 0, nil, fmt.Errorf("could not register temporary group: %w", err)
		}

		if unique := m.isUniqueGID(gid, tmpName); unique {
			break
		}

		// If the GID is not unique, remove the temporary group and generate a new one in the next iteration.
		if err := cleanup(); err != nil {
			return 0, nil, fmt.Errorf("could not remove temporary group %q: %w", tmpName, err)
		}
	}

	log.Debugf(context.Background(), "Registered group %q with GID %d", name, gid)

	return gid, cleanup, nil
}

func (m *Manager) isUniqueGID(gid uint32, tmpName string) bool {
	for _, entry := range localentries.GetGroupEntries() {
		if entry.GID == gid && entry.Name != tmpName {
			log.Debugf(context.Background(), "GID %d already in use by group %q, generating a new one", gid, entry.Name)
			return false
		}
	}

	return true
}

var errGroupAlreadyExists = errors.New("group already exists")

func (m *Manager) addTemporaryGroup(gid uint32) (name string, cleanup func() error, err error) {
	// Check if the GID is already registered.
	_, err = m.GroupByID(gid)
	if err == nil {
		return "", nil, errGroupAlreadyExists
	}
	if !errors.Is(err, NoDataFoundError{}) {
		return "", nil, err
	}

	// Generate a 32 character (16 bytes in hex) random name.
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	name = fmt.Sprintf("%x", bytes)

	m.temporaryRecords.groups[gid] = name

	cleanup = func() error {
		return m.deleteTemporaryGroup(gid)
	}

	return name, cleanup, nil
}

func (m *Manager) deleteTemporaryGroup(gid uint32) error {
	_, ok := m.temporaryRecords.groups[gid]
	if !ok {
		return fmt.Errorf("temporary group with GID %d does not exist", gid)
	}

	delete(m.temporaryRecords.groups, gid)
	return nil
}

// GenerateUID generates a random UID in the configured range.
func (m *Manager) GenerateUID() (uint32, error) {
	if m.uidsToGenerateInTests != nil {
		if len(m.uidsToGenerateInTests) == 0 {
			return 0, fmt.Errorf("no more UIDs to generate in tests")
		}

		uid := m.uidsToGenerateInTests[0]
		m.uidsToGenerateInTests = m.uidsToGenerateInTests[1:]
		return uid, nil
	}

	return generateID(m.config.UIDMin, m.config.UIDMax)
}

// GenerateGID generates a random GID in the configured range.
func (m *Manager) GenerateGID() (uint32, error) {
	if m.gidsToGenerateInTests != nil {
		if len(m.gidsToGenerateInTests) == 0 {
			return 0, fmt.Errorf("no more GIDs to generate in tests")
		}

		gid := m.gidsToGenerateInTests[0]
		m.gidsToGenerateInTests = m.gidsToGenerateInTests[1:]
		return gid, nil
	}

	return generateID(m.config.GIDMin, m.config.GIDMax)
}

func generateID(minID, maxID uint32) (uint32, error) {
	diff := int64(maxID - minID)
	// Generate a cryptographically secure random number between 0 and diff
	nBig, err := rand.Int(rand.Reader, big.NewInt(diff+1))
	if err != nil {
		return 0, err
	}

	// Add minID to get a number in the desired range
	//nolint:gosec // This conversion is safe because we only generate UIDs which ware positive and smaller than uint32.
	return uint32(nBig.Int64()) + minID, nil
}
