// Package users support all common action on the system for user handling.
package users

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"
	"syscall"

	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/decorate"
)

const (
	maxPreAuthUsers = 1024
)

// Config is the configuration for the user manager.
type Config struct {
	UIDMin uint32 `mapstructure:"uid_min"`
	UIDMax uint32 `mapstructure:"uid_max"`
	GIDMin uint32 `mapstructure:"gid_min"`
	GIDMax uint32 `mapstructure:"gid_max"`
}

// DefaultConfig is the default configuration for the user manager.
var DefaultConfig = Config{
	UIDMin: 1000000000,
	UIDMax: 1999999999,
	GIDMin: 1000000000,
	GIDMax: 1999999999,
}

type temporaryUser struct {
	name    string
	preAuth bool
}

// Manager is the manager for any user related operation.
type Manager struct {
	cache                 *cache.Cache
	config                Config
	temporaryEntriesMu    sync.Mutex
	numPreAuthUsers       int
	temporaryUsers        map[uint32]temporaryUser
	temporaryGroups       map[uint32]string
	uidsToGenerateInTests []uint32
	gidsToGenerateInTests []uint32
}

type options struct {
	uidsToGenerateInTests []uint32
	gidsToGenerateInTests []uint32
}

// Option is a function that allows changing some of the default behaviors of the manager.
type Option func(*options)

// WithUIDsToGenerateInTests makes the manager generate a specific UID in tests instead of a random one.
// Only use this option in tests.
func WithUIDsToGenerateInTests(ids []uint32) Option {
	return func(o *options) {
		o.uidsToGenerateInTests = ids
	}
}

// WithGIDsToGenerateInTests makes the manager generate a specific GID in tests instead of a random one.
// Only use this option in tests.
func WithGIDsToGenerateInTests(ids []uint32) Option {
	return func(o *options) {
		o.gidsToGenerateInTests = ids
	}
}

// NewManager creates a new user manager.
func NewManager(config Config, cacheDir string, args ...Option) (m *Manager, err error) {
	log.Debugf(context.Background(), "Creating user manager with config: %+v", config)

	opts := &options{}
	for _, arg := range args {
		arg(opts)
	}

	// Check that the ID ranges are valid.
	if config.UIDMin >= config.UIDMax {
		return nil, errors.New("UID_MIN must be less than UID_MAX")
	}
	if config.GIDMin >= config.GIDMax {
		return nil, errors.New("GID_MIN must be less than GID_MAX")
	}

	m = &Manager{
		config:                config,
		uidsToGenerateInTests: opts.uidsToGenerateInTests,
		gidsToGenerateInTests: opts.gidsToGenerateInTests,
		temporaryUsers:        make(map[uint32]temporaryUser),
		temporaryGroups:       make(map[uint32]string),
	}

	c, err := cache.New(cacheDir)
	if err != nil {
		return nil, err
	}
	m.cache = c

	return m, nil
}

// Stop closes the underlying cache.
func (m *Manager) Stop() error {
	return m.cache.Close()
}

// UpdateUser updates the user information in the cache.
func (m *Manager) UpdateUser(u UserInfo) (err error) {
	defer decorate.OnError(&err, "failed to update user %q", u.Name)

	m.temporaryEntriesMu.Lock()
	defer m.temporaryEntriesMu.Unlock()

	if u.Name == "" {
		return errors.New("empty username")
	}

	var uid uint32
	var oldGroups []cache.GroupDB

	// Check if the user already exists in the database
	oldUser, err := m.cache.UserByName(u.Name)
	if err != nil && !errors.Is(err, cache.NoDataFoundError{}) {
		return fmt.Errorf("could not get user %q: %w", u.Name, err)
	}
	if errors.Is(err, cache.NoDataFoundError{}) {
		// The user does not exist, so we register it with a unique UID.
		var cleanup func()
		uid, cleanup, err = m.registerUser(u.Name, false)
		if err != nil {
			return fmt.Errorf("could not register user %q: %w", u.Name, err)
		}

		defer cleanup()

		oldGroups = []cache.GroupDB{}
	} else {
		uid = oldUser.UID
		oldGroups, err = m.cache.UserGroups(oldUser.UID)
		if err != nil {
			return fmt.Errorf("could not get groups of user %q: %v", u.Name, err)
		}
	}

	// Prepend the user private group
	u.Groups = append([]GroupInfo{{Name: u.Name, UGID: u.Name}}, u.Groups...)

	var authdGroups []cache.GroupDB
	var localGroups []string
	for _, g := range u.Groups {
		if g.Name == "" {
			return fmt.Errorf("empty group name for user %q", u.Name)
		}

		if g.UGID == "" && g.GID == nil {
			// An empty UGID means that the group is local.
			localGroups = append(localGroups, g.Name)
			continue
		}

		// Check if the group exists in the database.
		for _, oldGroup := range oldGroups {
			if oldGroup.Name == g.Name {
				// The group already exists in the database, so we keep the GID.
				g.GID = &oldGroup.GID
			}
		}

		if g.GID == nil {
			// The group does not exist in the database, so we generate a new GID.
			gid, cleanup, err := m.registerGroup(g.Name)
			if err != nil {
				return fmt.Errorf("could not generate GID for group %q: %v", g.Name, err)
			}

			defer func() {
				cleanupErr := cleanup()
				if cleanupErr != nil {
					err = errors.Join(err, fmt.Errorf("could not remove temporary group %q: %v", g.Name, cleanupErr))
				}
			}()

			g.GID = &gid
		}

		authdGroups = append(authdGroups, cache.NewGroupDB(g.Name, *g.GID, nil))
	}

	oldLocalGroups, err := m.cache.UserLocalGroups(uid)
	if err != nil && !errors.Is(err, cache.NoDataFoundError{}) {
		return err
	}

	// Update user information in the cache.
	userDB := cache.NewUserDB(u.Name, uid, authdGroups[0].GID, u.Gecos, u.Dir, u.Shell)
	if err := m.cache.UpdateUserEntry(userDB, authdGroups, localGroups); err != nil {
		return err
	}

	// Update local groups.
	if err := localentries.Update(u.Name, localGroups, oldLocalGroups); err != nil {
		return errors.Join(err, m.cache.DeleteUser(uid))
	}

	if err = checkHomeDirOwnership(userDB.Dir, userDB.UID, userDB.GID); err != nil {
		return fmt.Errorf("failed to check home directory owner and group: %w", err)
	}

	return nil
}

