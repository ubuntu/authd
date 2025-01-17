// Package users support all common action on the system for user handling.
package users

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"sync"
	"syscall"

	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/tempentries"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
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
	cache            *cache.Cache
	config           Config
	temporaryRecords *tempentries.TemporaryRecords
	updateUserMu     sync.Mutex
}

type options struct {
	idGenerator tempentries.IDGenerator
}

// Option is a function that allows changing some of the default behaviors of the manager.
type Option func(*options)

// WithIDGenerator makes the manager use a specific ID generator.
// This option is only useful in tests.
func WithIDGenerator(g tempentries.IDGenerator) Option {
	return func(o *options) {
		o.idGenerator = g
	}
}

// NewManager creates a new user manager.
func NewManager(config Config, cacheDir string, args ...Option) (m *Manager, err error) {
	log.Debugf(context.Background(), "Creating user manager with config: %+v", config)

	opts := &options{}
	for _, arg := range args {
		arg(opts)
	}

	if opts.idGenerator == nil {
		// Check that the ID ranges are valid.
		if config.UIDMin >= config.UIDMax {
			return nil, errors.New("UID_MIN must be less than UID_MAX")
		}
		if config.GIDMin >= config.GIDMax {
			return nil, errors.New("GID_MIN must be less than GID_MAX")
		}
		// Check that the number of possible UIDs is at least twice the number of possible pre-auth users.
		numUIDs := config.UIDMax - config.UIDMin
		minNumUIDs := uint32(tempentries.MaxPreAuthUsers * 2)
		if numUIDs < minNumUIDs {
			return nil, fmt.Errorf("UID range configured via UID_MIN and UID_MAX is too small (%d), must be at least %d", numUIDs, minNumUIDs)
		}

		opts.idGenerator = &idgenerator.IDGenerator{
			UIDMin: config.UIDMin,
			UIDMax: config.UIDMax,
			GIDMin: config.GIDMin,
			GIDMax: config.GIDMax,
		}
	}

	m = &Manager{
		config:           config,
		temporaryRecords: tempentries.NewTemporaryRecords(opts.idGenerator),
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
func (m *Manager) UpdateUser(u types.UserInfo) (err error) {
	defer decorate.OnError(&err, "failed to update user %q", u.Name)

	if u.Name == "" {
		return errors.New("empty username")
	}

	var uid uint32

	// Prevent a TOCTOU race condition between the check for existence in our database and the registration of the
	// temporary user/group records. This does not prevent a race condition where a user is created by some other NSS
	// source, but that is handled in the temporaryRecords.RegisterUser and temporaryRecords.RegisterGroup functions.
	m.updateUserMu.Lock()
	defer m.updateUserMu.Unlock()

	// Check if the user already exists in the database
	oldUser, err := m.cache.UserByName(u.Name)
	if err != nil && !errors.Is(err, cache.NoDataFoundError{}) {
		return fmt.Errorf("could not get user %q: %w", u.Name, err)
	}
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the user exists on the system
		existingUser, err := user.Lookup(u.Name)
		var unknownUserErr user.UnknownUserError
		if !errors.As(err, &unknownUserErr) {
			log.Errorf(context.Background(), "User already exists on the system: %+v", existingUser)
			return fmt.Errorf("user %q already exists on the system (but not in this authd instance)", u.Name)
		}

		// The user does not exist, so we generate a unique UID for it. To avoid that a user with the same UID is
		// created by some other NSS source, this also registers a temporary user in our NSS handler. We remove that
		// temporary user before returning from this function, at which point the user is added to the database (so we
		// don't need the temporary user anymore to keep the UID unique).
		var cleanup func()
		uid, cleanup, err = m.temporaryRecords.RegisterUser(u.Name)
		if err != nil {
			return fmt.Errorf("could not register user %q: %w", u.Name, err)
		}
		defer cleanup()
	} else {
		uid = oldUser.UID
	}

	// Prepend the user private group
	u.Groups = append([]types.GroupInfo{{Name: u.Name, UGID: u.Name}}, u.Groups...)

	var authdGroups []cache.GroupDB
	var localGroups []string
	for _, g := range u.Groups {
		if g.Name == "" {
			return fmt.Errorf("empty group name for user %q", u.Name)
		}

		if g.UGID == "" {
			// An empty UGID means that the group is local.
			localGroups = append(localGroups, g.Name)
			continue
		}

		// Check if the group already exists in the database
		oldGroup, err := m.findGroup(g)
		if err != nil && !errors.Is(err, cache.NoDataFoundError{}) {
			// Unexpected error
			return err
		}
		// Keep the old GID if the group already exists in the database, to avoid permission issues
		if err == nil {
			g.GID = &oldGroup.GID
		}

		if g.GID == nil {
			// The group does not exist in the database, so we generate a unique GID for it. Similar to the RegisterUser
			// call above, this also registers a temporary group in our NSS handler. We remove that temporary group
			// before returning from this function, at which point the group is added to the database (so we don't need
			// the temporary group anymore to keep the GID unique).
			gid, cleanup, err := m.temporaryRecords.RegisterGroup(g.Name)
			if err != nil {
				return fmt.Errorf("could not generate GID for group %q: %v", g.Name, err)
			}

			defer cleanup()

			g.GID = &gid
		}

		authdGroups = append(authdGroups, cache.NewGroupDB(g.Name, *g.GID, g.UGID, nil))
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

func (m *Manager) findGroup(group types.GroupInfo) (oldGroup cache.GroupDB, err error) {
	// Search by UGID first to support renaming groups
	oldGroup, err = m.cache.GroupByUGID(group.UGID)
	if err == nil {
		return oldGroup, nil
	}
	if !errors.Is(err, cache.NoDataFoundError{}) {
		// Unexpected error
		return oldGroup, err
	}

	// The group was not found by UGID. Search by name, because we didn't store the UGID in 0.3.7 and earlier.
	log.Debugf(context.Background(), "Group %q not found by UGID %q, trying lookup by name", group.Name, group.UGID)
	oldGroup, err = m.cache.GroupByName(group.Name)
	if err == nil && oldGroup.UGID != "" {
		// There is a group with the same name but a different UGID, which should not happen
		return oldGroup, fmt.Errorf("group %q already exists with UGID %q (expected %q)", group.Name, oldGroup.UGID, group.UGID)
	}
	return oldGroup, err
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
func (m *Manager) UserByName(username string) (types.UserEntry, error) {
	usr, err := m.cache.UserByName(username)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		return m.temporaryRecords.UserByName(username)
	}
	if err != nil {
		return types.UserEntry{}, err
	}
	return userEntryFromUserDB(usr), nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid uint32) (types.UserEntry, error) {
	usr, err := m.cache.UserByID(uid)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		return m.temporaryRecords.UserByID(uid)
	}
	if err != nil {
		return types.UserEntry{}, err
	}
	return userEntryFromUserDB(usr), nil
}

// AllUsers returns all users.
func (m *Manager) AllUsers() ([]types.UserEntry, error) {
	// TODO: I'm not sure if we should return temporary users here. On the one hand, they are usually not interesting to
	// the user and would clutter the output of `getent passwd`. On the other hand, it might be surprising that some
	// users are not returned by `getent passwd` and some apps might rely on all users being returned.
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, err
	}

	var usrEntries []types.UserEntry
	for _, usr := range usrs {
		usrEntries = append(usrEntries, userEntryFromUserDB(usr))
	}
	return usrEntries, err
}

// GroupByName returns the group information for the given group name.
func (m *Manager) GroupByName(groupname string) (types.GroupEntry, error) {
	grp, err := m.cache.GroupByName(groupname)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the group is a temporary group.
		return m.temporaryRecords.GroupByName(groupname)
	}
	if err != nil {
		return types.GroupEntry{}, err
	}
	return groupEntryFromGroupDB(grp), nil
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid uint32) (types.GroupEntry, error) {
	grp, err := m.cache.GroupByID(gid)
	if errors.Is(err, cache.NoDataFoundError{}) {
		// Check if the group is a temporary group.
		return m.temporaryRecords.GroupByID(gid)
	}
	if err != nil {
		return types.GroupEntry{}, err
	}
	return groupEntryFromGroupDB(grp), nil
}

