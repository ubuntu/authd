package users

import (
	"testing"
	"time"
)

// WaitCleanupRoutineDone gives the cleanup routine some time to run, then stops it and closes the cache.
func (m *Manager) WaitCleanupRoutineDone(t *testing.T) {
	t.Helper()

	// Give some time for the cleanup routine to run.
	time.Sleep(100 * time.Millisecond)

	// Wait for the cleanup routine to finish.
	close(m.quit)
	<-m.cleanupStopped

	// We need to close the cache only at the end of the test because we might need to assert on its content.
	t.Cleanup(func() { _ = m.cache.Close() })
}

// RequestClearDatabase exposes the private method for tests.
func (m *Manager) RequestClearDatabase() {
	m.requestClearDatabase()
}
