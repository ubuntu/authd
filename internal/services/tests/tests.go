// Package tests export services test functionalities used by other packages to change some default options.
package tests

//nolint:gci // Importing unsafe requires a comment, so the imports can't be gci'ed.
import (
	"testing"
	"time"

	//nolint:revive,nolintlint // needed for go:linkname, but only used in tests. nolinlint as false positive then.
	_ "unsafe"
)

var (
	//go:linkname grpCleanInterval github.com/ubuntu/authd/internal/services.grpCleanInterval
	grpCleanInterval time.Duration
)

// OverrideCleanupInterval allow to change grpCleanInterval without using options.
// This is used for tests when we don’t have access to the manager object directly, like integration tests.
// Tests using this can't be run in parallel.
func OverrideCleanupInterval(t *testing.T, cleanupInterval time.Duration) {
	t.Helper()

	prev := grpCleanInterval
	t.Cleanup(func() { grpCleanInterval = prev })

	grpCleanInterval = cleanupInterval
}
