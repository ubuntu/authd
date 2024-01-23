// Package users support all common action on the system for user handling.
package users

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/decorate"
)

const (
	// defaultEntryExpiration is the amount of time the user is allowed on the cache without authenticating.
	// It's equivalent to 6 months.
	defaultEntryExpiration = time.Hour * 24 * 30 * 6

	// defaultCleanupInterval is the interval upon which the cache will be cleaned of expired users.
	defaultCleanupInterval = time.Hour * 24
)

var (
	dirtyFlagName = ".corrupted"
)

// Manager is the manager for any user related operation.
type Manager struct {
	cache         *cache.Cache
	dirtyFlagPath string

	doClear        chan struct{}
	quit           chan struct{}
	cleanupStopped chan struct{}
}

type options struct {
	expirationDate  time.Time
	cleanOnNew      bool
	cleanupInterval time.Duration
	procDir         string // This is to force failure in tests.
}

// Option is a function that allows changing some of the default behaviors of the manager.
type Option func(*options)

// WithUserExpirationDate overrides the default time for when a user should be cleaned from the cache.
func WithUserExpirationDate(date time.Time) Option {
	return func(o *options) {
		o.expirationDate = date
	}
}

// NewManager creates a new user manager.
func NewManager(cacheDir string, args ...Option) (m *Manager, err error) {
	opts := &options{
		expirationDate:  time.Now().Add(-1 * defaultEntryExpiration),
		cleanOnNew:      true,
		cleanupInterval: defaultCleanupInterval,
		procDir:         "/proc/",
	}
	for _, arg := range args {
		arg(opts)
	}

	m = &Manager{
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

	if opts.cleanOnNew {
		if err := m.cleanExpiredUserData(opts); err != nil {
			slog.Warn(fmt.Sprintf("Could not fully clean expired user data: %v", err))
		}
	}
	m.startUserCleanupRoutine(cacheDir, opts)

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

	if len(u.Groups) == 0 {
		return fmt.Errorf("no group provided for user %s (%v)", u.Name, u.UID)
	}
	if u.Groups[0].GID == nil {
		return fmt.Errorf("no gid provided for default group %q", u.Groups[0].Name)
	}

	var groupContents []cache.GroupDB
	for _, g := range u.Groups {
		// System group: ignore here, not part of the cache.
		if g.GID == nil {
			continue
		}
		groupContents = append(groupContents, cache.NewGroupDB(g.Name, *g.GID, nil))
	}

	// Update user information in the cache.
	userDB := cache.NewUserDB(u.Name, u.UID, *u.Groups[0].GID, u.Gecos, u.Dir, u.Shell)
	if err := m.cache.UpdateUserEntry(userDB, groupContents); err != nil {
		return m.shouldClearDb(err)
	}

	return nil
}

// BrokerForUser returns the broker ID for the given user.
func (m *Manager) BrokerForUser(username string) (string, error) {
	brokerID, err := m.cache.BrokerForUser(username)
	if err != nil {
		return "", m.shouldClearDb(err)
	}
	return brokerID, err
}

// UpdateBrokerForUser updates the broker ID for the given user.
func (m *Manager) UpdateBrokerForUser(username, brokerID string) error {
	return m.shouldClearDb(m.cache.UpdateBrokerForUser(username, brokerID))
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

func (m *Manager) startUserCleanupRoutine(cacheDir string, opts *options) {
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

			case <-time.After(opts.cleanupInterval):
				func() {
					if err := m.cleanExpiredUserData(opts); err != nil {
						slog.Warn(fmt.Sprintf("Could not clean expired user data: %v", err))
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

	return nil
}

// cleanExpiredUserData cleans up the data belonging to expired users.
func (m *Manager) cleanExpiredUserData(opts *options) error {
	activeUsers, err := getActiveUsers(opts.procDir)
	if err != nil {
		return fmt.Errorf("could not get list of active users: %v", err)
	}

	if err := m.cache.CleanExpiredUsers(activeUsers, opts.expirationDate); err != nil {
		return fmt.Errorf("could not clean database of expired users: %v", err)
	}

	return nil
}

// getActiveUsers walks through procDir and returns a map with the usernames of the owners of all active processes.
func getActiveUsers(procDir string) (activeUsers map[string]struct{}, err error) {
	defer decorate.OnError(&err, "could not get list of active users")

	activeUsers = make(map[string]struct{})

	dirEntries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}

	for _, dirEntry := range dirEntries {
		// Checks if the dirEntry represents a process dir (i.e. /proc/<pid>/)
		if _, err := strconv.Atoi(dirEntry.Name()); err != nil {
			continue
		}

		info, err := dirEntry.Info()
		if err != nil {
			// If the file doesn't exist, it means the process is not running anymore so we can ignore it.
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}

		stats, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, fmt.Errorf("could not get ownership of file %q", info.Name())
		}

		u, err := user.LookupId(strconv.Itoa(int(stats.Uid)))
		if err != nil {
			// Possibly a ghost/orphaned UID - no reason to error out. Just warn the user and continue.
			slog.Warn(fmt.Sprintf("Could not map active user ID to an actual user: %v", err))
			continue
		}

		activeUsers[u.Name] = struct{}{}
	}
	return activeUsers, nil
}
