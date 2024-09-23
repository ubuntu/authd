package users

import (
	"github.com/ubuntu/authd/internal/users/tempentries"
)

func (m *Manager) TemporaryRecords() *tempentries.TemporaryRecords {
	return m.temporaryRecords
}