// AllGroups returns all groups.
func (m *Manager) AllGroups() ([]types.GroupEntry, error) {
	// TODO: Same as for AllUsers, we might want to return temporary groups here.
	grps, err := m.cache.AllGroups()
	if err != nil {
		return nil, err
	}

	var grpEntries []types.GroupEntry
	for _, grp := range grps {
		grpEntries = append(grpEntries, groupEntryFromGroupDB(grp))
	}
	return grpEntries, nil
}

// ShadowByName returns the shadow information for the given user name.
func (m *Manager) ShadowByName(username string) (types.ShadowEntry, error) {
	usr, err := m.cache.UserByName(username)
	if err != nil {
		return types.ShadowEntry{}, err
	}
	return shadowEntryFromUserDB(usr), nil
}

// AllShadows returns all shadow entries.
func (m *Manager) AllShadows() ([]types.ShadowEntry, error) {
	// TODO: Even less sure if we should return temporary users here.
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, err
	}

	var shadowEntries []types.ShadowEntry
	for _, usr := range usrs {
		shadowEntries = append(shadowEntries, shadowEntryFromUserDB(usr))
	}
	return shadowEntries, err
}

// RegisterUserPreAuth registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// The temporary user record is removed when UpdateUser is called with the same username.
func (m *Manager) RegisterUserPreAuth(name string) (uint32, error) {
	return m.temporaryRecords.RegisterPreAuthUser(name)
}
