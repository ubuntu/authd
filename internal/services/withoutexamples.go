//go:build !withexamplebroker

package services

// Stop is a no-op in production code.
func (m *Manager) Stop() {}
