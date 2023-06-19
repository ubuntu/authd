package daemon

import (
	"testing"
)

func withCacheDir(dir string) func(*options) {
	return func(o *options) {
		o.cacheDir = dir
	}
}

// NewForTesting creates a new App with overridden paths for the service and daemon caches.
func NewForTesting(t *testing.T, cacheDir string) *App {
	t.Helper()

	if cacheDir == "" {
		cacheDir = t.TempDir()
	}
	return New(withCacheDir(cacheDir))
}
