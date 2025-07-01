// Package users support all common action on the system for user handling.
package users

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/tempentries"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/decorate"
)

// Config is the configuration for the user manager.
type Config struct {
	UIDMin uint32 `mapstructure:"uid_min" yaml:"uid_min"`
	UIDMax uint32 `mapstructure:"uid_max" yaml:"uid_max"`
	GIDMin uint32 `mapstructure:"gid_min" yaml:"gid_min"`
	GIDMax uint32 `mapstructure:"gid_max" yaml:"gid_max"`
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
	// userManagementMu must be used to protect all the operations in which we
	// do users registration to the DB, to ensure that concurrent goroutines may
	// not falsify the checks we are performing (such as the users existence).
	userManagementMu sync.Mutex

	db             *db.Manager
	config         Config
	preAuthRecords *tempentries.PreAuthUserRecords
	idGenerator    tempentries.IDGenerator

	userEntries  []types.UserEntry
	groupEntries []types.GroupEntry
}

type options struct {
	idGenerator tempentries.IDGenerator
}

// Avoid to loop forever if we can't find an UID for the user, it's just better
// to fail after a limit is reached than hang or crash.
const maxIDGenerateIterations = 256

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
func NewManager(config Config, dbDir string, args ...Option) (m *Manager, err error) {
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
		config:         config,
		preAuthRecords: tempentries.NewPreAuthUserRecords(),
		idGenerator:    opts.idGenerator,
	}

	m.db, err = db.New(dbDir)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Stop closes the underlying db.
func (m *Manager) Stop() error {
	return m.db.Close()
}

// UpdateUser updates the user information in the db.
func (m *Manager) UpdateUser(u types.UserInfo) (err error) {
	defer decorate.OnError(&err, "failed to update user %q", u.Name)

	m.userManagementMu.Lock()
	defer m.userManagementMu.Unlock()

	log.Debugf(context.TODO(), "Updating user %q", u.Name)

	// authd uses lowercase usernames
	u.Name = strings.ToLower(u.Name)

	if u.Name == "" {
		return errors.New("empty username")
	}

	var uid uint32
	// Check if the user already exists in the database
	oldUser, err := m.db.UserByName(u.Name)
	if err != nil && !errors.Is(err, db.NoDataFoundError{}) {
		return fmt.Errorf("could not get user %q: %w", u.Name, err)
	}
	if errors.Is(err, db.NoDataFoundError{}) {
		preauthUID, cleanup, err := m.preAuthRecords.MaybeCompletePreauthUser(u.Name)
		if err != nil && !errors.Is(err, tempentries.NoDataFoundError{}) {
			return err
		}
		if preauthUID != 0 {
			uid = preauthUID
			defer cleanup()
		} else {
			unlockEntries, err := m.cacheLockedEntries()
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, unlockEntries()) }()

			if !m.isUniqueSystemUserName(u.Name) {
				log.Warningf(context.Background(), "Another user exists with name %q", u.Name)
				return fmt.Errorf("another system user exists with %q name", u.Name)
			}

			if uid, err = m.generateUniqueUID(); err != nil {
				return err
			}
			log.Debugf(context.Background(), "Using new UID %d for user %q", uid, u.Name)
		}
	} else {
		// The user already exists in the database, use the existing UID to avoid permission issues.
		uid = oldUser.UID
	}

	// Prepend the user private group
	u.Groups = append([]types.GroupInfo{{Name: u.Name, GID: &uid, UGID: u.Name}}, u.Groups...)

	var groupRows []db.GroupRow
	var localGroups []string
	var newGroups []types.GroupInfo
	for _, g := range u.Groups {
		if g.Name == "" {
			return fmt.Errorf("empty group name for user %q", u.Name)
		}

		if g.UGID == "" {
			// An empty UGID means that the group is local, i.e. it's not stored in the database but expected to be
			// already present in /etc/group.
			localGroups = append(localGroups, g.Name)
			continue
		}

		// authd groups are lowercase
		g.Name = strings.ToLower(g.Name)

		// It's not a local group, so before storing it in the database, check if a group with the same name already
		// exists.
		if err := m.checkGroupNameConflict(g.Name, g.UGID); err != nil {
			return err
		}

		// Check if the group already exists in the database
		oldGroup, err := m.findGroup(g)
		if err != nil && !errors.Is(err, db.NoDataFoundError{}) {
			// Unexpected error
			return err
		}
		if !errors.Is(err, db.NoDataFoundError{}) {
			// The group already exists in the database, use the existing GID to avoid permission issues.
			g.GID = &oldGroup.GID
		}

		if g.GID == nil {
			// The group does not exist in the database, so we generate a unique GID for it.
			newGroups = append(newGroups, g)
			continue
		}

		groupRows = append(groupRows, db.NewGroupRow(g.Name, *g.GID, g.UGID))
	}

	if len(newGroups) > 0 {
		unlockEntries, err := m.cacheLockedEntries()
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, unlockEntries()) }()

		for i, j := 0, 0; i < len(newGroups); j++ {
			g := &newGroups[i]
			if j >= maxIDGenerateIterations {
				return fmt.Errorf("failed to find a valid GID for %q after %d attempts",
					g.Name, maxIDGenerateIterations)
			}

			gid, err := m.generateUniqueGID()
			if err != nil {
				return err
			}
			if gid == uid {
				continue
			}
			if slices.ContainsFunc(newGroups, func(og types.GroupInfo) bool {
				return og.GID != nil && *og.GID == gid
			}) {
				continue
			}

			if !m.isUniqueSystemGroupName(g.Name) {
				log.Warningf(context.Background(), "Group %q already exists", g.Name)
				return fmt.Errorf("another system group exists with %q name", g.Name)
			}

			g.GID = &gid
			groupRows = append(groupRows, db.NewGroupRow(g.Name, *g.GID, g.UGID))
			log.Debugf(context.Background(), "Using new GID %d for group %q", gid, u.Name)
			i++
		}
	}

	oldLocalGroups, err := m.db.UserLocalGroups(uid)
	if err != nil && !errors.Is(err, db.NoDataFoundError{}) {
		return err
	}

	// Update user information in the db.
	userPrivateGroup := groupRows[0]
	userRow := db.NewUserRow(u.Name, uid, userPrivateGroup.GID, u.Gecos, u.Dir, u.Shell)
	if err = m.db.UpdateUserEntry(userRow, groupRows, localGroups); err != nil {
		return err
	}

	// Update local groups.
	localEntries, unlockEntries, err := localentries.NewUserDBLocked()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlockEntries()) }()

	lockedGroups := localentries.GetGroupsWithLock(localEntries)
	if err := lockedGroups.Update(u.Name, localGroups, oldLocalGroups); err != nil {
		return err
	}

	if err = checkHomeDirOwnership(userRow.Dir, userRow.UID, userRow.GID); err != nil {
		log.Warningf(context.Background(), "Failed to check home directory ownership: %v", err)
	}

	return nil
}

