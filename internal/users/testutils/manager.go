// Package userstestutils export cache test functionalities used by other packages.
package userstestutils

import (
	"unsafe"

	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/cache"
)

func init() {
	// No import outside of testing environment.
	testsdetection.MustBeTesting()
}

type manager struct {
	cache *cache.Cache
}

// GetManagerCache returns the cache of the manager.
func GetManagerCache(m *users.Manager) *cache.Cache {
	//#nosec:G103 // This is only used in tests.
	mTest := *(*manager)(unsafe.Pointer(m))

	return mTest.cache
}
