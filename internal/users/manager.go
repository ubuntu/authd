// Package users support all common action on the system for user handling.
package users

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/localgroups"
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
	cache  *cache.Cache
	config Config
}

// NewManager creates a new user manager.
func NewManager(config Config, cacheDir string) (m *Manager, err error) {
	slog.Debug(fmt.Sprintf("Creating user manager with config: %+v", config))

	// Check that the ID ranges are valid.
	if config.UIDMin >= config.UIDMax {
		return nil, errors.New("UID_MIN must be less than UID_MAX")
	}
	if config.GIDMin >= config.GIDMax {
		return nil, errors.New("GID_MIN must be less than GID_MAX")
	}

	m = &Manager{
		config: config,
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

	if u.Name == "" {
		return errors.New("empty username")
	}

	// Check if the user already exists in the database
	oldUser, err := m.cache.UserByName(u.Name)
	if err != nil && !errors.Is(err, cache.NoDataFoundError{}) {
		return err
	}
	// Keep the old UID if the user already exists in the database, to avoid permission issues with the user's home
	// directory and other files.
	if !errors.Is(err, cache.NoDataFoundError{}) {
		u.UID = oldUser.UID
	}

	// Generate the UID of the user unless a UID is already set.
	if u.UID == 0 {
		u.UID = m.GenerateUID(u.Name)
	}

	// Prepend the user private group
	u.Groups = append([]GroupInfo{{Name: u.Name, UGID: u.Name}}, u.Groups...)

	// Generate the GIDs of the user groups
	for i, g := range u.Groups {
		if g.Name == "" {
			return fmt.Errorf("empty group name for user %q", u.Name)
		}

		if g.UGID == "" {
			// An empty UGID means that the group is a local group, so we don't need to store a GID for it.
			continue
		}

		// Check if the group already exists in the database
		oldGroup, err := m.cache.GroupByName(g.Name)
		if err != nil && !errors.Is(err, cache.NoDataFoundError{}) {
			return err
		}
		// Keep the old GID if the group already exists in the database, to avoid permission issues
		if !errors.Is(err, cache.NoDataFoundError{}) {
			u.Groups[i].GID = &oldGroup.GID
		}

		// Generate the GID of the group unless a GID is already set.
		if u.Groups[i].GID == nil || *u.Groups[i].GID == 0 {
			gidv := m.GenerateGID(u.Groups[i].UGID)
			u.Groups[i].GID = &gidv
		}
	}

	var groupContents []cache.GroupDB
	var localGroups []string
	for _, g := range u.Groups {
		// Empty GID assume local group
		if g.GID == nil {
			localGroups = append(localGroups, g.Name)
			continue
		}
		groupContents = append(groupContents, cache.NewGroupDB(g.Name, *g.GID, nil))
	}

	// Update user information in the cache.
	userDB := cache.NewUserDB(u.Name, u.UID, *u.Groups[0].GID, u.Gecos, u.Dir, u.Shell)
	if err := m.cache.UpdateUserEntry(userDB, groupContents); err != nil {
		return err
	}

	// Update local groups.
	if err := localgroups.Update(u.Name, localGroups); err != nil {
		return errors.Join(err, m.cache.DeleteUser(u.UID))
	}

	return nil
}

// BrokerForUser returns the broker ID for the given user.
func (m *Manager) BrokerForUser(username string) (string, error) {
	brokerID, err := m.cache.BrokerForUser(username)
	// User not in cache.
	if err != nil && errors.Is(err, cache.NoDataFoundError{}) {
		return "", ErrNoDataFound{}
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
		return UserEntry{}, err
	}
	return userEntryFromUserDB(usr), nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid uint32) (UserEntry, error) {
	usr, err := m.cache.UserByID(uid)
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
	if err != nil {
		return GroupEntry{}, err
	}
	return groupEntryFromGroupDB(grp), nil
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid uint32) (GroupEntry, error) {
	grp, err := m.cache.GroupByID(gid)
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

// GenerateUID deterministically generates an ID between from the given string, ignoring case,
// in the range [UIDMin, UIDMax]. The generated ID is *not* guaranteed to be unique.
func (m *Manager) GenerateUID(str string) uint32 {
	return generateID(str, m.config.UIDMin, m.config.UIDMax)
}

// GenerateGID deterministically generates an ID between from the given string, ignoring case,
// in the range [GIDMin, GIDMax]. The generated ID is *not* guaranteed to be unique.
func (m *Manager) GenerateGID(str string) uint32 {
	return generateID(str, m.config.GIDMin, m.config.GIDMax)
}

func generateID(str string, minID, maxID uint32) uint32 {
	str = strings.ToLower(str)

	// Create a SHA-256 hash of the input string
	hash := sha256.Sum256([]byte(str))

	// Convert the first 4 bytes of the hash into an integer
	number := binary.BigEndian.Uint32(hash[:4]) % (maxID + 1)

	// Repeat hashing until we get a number in the desired range. This ensures that the generated IDs are uniformly
	// distributed in the range, opposed to a simple modulo operation.
	for number < minID {
		hash = sha256.Sum256(hash[:])
		number = binary.BigEndian.Uint32(hash[:4]) % (maxID + 1)
	}

	return number
}