// checkGroupNameConflict checks if a group with the given name already exists.
// If it does, it checks if it has the same UGID.
func (m *Manager) checkGroupNameConflict(name string, ugid string) error {
	// First check in our database.
	existingGroup, err := m.db.GroupByName(name)
	if err != nil && !errors.Is(err, db.NoDataFoundError{}) {
		// Unexpected error
		return err
	}

	if errors.Is(err, db.NoDataFoundError{}) {
		// The group does not exist in the database, the check in the system
		// can be delayed to the registration point.
		return nil
	}

	// A group with that name already exists in the database, check if it has the same UGID.
	// Ignore it if the UGID of the existing group is empty, because we didn't store the UGID in 0.3.7 and earlier.
	if existingGroup.UGID == "" {
		return nil
	}
	if existingGroup.UGID != ugid {
		log.Errorf(context.Background(), "Group %q already exists in the database with UGID %q (expected %q)", name, existingGroup.UGID, ugid)
		return errors.New("found a different group with the same name in the database")
	}

	// The group exists in the database and has the same UGID, so we can proceed.
	return nil
}

func (m *Manager) findGroup(group types.GroupInfo) (oldGroup db.GroupRow, err error) {
	// Search by UGID first to support renaming groups
	oldGroup, err = m.db.GroupByUGID(group.UGID)
	if err == nil {
		return oldGroup, nil
	}
	if !errors.Is(err, db.NoDataFoundError{}) {
		// Unexpected error
		return oldGroup, err
	}

	// The group was not found by UGID. Search by name, because we didn't store the UGID in 0.3.7 and earlier.
	return m.db.GroupByName(group.Name)
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
	if oldUID != uid && oldGID != gid {
		log.Warningf(context.Background(), "Home directory %q is not owned by UID %d and GID %d. To fix this, run `sudo chown -R %d:%d %q`.", home, uid, gid, uid, gid, home)
		return nil
	}
	if oldUID != uid {
		log.Warningf(context.Background(), "Home directory %q is not owned by UID %d. To fix this, run `sudo chown -R --from=%d %d %q`.", home, oldUID, oldUID, uid, home)
	}
	if oldGID != gid {
		log.Warningf(context.Background(), "Home directory %q is not owned by GID %d. To fix this, run `sudo chown -R --from=:%d :%d %q`.", home, oldGID, oldGID, gid, home)
	}

	return nil
}

