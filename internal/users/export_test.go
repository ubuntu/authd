package users

import (
	"github.com/ubuntu/authd/internal/users/db"
)

func (m *Manager) DB() *db.Manager {
	return m.db
}