// checkHomeDirOwnership checks if the home directory of the user is owned by the user and the user's group.
// If not, it logs a warning.
func checkHomeDirOwnership(home string, uid, gid uint32) error {
	fileInfo, err := os.Stat(home)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		// The home directory does not exist, so we don't need to check the owner.
		return nil
	}

	sys, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("failed to get file info")
	}
	oldUID, oldGID := sys.Uid, sys.Gid

	// Check if the home directory is owned by the user.
	if oldUID != uid {
		log.Warningf(context.Background(), "Home directory %q is not owned by UID %d. To fix this, run `sudo chown -R --from=%d %d %s`.", home, oldUID, oldUID, uid, home)
	}
	if oldGID != gid {
		log.Warningf(context.Background(), "Home directory %q is not owned by GID %d. To fix this, run `sudo chown -R --from=:%d :%d %s`.", home, oldGID, oldGID, gid, home)
	}

	return nil
}

// BrokerForUser returns the broker ID for the given user.
func (m *Manager) BrokerForUser(username string) (string, error) {
	brokerID, err := m.cache.BrokerForUser(username)
	// User not in cache.
	if err != nil && errors.Is(err, cache.NoDataFoundError{}) {
		return "", NoDataFoundError{}
	} else if err != nil {
		return "", err
	}

	return brokerID, nil
}

// UpdateBrokerForUser updates the broker ID for the given user.
func (m *Manager) UpdateBrokerForUser(username, brokerID string) error {
	if err := m.cache.UpdateBrokerForUser(username, brokerID); err != nil {
		return err
	}

	return nil
}

// UserByName returns the user information for the given user name.
func (m *Manager) UserByName(username string) (UserEntry, error) {
	usr, err := m.cache.UserByName(username)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		for uid, user := range m.temporaryUsers {
			if user.name == username {
				return UserEntry{Name: user.name, UID: uid}, nil
			}
		}
	}
	if err != nil {
		return UserEntry{}, err
	}
	return userEntryFromUserDB(usr), nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid uint32) (UserEntry, error) {
	usr, err := m.cache.UserByID(uid)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		if user, ok := m.temporaryUsers[uid]; ok {
			return UserEntry{Name: user.name, UID: uid}, nil
		}
	}
	if err != nil {
		return UserEntry{}, err
	}
	return userEntryFromUserDB(usr), nil
}

// AllUsers returns all users.
func (m *Manager) AllUsers() ([]UserEntry, error) {
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, err
	}

	var usrEntries []UserEntry
	for _, usr := range usrs {
		usrEntries = append(usrEntries, userEntryFromUserDB(usr))
	}
	return usrEntries, err
}

// GroupByName returns the group information for the given group name.
func (m *Manager) GroupByName(groupname string) (GroupEntry, error) {
	grp, err := m.cache.GroupByName(groupname)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the group is a temporary group.
		for gid, name := range m.temporaryGroups {
			if name == groupname {
				return GroupEntry{Name: name, GID: gid}, nil
			}
		}
	}
	if err != nil {
		return GroupEntry{}, err
	}
	return groupEntryFromGroupDB(grp), nil
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid uint32) (GroupEntry, error) {
	grp, err := m.cache.GroupByID(gid)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the group is a temporary group.
		if name, ok := m.temporaryGroups[gid]; ok {
			return GroupEntry{Name: name, GID: gid}, nil
		}
	}
	if err != nil {
		return GroupEntry{}, err
	}
	return groupEntryFromGroupDB(grp), nil
}