// BrokerForUser returns the broker ID for the given user.
func (m *Manager) BrokerForUser(username string) (string, error) {
	u, err := m.db.UserByName(username)
	if err != nil {
		return "", err
	}

	return u.BrokerID, nil
}

// UpdateBrokerForUser updates the broker ID for the given user.
func (m *Manager) UpdateBrokerForUser(username, brokerID string) error {
	if err := m.db.UpdateBrokerForUser(username, brokerID); err != nil {
		return err
	}

	return nil
}

// UserByName returns the user information for the given user name.
func (m *Manager) UserByName(username string) (types.UserEntry, error) {
	usr, err := m.db.UserByName(username)
	if err != nil {
		return types.UserEntry{}, err
	}
	return userEntryFromUserRow(usr), nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid uint32) (types.UserEntry, error) {
	usr, err := m.db.UserByID(uid)
	if errors.Is(err, db.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		return m.preAuthRecords.UserByID(uid)
	}
	if err != nil {
		return types.UserEntry{}, err
	}
	return userEntryFromUserRow(usr), nil
}

// AllUsers returns all users.
func (m *Manager) AllUsers() ([]types.UserEntry, error) {
	// We don't return temporary users here, because they are not interesting to the user and would clutter the output
	// of `getent passwd`. Other tools should check `getpwnam`/`getpwuid` to check for conflicts, like `useradd` does.
	usrs, err := m.db.AllUsers()
	if err != nil {
		return nil, err
	}

	var usrEntries []types.UserEntry
	for _, usr := range usrs {
		usrEntries = append(usrEntries, userEntryFromUserRow(usr))
	}
	return usrEntries, err
}

// GroupByName returns the group information for the given group name.
func (m *Manager) GroupByName(groupname string) (types.GroupEntry, error) {
	grp, err := m.db.GroupWithMembersByName(groupname)
	if err != nil {
		return types.GroupEntry{}, err
	}
	return groupEntryFromGroupWithMembers(grp), nil
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid uint32) (types.GroupEntry, error) {
	grp, err := m.db.GroupWithMembersByID(gid)
	if err != nil {
		return types.GroupEntry{}, err
	}
	return groupEntryFromGroupWithMembers(grp), nil
}

// AllGroups returns all groups.
func (m *Manager) AllGroups() ([]types.GroupEntry, error) {
	// Same as in AllUsers, we don't return temporary groups here.
	grps, err := m.db.AllGroupsWithMembers()
	if err != nil {
		return nil, err
	}

	var grpEntries []types.GroupEntry
	for _, grp := range grps {
		grpEntries = append(grpEntries, groupEntryFromGroupWithMembers(grp))
	}
	return grpEntries, nil
}

// ShadowByName returns the shadow information for the given user name.
func (m *Manager) ShadowByName(username string) (types.ShadowEntry, error) {
	usr, err := m.db.UserByName(username)
	if err != nil {
		return types.ShadowEntry{}, err
	}
	return shadowEntryFromUserRow(usr), nil
}

// AllShadows returns all shadow entries.
func (m *Manager) AllShadows() ([]types.ShadowEntry, error) {
	usrs, err := m.db.AllUsers()
	if err != nil {
		return nil, err
	}

	var shadowEntries []types.ShadowEntry
	for _, usr := range usrs {
		shadowEntries = append(shadowEntries, shadowEntryFromUserRow(usr))
	}
	return shadowEntries, err
}

func (m *Manager) generateUniqueID(generator func() (uint32, error)) (uint32, error) {
	for range maxIDGenerateIterations {
		uid, err := generator()
		if err != nil {
			return 0, err
		}

		var unique bool
		unique, err = m.isUniqueID(uid)
		if err != nil {
			return 0, err
		}
		if !unique {
			log.Debugf(context.Background(), "UID %d is not unique", uid)
			continue
		}

		return uid, nil
	}

	return 0, fmt.Errorf("Cannot find an unique ID")
}

