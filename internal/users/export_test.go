package users

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/tempentries"
)

func (m *Manager) TemporaryRecords() *tempentries.TemporaryRecords {
	return m.temporaryRecords
}

func (m *Manager) DB() *db.Manager {
	return m.db
}
