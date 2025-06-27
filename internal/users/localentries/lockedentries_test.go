package localentries_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/types"
)

func TestEntriesWithLockInvalidActions(t *testing.T) {
	// This cannot be parallel

	require.Panics(t, func() { (&localentries.WithLock{}).MustBeLocked() },
		"MustBeLocked should panic but did not")
	require.Panics(t, func() { _, _ = (&localentries.WithLock{}).GetUserEntries() },
		"GetUserEntries should panic but did not")
	require.Panics(t, func() { _, _ = (&localentries.WithLock{}).GetGroupEntries() },
		"GetGroupEntries should panic but did not")
	require.Panics(t, func() { _, _ = (&localentries.WithLock{}).GetLocalGroupEntries() },
		"GetLocalGroupEntries should panic but did not")

	le, unlock, err := localentries.NewWithLock()

	require.NoError(t, err, "Setup: failed to lock the users group")
	err = unlock()
	require.NoError(t, err, "Unlock should not fail to lock the users group")

	err = unlock()
	require.Error(t, err, "Unlocking twice should fail")

	require.Panics(t, func() { le.MustBeLocked() },
		"MustBeLocked should panic but did not")
	require.Panics(t, func() { _, _ = le.GetUserEntries() },
		"GetUserEntries should panic but did not")
	require.Panics(t, func() { _, _ = le.GetGroupEntries() },
		"GetGroupEntries should panic but did not")
	require.Panics(t, func() { _, _ = le.GetLocalGroupEntries() },
		"GetLocalGroupEntries should panic but did not")

	// This is to ensure that we're in a good state, despite the actions above
	for range 10 {
		le, unlock, err = localentries.NewWithLock()
		require.NoError(t, err, "Failed to lock the users group")
		defer func() {
			err := unlock()
			require.NoError(t, err, "Unlock should not fail to lock the users group")
		}()
	}
}

//nolint:tparallel // This can't be parallel, but subtests can.
func TestRacingEntriesLockingActions(t *testing.T) {
	const nIterations = 50

	testFilePath := filepath.Join("testdata", "no_users_in_our_groups.group")

	wg := sync.WaitGroup{}
	wg.Add(nIterations)

	// Lock and get the values in parallel.
	for idx := range nIterations {
		t.Run(fmt.Sprintf("iteration_%d", idx), func(t *testing.T) {
			t.Parallel()

			t.Cleanup(wg.Done)

			var opts []localentries.Option
			wantGroup := types.GroupEntry{Name: "root", GID: 0, Passwd: "x"}
			useTestGroupFile := idx%3 == 0

			if useTestGroupFile {
				// Mix the requests with test-only code paths...
				opts = append(opts, localentries.WithGroupPath(testFilePath))
				wantGroup = types.GroupEntry{Name: "localgroup1", GID: 41, Passwd: "x"}
			}

			lockedEntries, entriesUnlock, err := localentries.NewWithLock(opts...)
			require.NoError(t, err, "Failed to lock the local entries")

			lg := localentries.GetGroupsWithLock(lockedEntries)
			groups, err := lg.GetEntries()
			require.NoError(t, err, "GetEntries should not return an error, but did")
			require.NotEmpty(t, groups, "Got empty groups (test groups: %v)", useTestGroupFile)
			require.Contains(t, groups, wantGroup, "Expected group was not found  (test groups: %v)", useTestGroupFile)
			err = entriesUnlock()
			require.NoError(t, err, "EntriesUnlock() should not fail to lock the users group (test groups: %v)", useTestGroupFile)
		})
	}
}
