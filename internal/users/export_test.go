package users

import (
	"testing"
	"time"
)

// WithoutCleaningCacheOnNew skips the cleaning of old users when creating the cache.
func WithoutCleaningCacheOnNew() Option {
	return func(o *options) {
		o.cleanOnNew = false
	}
}

// WithCacheCleanupInterval overrides the default interval for cleaning the cache of expired users.
func WithCacheCleanupInterval(interval time.Duration) Option {
	return func(o *options) {
		o.cleanupInterval = interval
	}
}

// WithProcDir overrides the default directory where the cleanup function will look for active users.
func WithProcDir(path string) Option {
	return func(o *options) {
		o.procDir = path
	}
}

// WaitCleanupRoutineDone ensures that the cleanup routine ran at least once, then stops it and closes the cache.
func (m *Manager) WaitCleanupRoutineDone(t *testing.T, args ...Option) {
	t.Helper()

	opt := options{}
	for _, arg := range args {
		arg(&opt)
	}

	// Give some time for the cleanup routine to run.
	// Ensure that we sleep at least more than the cleanup interval.
	time.Sleep(opt.cleanupInterval + (100 * time.Millisecond))

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
