package users

import "time"

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