func (m *Manager) generateUniqueUID() (uint32, error) {
	return m.generateUniqueID(m.idGenerator.GenerateUID)
}

func (m *Manager) generateUniqueGID() (uint32, error) {
	return m.generateUniqueID(m.idGenerator.GenerateGID)
}

func (m *Manager) isUniqueID(id uint32) (bool, error) {
	oldUser, err := m.UserByID(id)
	if err == nil {
		log.Debugf(context.TODO(), "Found duplicate user %v", oldUser)
		return false, nil
	}
	if !errors.Is(err, db.NoDataFoundError{}) {
		return false, err
	}

	oldGroup, err := m.GroupByID(id)
	if err == nil {
		log.Debugf(context.TODO(), "Found duplicate group %v", oldGroup)
		return false, nil
	}
	if !errors.Is(err, db.NoDataFoundError{}) {
		return false, err
	}

	return m.isUniqueSystemID(id)
}

func (m *Manager) isUniqueSystemID(id uint32) (unique bool, err error) {
	if m.userEntries == nil {
		panic("User entries have note ben set")
	}
	if m.groupEntries == nil {
		panic("Group entries have note ben set")
	}

	if idx := slices.IndexFunc(m.userEntries, func(p types.UserEntry) (found bool) {
		return p.UID == id
	}); idx != -1 {
		log.Debugf(context.Background(), "ID %d already in use by user %q",
			id, m.userEntries[idx].Name)
		return false, nil
	}

	if idx := slices.IndexFunc(m.groupEntries, func(g types.GroupEntry) (found bool) {
		return g.GID == id
	}); idx != -1 {
		log.Debugf(context.Background(), "ID %d already in use by user %q",
			id, m.groupEntries[idx].Name)
		return false, nil
	}

	return true, nil
}

func (m *Manager) cacheLockedEntries() (unlockEntries func() error, err error) {
	if m.userEntries != nil && m.groupEntries != nil {
		return func() error { return nil }, nil
	}

	localEntries, unlock, err := localentries.NewUserDBLocked()
	if err != nil {
		return nil, err
	}

	m.userEntries, err = localEntries.GetUserEntries()
	if err != nil {
		return nil, err
	}

	m.groupEntries, err = localEntries.GetGroupEntries()
	if err != nil {
		return nil, err
	}

	return func() error {
		m.userEntries = nil
		m.groupEntries = nil
		return unlock()
	}, nil
}

func (m *Manager) isUniqueSystemUserName(name string) (unique bool) {
	if m.userEntries == nil {
		panic("User entries are not set!")
	}
	return !slices.ContainsFunc(m.userEntries, func(g types.UserEntry) bool {
		return g.Name == name
	})
}

func (m *Manager) isUniqueSystemGroupName(name string) (unique bool) {
	if m.groupEntries == nil {
		panic("Group entries are not set!")
	}
	return !slices.ContainsFunc(m.groupEntries, func(g types.GroupEntry) bool {
		return g.Name == name
	})
}

// RegisterUserPreAuth registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// The temporary user record is removed when UpdateUser is called with the same username.
func (m *Manager) RegisterUserPreAuth(name string) (uid uint32, err error) {
	defer decorate.OnError(&err, "failed to register pre-auth user %q", name)

	m.userManagementMu.Lock()
	defer m.userManagementMu.Unlock()

	if userRow, err := m.db.UserByName(name); err == nil {
		log.Debugf(context.Background(), "user %q already exists on the system", name)
		return userRow.UID, nil
	}

	user, err := m.preAuthRecords.UserByLogin(name)
	if err == nil {
		log.Debugf(context.Background(), "user %q already pre-authenticated", name)
		return user.UID, nil
	}
	if err != nil && !errors.Is(err, tempentries.NoDataFoundError{}) {
		return 0, err
	}

	unlockEntries, err := m.cacheLockedEntries()
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, unlockEntries()) }()

	if !m.isUniqueSystemUserName(name) {
		return 0, fmt.Errorf("another system user exists with %q name", name)
	}

	uid, err = m.generateUniqueUID()
	if err != nil {
		return 0, err
	}

	if err := m.preAuthRecords.RegisterPreAuthUser(name, uid); err != nil {
		return 0, err
	}

	log.Debugf(context.Background(), "Using new UID %d for temporary user %q", uid, name)
	return uid, nil
}