// AllGroups returns all groups.
func (m *Manager) AllGroups() ([]GroupEntry, error) {
	grps, err := m.cache.AllGroups()
	if err != nil {
		return nil, err
	}

	var grpEntries []GroupEntry
	for _, grp := range grps {
		grpEntries = append(grpEntries, groupEntryFromGroupDB(grp))
	}
	return grpEntries, nil
}

// ShadowByName returns the shadow information for the given user name.
func (m *Manager) ShadowByName(username string) (ShadowEntry, error) {
	usr, err := m.cache.UserByName(username)
	if err != nil {
		return ShadowEntry{}, err
	}
	return shadowEntryFromUserDB(usr), nil
}

// AllShadows returns all shadow entries.
func (m *Manager) AllShadows() ([]ShadowEntry, error) {
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, err
	}

	var shadowEntries []ShadowEntry
	for _, usr := range usrs {
		shadowEntries = append(shadowEntries, shadowEntryFromUserDB(usr))
	}
	return shadowEntries, err
}

// RegisterUserPreAuth registers a temporary user with the given name and a unique UID in our NSS handler (in memory,
// not in the database). It returns the generated UID or an error if the user could not be registered.
func (m *Manager) RegisterUserPreAuth(name string) (uint32, error) {
	m.temporaryEntriesMu.Lock()
	defer m.temporaryEntriesMu.Unlock()

	if m.numPreAuthUsers >= maxPreAuthUsers {
		return 0, errors.New("maximum number of pre-auth users reached, login for new users via SSH is disabled until authd is restarted")
	}

	uid, _, err := m.registerUser(name, true)
	if err != nil {
		return 0, fmt.Errorf("could not register user %q: %w", name, err)
	}

	m.numPreAuthUsers++

	return uid, nil
}

// registerUser registers a temporary user with the given name and a unique UID in our NSS handler (in memory, not in
// the database).
//
// The caller must lock m.temporaryEntriesMu for writing before calling this function.
//
// Returns the generated UID and a cleanup function that should be called to remove the temporary user once the user was
// added to the database.
func (m *Manager) registerUser(name string, preAuth bool) (uid uint32, cleanup func(), err error) {
	// Check if there is already a temporary user for that name
	for uid, user := range m.temporaryUsers {
		if user.name == name+"-preauth" {
			// A temporary user with the same name is already registered. To avoid that we generate multiple UIDs for
			// the same user, we return the already generated UID.
			cleanup = func() { m.deleteTemporaryUser(uid) }
			return uid, cleanup, nil
		}
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
		tmpName, cleanup, err = m.addTemporaryUser(uid, preAuth)
		if errors.Is(err, errUserAlreadyExists) {
			log.Debugf(context.Background(), "UID %d already in use, generating a new one", uid)
			continue
		}
		if err != nil {
			return 0, nil, fmt.Errorf("could not register temporary user: %w", err)
		}

		if unique := m.isUniqueUID(uid, tmpName); unique {
			if preAuth {
				// Rename the temporary user so that we can recognize it later.
				tmpName = name + "-preauth"
				m.temporaryUsers[uid] = temporaryUser{name: tmpName, preAuth: preAuth}
			}
			log.Debugf(context.Background(), "Registered temporary user %q with UID %d", tmpName, uid)
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
func (m *Manager) addTemporaryUser(uid uint32, preAuth bool) (name string, cleanup func(), err error) {
	// Check if the UID is already registered.
	_, err = m.UserByID(uid)
	if err == nil {
		return "", nil, errUserAlreadyExists
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

	m.temporaryUsers[uid] = temporaryUser{name: name, preAuth: preAuth}

	cleanup = func() { m.deleteTemporaryUser(uid) }

	return name, cleanup, nil
}

// deleteTemporaryUser deletes the temporary user with the given UID.
func (m *Manager) deleteTemporaryUser(uid uint32) {
	// We ignore the case that the temporary user does not exist, because it might happen that the same user is
	// registered multiple times (by UserPreCheck) and the cleanup function is called multiple times.
	delete(m.temporaryUsers, uid)
	log.Debugf(context.Background(), "Removed temporary user with UID %d", uid)
}

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

	m.temporaryGroups[gid] = name

	cleanup = func() error {
		return m.deleteTemporaryGroup(gid)
	}

	return name, cleanup, nil
}

func (m *Manager) deleteTemporaryGroup(gid uint32) error {
	_, ok := m.temporaryGroups[gid]
	if !ok {
		return fmt.Errorf("temporary group with GID %d does not exist", gid)
	}

	delete(m.temporaryGroups, gid)
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
