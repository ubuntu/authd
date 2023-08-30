//go:build !withexamplebroker

package manager

// Stop is a no-op in production code.
func (m *Manager) Stop() {}
