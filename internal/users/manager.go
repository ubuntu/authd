// Package users support all common action on the system for user handling.
package users

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	"github.com/ubuntu/authd/internal/users/localentries"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
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
	db             *db.Manager
	config         Config
	idGenerator    tempentries.IDGenerator
	preAuthRecords *tempentries.PreAuthUserRecords
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
		idGenerator:    opts.idGenerator,
		preAuthRecords: tempentries.NewPreAuthUserRecords(opts.idGenerator),
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

	log.Debugf(context.TODO(), "Updating user %q", u.Name)

	// authd uses lowercase usernames
	u.Name = strings.ToLower(u.Name)

	if u.Name == "" {
		return errors.New("empty username")
	}

	var uid uint32
	ctx := context.Background()

	// Check if the user already exists in the database
	oldUser, err := m.db.UserByName(u.Name)
	if err != nil && !errors.Is(err, db.NoDataFoundError{}) {
		return fmt.Errorf("could not get user %q: %w", u.Name, err)
	}
	if errors.Is(err, db.NoDataFoundError{}) {
		ctx, err = userslocking.WithUserDBLock(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire userdb lock: %w", err)
		}
		defer func() { err = errors.Join(err, userslocking.GetUserDBLock(ctx).Unlock()) }()

		// Check if the user already exists on the system (e.g. in /etc/passwd).
		_, err := localentries.GetPasswdByName(u.Name)
		if err != nil && !errors.Is(err, localentries.ErrUserNotFound) {
			return fmt.Errorf("could not check if user %q exists on the system: %w", u.Name, err)
		}
		if err == nil {
			// The user already exists on the system, so we cannot create a new user with the same name.
			return fmt.Errorf("user %q already exists on the system (but not in the authd database)", u.Name)
		}

		uid, err = m.idGenerator.GenerateUID()
		if err != nil {
			return fmt.Errorf("could not generate UID for user %q: %w", u.Name, err)
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
		for _, g := range newGroups {
			gid, err := m.idGenerator.GenerateGID()
			if err != nil {
				return fmt.Errorf("could not generate GID for group %q: %v", g.Name, err)
			}

			groupRows = append(groupRows, db.NewGroupRow(g.Name, gid, g.UGID))
		}
	}

	oldLocalGroups, err := m.db.UserLocalGroups(uid)
	if err != nil && !errors.Is(err, db.NoDataFoundError{}) {
		return err
	}

	// Update user information in the db.
	userPrivateGroup := groupRows[0]
	userRow := db.NewUserRow(u.Name, uid, userPrivateGroup.GID, u.Gecos, u.Dir, u.Shell)
	err = m.db.UpdateUserEntry(userRow, groupRows, localGroups)
	if err != nil {
		return err
	}

	// Update local groups.
	if err := updateLocalGroups(ctx, u.Name, localGroups, oldLocalGroups); err != nil {
		return err
	}

	if err = checkHomeDirOwnership(userRow.Dir, userRow.UID, userRow.GID); err != nil {
		log.Warningf(context.Background(), "Failed to check home directory ownership: %v", err)
	}

	return nil
}

// updateLocalGroups updates the groups of a user, adding them to the groups in newGroups
// which they are not already part of, and removing them from the groups in oldGroups
// which are not in newGroups. It locks the system's user database to prevent concurrent modifications.
func updateLocalGroups(ctx context.Context, username string, newGroups []string, oldGroups []string) (err error) {
	if len(newGroups) == 0 && len(oldGroups) == 0 {
		return nil
	}

	lock := userslocking.GetUserDBLock(ctx)
	if lock == nil {
		ctx, err = userslocking.WithUserDBLock(ctx)
		defer func() { err = errors.Join(err, userslocking.GetUserDBLock(ctx).Unlock()) }()
	}

	groupManager := localentries.NewGroupManager()
	return groupManager.Update(username, newGroups, oldGroups)
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
	if errors.Is(err, db.NoDataFoundError{}) {
		// Check if the user is a temporary user.
		return m.preAuthRecords.UserByName(username)
	}
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

// RegisterUserPreAuth registers a temporary user with a unique UID in our NSS handler (in memory, not in the database).
//
// The temporary user record is removed when UpdateUser is called with the same username.
func (m *Manager) RegisterUserPreAuth(name string) (uid uint32, err error) {
	defer decorate.OnError(&err, "failed to register pre-auth user %q", name)

	if userRow, err := m.db.UserByName(name); err == nil {
		return userRow.UID, nil
	}

	userdbLock := userslocking.NewUserDBLock()
	if err := userdbLock.Lock(); err != nil {
		return 0, fmt.Errorf("failed to acquire userdb lock: %w", err)
	}
	defer func() { err = errors.Join(err, userdbLock.Unlock()) }()

	return m.preAuthRecords.RegisterPreAuthUser(name)
}
