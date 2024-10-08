package users

import (
	"github.com/ubuntu/authd/internal/users/cache"
)

// UserInfo is the user information returned by the broker.
type UserInfo struct {
	Name  string
	UID   uint32
	Gecos string
	Dir   string
	Shell string

	Groups []GroupInfo
}

// GroupInfo is the group information returned by the broker.
type GroupInfo struct {
	Name string
	GID  *uint32
	UGID string
}

// UserEntry is the user information sent to the NSS service.
type UserEntry struct {
	Name  string
	UID   uint32
	GID   uint32
	Gecos string
	Dir   string
	Shell string
}

// ShadowEntry is the shadow information sent to the NSS service.
type ShadowEntry struct {
	Name           string
	LastPwdChange  int
	MaxPwdAge      int
	PwdWarnPeriod  int
	PwdInactivity  int
	MinPwdAge      int
	ExpirationDate int
}

// GroupEntry is the group information sent to the NSS service.
type GroupEntry struct {
	Name  string
	GID   uint32
	Users []string
}

// userEntryFromUserDB returns a UserEntry from a UserDB.
func userEntryFromUserDB(u cache.UserDB) UserEntry {
	return UserEntry{
		Name:  u.Name,
		UID:   u.UID,
		GID:   u.GID,
		Gecos: u.Gecos,
		Dir:   u.Dir,
		Shell: u.Shell,
	}
}

// shadowEntryFromUserDB returns a ShadowEntry from a UserDB.
func shadowEntryFromUserDB(u cache.UserDB) ShadowEntry {
	return ShadowEntry{
		Name:           u.Name,
		LastPwdChange:  u.LastPwdChange,
		MaxPwdAge:      u.MaxPwdAge,
		PwdWarnPeriod:  u.PwdWarnPeriod,
		PwdInactivity:  u.PwdInactivity,
		MinPwdAge:      u.MinPwdAge,
		ExpirationDate: u.ExpirationDate,
	}
}

// groupEntryFromGroupDB returns a GroupEntry from a GroupDB.
func groupEntryFromGroupDB(g cache.GroupDB) GroupEntry {
	return GroupEntry{
		Name:  g.Name,
		GID:   g.GID,
		Users: g.Users,
	}
}

// ErrNoDataFound is the error returned when no entry is found in the cache.
type ErrNoDataFound = cache.NoDataFoundError
