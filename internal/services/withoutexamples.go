//go:build !withexamplebroker

package services

// Stop stops the underlying cache only in production code.
func (m *Manager) Stop() error {
	return m.stop()
}
