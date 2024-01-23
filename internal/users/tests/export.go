// Package tests export cache test functionalities used by other packages.
package tests

import (
	"unsafe"

	"github.com/ubuntu/authd/internal/newusers"
	"github.com/ubuntu/authd/internal/newusers/cache"
)

var (
	// DirtyFlagName is the dirty flag name exported for tests
	//
	//go:linkname DirtyFlagName github.com/ubuntu/authd/internal/newusers.dirtyFlagName
	DirtyFlagName string
)

type manager struct {
	cache *cache.Cache
}

// GetManagerCache returns the cache of the manager.
func GetManagerCache(m *newusers.Manager) *cache.Cache {
	//#nosec:G103 // This is only used in tests.
	mTest := *(*manager)(unsafe.Pointer(m))

	return mTest.cache
}
