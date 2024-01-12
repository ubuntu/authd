// Package newusers is a transitional package that will replaces the users one.
package newusers

import (
	"fmt"
	"time"

	"github.com/ubuntu/authd/internal/newusers/cache"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
)

// Manager is the manager for any user related operation.
type Manager struct {
	cache *cache.Cache
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
	cache *cache.Cache
}

// Option is a function that allows changing some of the default behaviors of the manager.
type Option func(*options)

// WithCache sets the cache to use for the manager.
func WithCache(c *cache.Cache) Option {
	return func(o *options) {
		o.cache = c
	}
}

// NewManager creates a new user manager.
func NewManager(cacheDir string, args ...Option) (*Manager, error) {
	opts := &options{}
	for _, arg := range args {
		arg(opts)
	}

	if opts.cache != nil {
		return &Manager{cache: opts.cache}, nil
	}

	c, err := cache.New(cacheDir)
	if err != nil {
		return nil, err
	}

	return &Manager{cache: c}, nil
}

// Stop closes the underlying cache.
func (m *Manager) Stop() error {
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
		return err
	}

	return nil
}

// BrokerForUser returns the broker ID for the given user.
func (m *Manager) BrokerForUser(username string) (string, error) {
	return m.cache.BrokerForUser(username)
}

// UpdateBrokerForUser updates the broker ID for the given user.
func (m *Manager) UpdateBrokerForUser(username, brokerID string) error {
	return m.cache.UpdateBrokerForUser(username, brokerID)
}

// UserByName returns the user information for the given user name.
func (m *Manager) UserByName(username string) (cache.UserPasswdShadow, error) {
	return m.cache.UserByName(username)
}

// UserByID returns the user information for the given user ID.
func (m *Manager) UserByID(uid int) (cache.UserPasswdShadow, error) {
	return m.cache.UserByID(uid)
}

// AllUsers returns all users.
func (m *Manager) AllUsers() ([]cache.UserPasswdShadow, error) {
	return m.cache.AllUsers()
}

// GroupByName returns the group information for the given group name.
func (m *Manager) GroupByName(groupname string) (cache.Group, error) {
	return m.cache.GroupByName(groupname)
}

// GroupByID returns the group information for the given group ID.
func (m *Manager) GroupByID(gid int) (cache.Group, error) {
	return m.cache.GroupByID(gid)
}

// AllGroups returns all groups.
func (m *Manager) AllGroups() ([]cache.Group, error) {
	return m.cache.AllGroups()
}
