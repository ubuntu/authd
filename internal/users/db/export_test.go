package db

// Path exposes the path to the database file for testing.
func (m *Manager) Path() string {
	return m.path
}
