// Package users support all common action on the system for user handling.
package users

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/localgroups"
	"github.com/ubuntu/decorate"
)

var (
	dirtyFlagName = ".corrupted"
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
	cache         *cache.Cache
	config        Config
	dirtyFlagPath string

	doClear        chan struct{}
	quit           chan struct{}
	cleanupStopped chan struct{}
}

// NewManager creates a new user manager.
func NewManager(config Config, cacheDir string) (m *Manager, err error) {
	m = &Manager{
		config:         config,
		dirtyFlagPath:  filepath.Join(cacheDir, dirtyFlagName),
		doClear:        make(chan struct{}),
		quit:           make(chan struct{}),
		cleanupStopped: make(chan struct{}),
	}

	for i := 0; i < 2; i++ {
		c, err := cache.New(cacheDir)
		if err != nil && errors.Is(err, cache.ErrNeedsClearing) {
			if err := cache.RemoveDb(cacheDir); err != nil {
				return nil, fmt.Errorf("could not clear database: %v", err)
			}
			if err := localgroups.Clean(); err != nil {
				slog.Warn(fmt.Sprintf("Could not clean local groups: %v", err))
			}
			continue
		} else if err != nil {
			return nil, err
		}
		m.cache = c
		break
	}

	if m.isMarkedCorrupted() {
		if err := m.clear(cacheDir); err != nil {
			return nil, fmt.Errorf("could not clear corrupted data: %v", err)
		}
	}

	m.startUserCleanupRoutine(cacheDir)

	return m, nil
}

// Stop closes the underlying cache.
func (m *Manager) Stop() error {
	close(m.quit)
	<-m.cleanupStopped
	return m.cache.Close()
}

// UpdateUser updates the user information in the cache.
func (m *Manager) UpdateUser(u UserInfo) (err error) {
	defer decorate.OnError(&err, "failed to update user %q", u.Name)

	if u.Name == "" {
		return errors.New("empty username")
	}

	// Generate the UID of the user unless a UID is already set (only the case in tests).
	if u.UID == 0 {
		u.UID = m.GenerateUID(u.Name)
	}

	// Prepend the user private group
	u.Groups = append([]GroupInfo{{Name: u.Name, UGID: u.Name}}, u.Groups...)

	// Generate the GIDs of the user groups
	for i := range u.Groups {
		if u.Groups[i].UGID != "" {
			gidv := m.GenerateGID(u.Groups[i].UGID)
			u.Groups[i].GID = &gidv
		}
	}

	var groupContents []cache.GroupDB
	var localGroups []string
	for _, g := range u.Groups {
		if g.Name == "" {
			return fmt.Errorf("empty group name for user %q", u.Name)
		}

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
		return m.shouldClearDb(err)
	}

	// Update local groups.
	if err := localgroups.Update(u.Name, localGroups); err != nil {
		return errors.Join(err, m.shouldClearDb(m.cache.DeleteUser(u.UID)))
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
		return "", m.shouldClearDb(err)
	}

	return brokerID, nil
}

// UpdateBrokerForUser updates the broker ID for the given user.
func (m *Manager) UpdateBrokerForUser(username, brokerID string) error {
	if err := m.cache.UpdateBrokerForUser(username, brokerID); err != nil {
		return m.shouldClearDb(err)
	}

	return nil
}

// UserByName returns the user information for the given user name.
func (m *Manager) UserByName(username string) (UserEntry, error) {
	usr, err := m.cache.UserByName(username)
	if err != nil {
		return UserEntry{}, m.shouldClearDb(err)
	}
	return userEntryFromUserDB(usr), nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid int) (UserEntry, error) {
	usr, err := m.cache.UserByID(uid)
	if err != nil {
		return UserEntry{}, m.shouldClearDb(err)
	}
	return userEntryFromUserDB(usr), nil
}

