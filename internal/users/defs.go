package users

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/types"
)

// userEntryFromUserRow returns a UserEntry from a UserRow.
func userEntryFromUserRow(u db.UserRow) types.UserEntry {
	return types.UserEntry{
		Name:  u.Name,
		UID:   u.UID,
		GID:   u.GID,
		Gecos: u.Gecos,
		Dir:   u.Dir,
		Shell: u.Shell,
	}
}

// shadowEntryFromUserRow returns a ShadowEntry from a UserRow.
func shadowEntryFromUserRow(u db.UserRow) types.ShadowEntry {
	return types.ShadowEntry{
		Name:           u.Name,
		LastPwdChange:  -1,
		MaxPwdAge:      -1,
		PwdWarnPeriod:  -1,
		PwdInactivity:  -1,
		MinPwdAge:      -1,
		ExpirationDate: -1,
	}
}

// groupEntryFromGroupWithMembers returns a GroupEntry from a GroupRow.
func groupEntryFromGroupWithMembers(g db.GroupWithMembers) types.GroupEntry {
	return types.GroupEntry{
		Name:  g.Name,
		GID:   g.GID,
		Users: g.Users,
	}
}

// NoDataFoundError is the error returned when no entry is found in the db.
type NoDataFoundError = db.NoDataFoundError
