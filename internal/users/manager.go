// Package users support all common action on the system for user handling.
package users

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/decorate"
)

const (
	maxPreAuthUsers = 65536
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

// Manager is the manager for any user related operation.
type Manager struct {
	cache                 *cache.Cache
	config                Config
	temporaryRecords      temporaryRecords
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
		temporaryRecords:      newTemporaryRecords(),
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

	// Protect writing to the temporary user/group maps with a mutex, to avoid a race where multiple calls to UpdateUser
	// could register the same UID/GID multiple times.
	m.temporaryRecords.mutex.Lock()
	defer m.temporaryRecords.mutex.Unlock()

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
		// The user does not exist, so we generate a unique UID for it. To avoid that a user with the same UID is
		// created by some other NSS source, this also registers a temporary user in our NSS handler. We remove that
		// temporary user before returning from this function, at which point the user is added to the database (so we
		// don't need the temporary user anymore to keep the UID unique).
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
			// The group does not exist in the database, so we generate a unique GID for it. Similar to the registerUser
			// call above, this also registers a temporary group in our NSS handler. We remove that temporary group
			// before returning from this function, at which point the group is added to the database (so we don't need
			// the temporary group anymore to keep the GID unique).
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
	if err != nil {
		// We explicitly don't check if the user is a temporary user here, because the temporary user records should
		// only reserve the UID, not the username (which is randomly generated).
		return UserEntry{}, err
	}
	return userEntryFromUserDB(usr), nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid uint32) (UserEntry, error) {
	usr, err := m.cache.UserByID(uid)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		m.temporaryRecords.mutex.RLock()
		defer m.temporaryRecords.mutex.RUnlock()
		if user, ok := m.temporaryRecords.users[uid]; ok {
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
		for gid, name := range m.temporaryRecords.groups {
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
		if name, ok := m.temporaryRecords.groups[gid]; ok {
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
