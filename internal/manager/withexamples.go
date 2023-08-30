//go:build withexamplebroker

package manager

// Stop calls the brokerManager function that stops and cleans the examplebroker files.
func (m *Manager) Stop() {
	m.brokerManager.Stop()
}
