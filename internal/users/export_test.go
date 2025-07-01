package users

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/tempentries"
)

func (m *Manager) PreAuthUserRecords() *tempentries.PreAuthUserRecords {
	return m.preAuthRecords
}

func (m *Manager) DB() *db.Manager {
	return m.db
}
