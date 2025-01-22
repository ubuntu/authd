package users

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/types"
)

// userEntryFromUserDB returns a UserEntry from a UserDB.
func userEntryFromUserDB(u db.UserDB) types.UserEntry {
	return types.UserEntry{
		Name:  u.Name,
		UID:   u.UID,
		GID:   u.GID,
		Gecos: u.Gecos,
		Dir:   u.Dir,
		Shell: u.Shell,
	}
}

// shadowEntryFromUserDB returns a ShadowEntry from a UserDB.
func shadowEntryFromUserDB(u db.UserDB) types.ShadowEntry {
	return types.ShadowEntry{
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
func groupEntryFromGroupDB(g db.GroupDB) types.GroupEntry {
	return types.GroupEntry{
		Name:  g.Name,
		GID:   g.GID,
		Users: g.Users,
	}
}

// NoDataFoundError is the error returned when no entry is found in the db.
type NoDataFoundError = db.NoDataFoundError