// AllUsers returns all users.
func (m *Manager) AllUsers() ([]UserEntry, error) {
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, m.shouldClearDb(err)
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
		return GroupEntry{}, m.shouldClearDb(err)
	}
	return groupEntryFromGroupDB(grp), nil
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid int) (GroupEntry, error) {
	grp, err := m.cache.GroupByID(gid)
	if err != nil {
		return GroupEntry{}, m.shouldClearDb(err)
	}
	return groupEntryFromGroupDB(grp), nil
}

// AllGroups returns all groups.
func (m *Manager) AllGroups() ([]GroupEntry, error) {
	grps, err := m.cache.AllGroups()
	if err != nil {
		return nil, m.shouldClearDb(err)
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
		return ShadowEntry{}, m.shouldClearDb(err)
	}
	return shadowEntryFromUserDB(usr), nil
}

// AllShadows returns all shadow entries.
func (m *Manager) AllShadows() ([]ShadowEntry, error) {
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, m.shouldClearDb(err)
	}

	var shadowEntries []ShadowEntry
	for _, usr := range usrs {
		shadowEntries = append(shadowEntries, shadowEntryFromUserDB(usr))
	}
	return shadowEntries, err
}

// shouldClearDb checks the error and requests a database clearing if needed.
func (m *Manager) shouldClearDb(err error) error {
	if errors.Is(err, cache.ErrNeedsClearing) {
		m.requestClearDatabase()
	}
	return err
}

// requestClearDatabase ask for the clean goroutine to clear up the database.
// If we already have a pending request, do not block on it.
// TODO: improve behavior when cleanup is already running
// (either remove the dangling dirty file or queue the cleanup request).
func (m *Manager) requestClearDatabase() {
	if err := m.markCorrupted(); err != nil {
		slog.Warn(fmt.Sprintf("Could not mark database as dirty: %v", err))
	}
	select {
	case m.doClear <- struct{}{}:
	case <-time.After(10 * time.Millisecond): // Let the time for the cleanup goroutine for the initial start.
	}
}

func (m *Manager) startUserCleanupRoutine(cacheDir string) {
	cleanupRoutineStarted := make(chan struct{})
	go func() {
		defer close(m.cleanupStopped)
		close(cleanupRoutineStarted)
		for {
			select {
			case <-m.doClear:
				func() {
					if err := m.clear(cacheDir); err != nil {
						slog.Warn(fmt.Sprintf("Could not clear corrupted data: %v", err))
					}
				}()

			case <-m.quit:
				return
			}
		}
	}()
	<-cleanupRoutineStarted
}

// isMarkedCorrupted checks if the database is marked as corrupted.
func (m *Manager) isMarkedCorrupted() bool {
	_, err := os.Stat(m.dirtyFlagPath)
	return err == nil
}

// markCorrupted writes a dirty flag in the cache directory to mark the database as corrupted.
func (m *Manager) markCorrupted() error {
	if m.isMarkedCorrupted() {
		return nil
	}
	return os.WriteFile(m.dirtyFlagPath, nil, 0600)
}

// clear clears the corrupted database and rebuilds it.
func (m *Manager) clear(cacheDir string) error {
	if err := m.cache.Clear(cacheDir); err != nil {
		return fmt.Errorf("could not clear corrupted data: %v", err)
	}
	if err := os.Remove(m.dirtyFlagPath); err != nil {
		slog.Warn(fmt.Sprintf("Could not remove dirty flag file: %v", err))
	}

	if err := localgroups.Clean(); err != nil {
		return fmt.Errorf("could not clean local groups: %v", err)
	}

	return nil
}

// GenerateUID deterministically generates an ID between from the given string, ignoring case,
// in the range [UIDMin, UIDMax]. The generated ID is *not* guaranteed to be unique.
func (m *Manager) GenerateUID(str string) int {
	return generateID(str, m.config.UIDMin, m.config.UIDMax)
}

// GenerateGID deterministically generates an ID between from the given string, ignoring case,
// in the range [GIDMin, GIDMax]. The generated ID is *not* guaranteed to be unique.
func (m *Manager) GenerateGID(str string) int {
	return generateID(str, m.config.GIDMin, m.config.GIDMax)
}

func generateID(str string, minID, maxID uint32) int {
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

	return int(number)
}
