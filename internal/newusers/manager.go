// Package newusers is a transitional package that will replaces the users one.
package newusers

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/ubuntu/authd/internal/newusers/cache"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
)

const (
	// defaultEntryExpiration is the amount of time the user is allowed on the cache without authenticating.
	// It's equivalent to 6 months.
	defaultEntryExpiration = time.Hour * 24 * 30 * 6

	// defaultCleanupInterval is the interval upon which the cache will be cleaned of expired users.
	defaultCleanupInterval = time.Hour * 24
)

// Manager is the manager for any user related operation.
type Manager struct {
	cache          *cache.Cache
	doClear        chan struct{}
	quit           chan struct{}
	cleanupStopped chan struct{}
}

// UserInfo is the user information returned by the broker.
type UserInfo struct {
	Name  string
	UID   int
	Gecos string
	Dir   string
	Shell string

	Groups []GroupInfo
}

// GroupInfo is the group information returned by the broker.
type GroupInfo struct {
	Name string
	GID  *int
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
		doClear:        make(chan struct{}),
		quit:           make(chan struct{}),
		cleanupStopped: make(chan struct{}),
	}
	c, err := cache.New(cacheDir)
	if err != nil {
		return nil, err
	}
	m.cache = c

	if opts.cleanOnNew {
		if activeUsers, err := getActiveUsers(opts.procDir); err != nil {
			slog.Warn(fmt.Sprintf("Could not get list of active users: %v", err))
		} else if err := m.cache.CleanExpiredUsers(activeUsers, opts.expirationDate); err != nil {
			slog.Warn(fmt.Sprintf("Could not clean database: %v", err))
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
func (m *Manager) UpdateUser(u users.UserInfo) (err error) {
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
		groupContents = append(groupContents, cache.GroupDB{
			Name: g.Name,
			GID:  *g.GID,
		})
	}

	// Update user information in the cache.
	userDB := cache.UserDB{
		UserPasswdShadow: cache.UserPasswdShadow{
			Name:           u.Name,
			UID:            u.UID,
			GID:            *u.Groups[0].GID,
			Gecos:          u.Gecos,
			Dir:            u.Dir,
			Shell:          u.Shell,
			LastPwdChange:  -1,
			MaxPwdAge:      -1,
			PwdWarnPeriod:  -1,
			PwdInactivity:  -1,
			MinPwdAge:      -1,
			ExpirationDate: -1,
		},
		LastLogin: time.Now(),
	}
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
func (m *Manager) UserByName(username string) (cache.UserPasswdShadow, error) {
	usr, err := m.cache.UserByName(username)
	if err != nil {
		return cache.UserPasswdShadow{}, m.evaluateError(err)
	}
	return usr, nil
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid int) (cache.UserPasswdShadow, error) {
	usr, err := m.cache.UserByID(uid)
	if err != nil {
		return cache.UserPasswdShadow{}, m.evaluateError(err)
	}
	return usr, nil
}

// AllUsers returns all users.
func (m *Manager) AllUsers() ([]cache.UserPasswdShadow, error) {
	usrs, err := m.cache.AllUsers()
	if err != nil {
		return nil, m.shouldClearDb(err)
	}
	return usrs, err
}

// GroupByName returns the group information for the given group name.
func (m *Manager) GroupByName(groupname string) (cache.Group, error) {
	grp, err := m.cache.GroupByName(groupname)
	if err != nil {
		return cache.Group{}, m.evaluateError(err)
	}
	return grp, nil
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid int) (cache.Group, error) {
	grp, err := m.cache.GroupByID(gid)
	if err != nil {
		return cache.Group{}, m.evaluateError(err)
	}
	return grp, nil
}

// AllGroups returns all groups.
func (m *Manager) AllGroups() ([]cache.Group, error) {
	grps, err := m.cache.AllGroups()
	if err != nil {
		return nil, m.shouldClearDb(err)
	}
	return grps, nil
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
	if err := m.cache.MarkDatabaseAsDirty(); err != nil {
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
					m.cache.ClearAndRebuild(cacheDir)
				}()

			case <-time.After(opts.cleanupInterval):
				func() {
					activeUsers, err := getActiveUsers(opts.procDir)
					if err != nil {
						slog.Warn(fmt.Sprintf("Could not get list of active users: %v", err))
						return
					}

					if err := m.cache.CleanExpiredUsers(activeUsers, opts.expirationDate); err != nil {
						slog.Warn(fmt.Sprintf("Could not clean database: %v", err))
					}
				}()

			case <-m.quit:
				return
			}
		}
	}()
	<-cleanupRoutineStarted
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
