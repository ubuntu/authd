package cache

import "time"

// RequestClearDatabase is used in tests for checking the behaviour of the database dynamic clear up.
func RequestClearDatabase(c *Cache) {
	c.requestClearDatabase()
}

// WithoutCleaningOnNew skips the cleaning of old users when creating the cache.
func WithoutCleaningOnNew() Option {
	return func(o *options) {
		o.cleanOnNew = false
	}
}

// WithCleanupInterval overrides the default interval for cleaning the cache of expired users.
func WithCleanupInterval(interval time.Duration) Option {
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
