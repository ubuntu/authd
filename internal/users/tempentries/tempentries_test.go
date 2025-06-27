package tempentries

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	"github.com/ubuntu/authd/internal/users/localentries"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

func TestLockedInvalidActions(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() { _, _ = (&temporaryRecordsLocked{}).RegisterPreAuthUser("foobar") },
		"RegisterPreAuthUser should panic but did not")
	require.Panics(t, func() { _, _, _ = (&temporaryRecordsLocked{}).RegisterUser("foobar") },
		"RegisterUser should panic but did not")
	require.Panics(t, func() { _, _, _ = (&temporaryRecordsLocked{}).RegisterGroupForUser(123, "foobar") },
		"RegisterGroupForUser should panic but did not")

	require.Panics(t, func() { NewTemporaryRecords(nil).LockForChanges(&localentries.WithLock{}) },
		"LockForChanges should panic but did not")

	tmpRecords := NewTemporaryRecords(&idgenerator.IDGenerator{UIDMax: 100})

	entries, entriesUnlock, err := localentries.NewWithLock(([]localentries.Option{})...)
	require.NoError(t, err, "Setup: failed to lock the locale entries")

	records := tmpRecords.LockForChanges(entries)

	err = entriesUnlock()
	require.NoError(t, err, "entriesUnlock should not fail to unlock the local entries")

	err = entriesUnlock()
	require.Error(t, err, "Unlocking twice should fail")

	require.Panics(t, func() { _, _ = records.RegisterPreAuthUser("foobar") },
		"RegisterPreAuthUser should panic but did not")
	require.Panics(t, func() { _, _, _ = records.RegisterUser("foobar") },
		"RegisterUser should panic but did not")
	require.Panics(t, func() { _, _, _ = records.RegisterGroupForUser(123, "foobar") },
		"RegisterGroupForUser should panic but did not")

	// This is to ensure that we're in a good state, despite the actions above
	for range 10 {
		entries, entriesUnlock, err := localentries.NewWithLock()
		require.NoError(t, err, "Failed to lock the local entries")
		_ = tmpRecords.LockForChanges(entries)
		defer func() {
			err := entriesUnlock()
			require.NoError(t, err, "entriesUnlock should not fail to unlock the local entries")
		}()
	}
}

func TestRacingLockingActions(t *testing.T) {
	t.Parallel()

	// These tests are meant to stress-test in parallel our temporary entries,
	// this is happening by registering new users or pre-auth some of them and
	// ensure that the generated IDs and GIDs are not clashing.
	// There may be still clashes when the cleanup functions are called, but
	// then it's up to the caller to ensure that these IDs are not duplicated
	// in the database.

	const nIterations = 100

	type cleanupType string
	const (
		noCleanup      cleanupType = "no_cleanup"
		perUserCleanup cleanupType = "per_user_cleanup"
		perTestCleanup cleanupType = "per_test_cleanup"
	)

	for _, cleanupType := range []cleanupType{noCleanup, perTestCleanup, perUserCleanup} {
		registeredUsersMu := sync.Mutex{}
		registeredUsers := make(map[string][]uint32)
		registeredGroups := make(map[string][]uint32)

		tmpRecords := NewTemporaryRecords(&idgenerator.IDGenerator{
			UIDMin: 0,
			UIDMax: nIterations * 5,
			GIDMin: 0,
			GIDMax: nIterations * 5,
		})

		t.Run(fmt.Sprintf("with_%s", cleanupType), func(t *testing.T) {
			t.Parallel()

			for idx := range nIterations {
				t.Run(fmt.Sprintf("iteration_%d", idx), func(t *testing.T) {
					t.Parallel()

					entries, entriesUnlock, err := localentries.NewWithLock()
					require.NoError(t, err, "Setup: failed to lock the locale entries")
					t.Cleanup(func() {
						err = entriesUnlock()
						require.NoError(t, err, "entriesUnlock should not fail to unlock the local entries")
					})

					records := tmpRecords.LockForChanges(entries)
					doPreAuth := idx%3 == 0
					userName := fmt.Sprintf("authd-test-user%d", idx)

					cleanupsMu := sync.Mutex{}
					var cleanups []func()

					var userID atomic.Uint32
					var firstGroupID atomic.Uint32

					if doPreAuth {
						// In the pre-auth case we do even more parallelization, so that
						// the pre-auth happens without a defined order of the actual
						// registration.
						userName = fmt.Sprintf("authd-test-maybe-pre-check-user%d", idx)

						//nolint:thelper // This is actually a test function!
						preAuth := func(t *testing.T) {
							t.Parallel()

							uid, err := records.RegisterPreAuthUser(userName)
							require.NoError(t, err, "RegisterPreAuthUser should not fail but it did")

							if !userID.CompareAndSwap(0, uid) && cleanupType != perTestCleanup {
								require.Equal(t, int(uid), int(userID.Load()),
									"Pre-auth UID for already-registered user is not matching expected")
							}
						}

						for i := range 3 {
							t.Run(fmt.Sprintf("pre_auth%d", i), preAuth)
						}
					}

					//nolint:thelper // This is actually a test function!
					userUpdate := func(t *testing.T) {
						t.Parallel()

						// We do not run the cleanup function here, because we want to preserve the
						// user in our temporary entries, to ensure that we may not register the same
						// twice.
						uid, userCleanup, err := records.RegisterUser(userName)
						require.NoError(t, err, "RegisterUser should not fail but it did")
						if cleanupType == perTestCleanup {
							t.Cleanup(userCleanup)
						}

						if !userID.CompareAndSwap(0, uid) && cleanupType != perTestCleanup {
							require.Equal(t, int(uid), int(userID.Load()),
								"UID for pre-auth or already registered user %q is not matching expected",
								userName)
						}

						groupName1 := fmt.Sprintf("authd-test-group%d.1", idx)
						gid1, groupCleanup1, err := records.RegisterGroupForUser(uid, groupName1)
						require.NoError(t, err, "RegisterGroupForUser should not fail but it did")
						if cleanupType == perTestCleanup {
							t.Cleanup(groupCleanup1)
						}

						if !firstGroupID.CompareAndSwap(0, gid1) && cleanupType != perTestCleanup {
							require.Equal(t, int(gid1), int(firstGroupID.Load()),
								"GID for group %q is not matching expected", groupName1)
						}

						groupName2 := fmt.Sprintf("authd-test-group%d.2", idx)
						gid2, groupCleanup2, err := records.RegisterGroupForUser(uid, groupName2)
						require.NoError(t, err, "RegisterGroupForUser should not fail but it did")
						if cleanupType == perTestCleanup {
							t.Cleanup(groupCleanup2)
						}

						registeredUsersMu.Lock()
						defer registeredUsersMu.Unlock()
						registeredUsers[userName] = append(registeredUsers[userName], uid)
						registeredGroups[groupName1] = append(registeredGroups[groupName1], gid1)
						registeredGroups[groupName2] = append(registeredGroups[groupName2], gid2)

						cleanupsMu.Lock()
						defer cleanupsMu.Unlock()

						if cleanupType == perUserCleanup {
							cleanups = append(cleanups, userCleanup, groupCleanup1, groupCleanup2)
						}
					}

					testName := "update_user"
					if doPreAuth {
						testName = "maybe_finish_registration"
					}

					for i := range 3 {
						t.Run(fmt.Sprintf("%s%d", testName, i), userUpdate)
					}

					t.Cleanup(func() {
						cleanupsMu.Lock()
						defer cleanupsMu.Unlock()

						t.Logf("Running cleanups for iteration %d", idx)
						for _, cleanup := range cleanups {
							cleanup()
						}
					})
				})
			}

			t.Cleanup(func() {
				t.Log("Running final checks for cleanup mode " + fmt.Sprint(cleanupType))

				registeredUsersMu.Lock()
				defer registeredUsersMu.Unlock()

				if cleanupType == perTestCleanup {
					// In such case we may have duplicate UIDs or GIDs since they
					// may be duplicated after each test has finished.
					// This is not actually a problem because in the real scenario
					// the temporary entries owner (the user manager) should check
					// that such IDs are already registered in the database before
					// actually using them.
					return
				}

				uniqueIDs := make(map[uint32]string)

				for u, uids := range registeredUsers {
					uids = slices.Compact(uids)
					require.Len(t, uids, 1, "Only one UID should be registered for user %q", u)

					if cleanupType == perUserCleanup {
						// In this case we only care about the fact of having registered only one
						// UID for each user, although the UIDs may not be unique across all the
						// tests, since the cleanup functions have deleted the temporary data.
						// It's still important to test this case though, since it allows to ensure
						// that the pre-check and registered user IDs are valid.
						continue
					}

					old, ok := uniqueIDs[uids[0]]
					require.False(t, ok, "ID %d must be unique across entries, but it's used by both %q and %q",
						uids[0], u, old)
					uniqueIDs[uids[0]] = u
				}

				for g, gids := range registeredGroups {
					gids = slices.Compact(gids)
					require.Len(t, gids, 1, "Only one GID should be registered for group %q", g)

					if cleanupType == perUserCleanup {
						// In this case we only care about the fact of having registered only one
						// UID for each user, although the UIDs may not be unique across all the
						// tests, since the cleanup functions have deleted the temporary data.
						// It's still important to test this case though, since it allows to ensure
						// that the pre-check and registered user IDs are valid.
						continue
					}

					old, ok := uniqueIDs[gids[0]]
					require.False(t, ok, "ID %d must be unique across entries, but it's used both %q and %q",
						gids[0], g, old)
					uniqueIDs[gids[0]] = g
				}
			})
		})
	}
}

func TestRegisterUser(t *testing.T) {
	t.Parallel()

	uidToGenerate := uint32(12345)
	userName := "authd-temp-users-test"

	tests := map[string]struct {
		userName                string
		uidsToGenerate          []uint32
		userAlreadyRemoved      bool
		replacesPreAuthUser     bool
		preAuthUIDAlreadyExists bool

		wantErr bool
	}{
		"Successfully_register_a_new_user": {},
		"Successfully_register_a_user_if_the_first_generated_UID_is_already_in_use": {
			uidsToGenerate: []uint32{0, uidToGenerate}, // UID 0 (root) always exists
		},
		"Successfully_register_a_user_if_the_pre-auth_user_already_exists": {
			replacesPreAuthUser: true,
			uidsToGenerate:      []uint32{}, // No UID generation needed
		},

		"Error_when_name_is_already_in_use": {userName: "root", wantErr: true},
		"Error_when_pre-auth_user_already_exists_and_name_is_not_unique": {
			replacesPreAuthUser: true,
			userName:            "root",
			wantErr:             true,
		},
		"Error_when_a_valid_UID_cannot_be_found": {
			uidsToGenerate: make([]uint32, maxIDGenerateIterations*10),
			wantErr:        true,
		},
		"Error_when_pre-auth_UID_is_not_unique": {
			replacesPreAuthUser:     true,
			preAuthUIDAlreadyExists: true,
			wantErr:                 true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.userName == "" {
				tc.userName = userName
			}

			if tc.uidsToGenerate == nil {
				tc.uidsToGenerate = []uint32{uidToGenerate}
			}

			idGeneratorMock := &idgenerator.IDGeneratorMock{UIDsToGenerate: tc.uidsToGenerate}
			records := NewTemporaryRecords(idGeneratorMock)

			var preAuthUID uint32
			if tc.replacesPreAuthUser {
				preAuthUID = uidToGenerate
				if tc.preAuthUIDAlreadyExists {
					preAuthUID = 0 // UID 0 (root) always exists
				}
				err := records.preAuthUserRecords.addPreAuthUser(preAuthUID, tc.userName)
				require.NoError(t, err, "addPreAuthUser should not return an error, but did")
			}

			uid, cleanup, err := records.registerUser(tc.userName)
			if tc.wantErr {
				require.Error(t, err, "RegisterUser should return an error, but did not")
				return
			}
			t.Cleanup(cleanup)
			require.NoError(t, err, "RegisterUser should not return an error, but did")
			require.Equal(t, uidToGenerate, uid, "UID should be the one generated by the IDGenerator")

			if tc.replacesPreAuthUser {
				// Check that the pre-auth user was removed
				user, err := records.preAuthUserRecords.userByID(preAuthUID)
				require.NoError(t, err, "userByID should not return an error, but it not")
				checkPreAuthUser(t, user)
				return
			}

			_, err = records.UserByID(uid)
			require.Error(t, err, "userByID should return an error, but did not")
		})
	}
}

func TestRegisterUserAndGroupForUser(t *testing.T) {
	t.Parallel()

	defaultUserName := "authd-temp-users-test"
	uidToGenerate := uint32(12345)
	defaultGroupName := "authd-temp-groups-test"
	gidToGenerate := uint32(54321)

	type userTestData struct {
		name       string
		preAuth    bool
		groups     []string
		runCleanup bool

		// Simulates the case in which an UID has been registered while another
		// request wins the UID registration race.
		simulateUIDRace bool

		// Simulates the case in which an user has been registered while another
		// request wins the username registration race.
		simulateNameRace bool

		wantUID      uint32
		wantErr      bool
		wantGroupErr []bool
		wantGIDs     []uint32
	}

	defaultUserTestData := userTestData{name: defaultUserName}

	tests := map[string]struct {
		users          []userTestData
		uidsToGenerate []uint32
		gidsToGenerate []uint32
	}{
		"Successfully_register_a_new_group_for_generic_user": {},
		"Successfully_register_a_new_group_for_pre-auth_user": {
			users: []userTestData{{name: defaultUserName, preAuth: true}},
		},
		"Successfully_register_a_new_group_for_various_generic_users": {
			users: []userTestData{
				{name: defaultUserName + fmt.Sprint(1)},
				{name: defaultUserName + fmt.Sprint(2)},
				{name: defaultUserName + fmt.Sprint(3)},
			},
		},
		"Successfully_register_a_new_group_for_various_pre-auth_users": {
			users: []userTestData{
				{name: defaultUserName + fmt.Sprint(1), preAuth: true},
				{name: defaultUserName + fmt.Sprint(2), preAuth: true},
				{name: defaultUserName + fmt.Sprint(3), preAuth: true},
			},
		},
		"Successfully_register_a_various_groups_for_generic_user": {
			users: []userTestData{
				{name: defaultUserName, groups: []string{"group-a", "group-b", "group-c"}},
			},
		},
		"Successfully_register_a_various_groups_for_pre-auth_user": {
			users: []userTestData{
				{name: defaultUserName, preAuth: true, groups: []string{"group-a", "group-b", "group-c"}},
			},
		},
		"Successfully_register_a_new_user_if_the_first_generated_UID_is_already_in_use": {
			uidsToGenerate: []uint32{0, uidToGenerate}, // UID 0 (root) always exists
			users:          []userTestData{{name: defaultUserName, preAuth: true, wantUID: uidToGenerate}},
		},
		"Successfully_register_a_new_group_if_the_first_generated_GID_is_already_in_use": {
			users:          []userTestData{{name: defaultUserName, wantGIDs: []uint32{gidToGenerate}}},
			gidsToGenerate: []uint32{0, gidToGenerate}, // GID 0 (root) always exists
		},
		"Successfully_register_a_new_group_if_the_first_generated_GID_matches_the_user_UID": {
			users:          []userTestData{{name: defaultUserName, wantGIDs: []uint32{gidToGenerate}}},
			gidsToGenerate: []uint32{uidToGenerate, gidToGenerate}, // UID should be skipped!
		},
		"Successfully_register_a_new_group_if_the_first_generated_GID_is_already_in_use_by_a_pre-check_user": {
			users: []userTestData{
				{name: defaultUserName, preAuth: true},
				{name: "another-user-name"},
			},
			gidsToGenerate: []uint32{uidToGenerate, gidToGenerate, gidToGenerate + 1},
		},
		"Successfully_register_an_user_if_the_first_generated_UID_is_already_registered": {
			users: []userTestData{
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "pre-checked-user", wantUID: uidToGenerate, wantGIDs: []uint32{gidToGenerate}},
				{name: "other-user", wantUID: uidToGenerate + 1, wantGIDs: []uint32{gidToGenerate + 1}},
			},
			uidsToGenerate: []uint32{uidToGenerate, uidToGenerate, uidToGenerate + 1},
		},
		"Successfully_register_a_pre-checked_user_if_the_first_generated_UID_is_already_registered": {
			users: []userTestData{
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "other-pre-checked-user", preAuth: true, wantUID: uidToGenerate + 1},
			},
			uidsToGenerate: []uint32{uidToGenerate, uidToGenerate, uidToGenerate + 1},
		},
		"Successfully_register_a_pre-checked_user_twice_with_the_same_UID": {
			users: []userTestData{
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
			},
		},
		"Successfully_register_an_user_after_two_pre_checks": {
			users: []userTestData{
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "pre-checked-user", wantUID: uidToGenerate, wantGIDs: []uint32{gidToGenerate}},
			},
		},
		"Successfully_register_an_user_with_the_same_name_after_two_pre_checks": {
			users: []userTestData{
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate},
				{name: "pre-checked-user", wantUID: uidToGenerate, wantGIDs: []uint32{gidToGenerate}, runCleanup: true},
				{name: "pre-checked-user", preAuth: true, wantUID: uidToGenerate + 1},
			},
		},
		"Successfully_register_a_pre-check_user_if_multiple_concurrent_requests_happen": {
			users: []userTestData{
				{name: "racing-pre-checked-user", preAuth: true, simulateNameRace: true},
				{name: "racing-pre-checked-user", preAuth: true, wantUID: uidToGenerate},
			},
		},
		"Successfully_register_an_user_if_multiple_concurrent_pre-check_requests_happen": {
			users: []userTestData{
				{name: "racing-pre-checked-user", preAuth: true, simulateNameRace: true},
				{name: "racing-pre-checked-user", wantUID: uidToGenerate, wantGIDs: []uint32{gidToGenerate}},
			},
		},
		"Successfully_register_an_user_if_multiple_concurrent_requests_happen": {
			users: []userTestData{
				{name: "racing-user", simulateNameRace: true, wantUID: uidToGenerate},
				{name: "racing-user", wantUID: uidToGenerate + 1, wantGIDs: []uint32{gidToGenerate}},
			},
		},
		"Successfully_register_an_user_if_multiple_concurrent_pre-check_requests_happen_for_the_same_UID": {
			users: []userTestData{
				{name: "racing-pre-checked-user", preAuth: true, simulateUIDRace: true},
				{name: "racing-pre-checked-user", wantUID: uidToGenerate, wantGIDs: []uint32{gidToGenerate}},
			},
		},
		"Successfully_register_an_user_if_multiple_concurrent_requests_happen_for_the_same_UID": {
			users: []userTestData{
				{name: "racing-user", simulateUIDRace: true, wantUID: uidToGenerate},
				{name: "racing-user", wantUID: uidToGenerate, wantGIDs: []uint32{gidToGenerate}},
			},
		},

		"Error_if_there_are_no_UID_to_generate": {
			users:          []userTestData{{name: defaultUserName, wantErr: true}},
			uidsToGenerate: []uint32{0},
		},
		"Error_if_there_are_no_UID_to_generate_for_pre-check_user": {
			users:          []userTestData{{name: defaultUserName, preAuth: true, wantErr: true}},
			uidsToGenerate: []uint32{0},
		},
		"Error_if_there_are_no_GID_to_generate": {
			users:          []userTestData{{name: defaultUserName, preAuth: true, wantGroupErr: []bool{true}}},
			gidsToGenerate: []uint32{},
		},
		"Error_if_there_are_no_GID_to_generate_for_pre-check_user": {
			users:          []userTestData{{name: defaultUserName, preAuth: true, wantGroupErr: []bool{true}}},
			gidsToGenerate: []uint32{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if len(tc.users) == 0 {
				tc.users = append(tc.users, defaultUserTestData)
			}

			if tc.uidsToGenerate == nil {
				uid := uidToGenerate
				for range tc.users {
					tc.uidsToGenerate = append(tc.uidsToGenerate, uid)
					uid++
				}
			}
			t.Log("UIDs to generate", tc.uidsToGenerate)

			if tc.gidsToGenerate == nil {
				gid := gidToGenerate
				for _, u := range tc.users {
					if u.preAuth {
						continue
					}
					groups := len(u.groups)
					if u.groups == nil {
						groups = 1
					}
					for range groups {
						tc.gidsToGenerate = append(tc.gidsToGenerate, gid)
						gid++
					}
				}
			}
			t.Log("GIDs to generate", tc.gidsToGenerate)

			wantRegisteredUsers := 0
			var registeredUIDs []uint32

			wantRegisteredGroups := 0
			var registeredGIDs []uint32

			lastCleanupIdx := 0

			idGeneratorMock := &idgenerator.IDGeneratorMock{
				UIDsToGenerate: tc.uidsToGenerate,
				GIDsToGenerate: tc.gidsToGenerate,
			}
			records := NewTemporaryRecords(idGeneratorMock)

			for idx, uc := range tc.users {
				t.Logf("Registering user %q", uc.name)

				replacingPreAuthUser := false
				if !uc.preAuth {
					_, err := records.userByLogin(uc.name)
					replacingPreAuthUser = err == nil
				}

				entries, entriesUnlock, err := localentries.NewWithLock()
				require.NoError(t, err, "Setup: failed to lock the locale entries")
				t.Cleanup(func() {
					err = entriesUnlock()
					require.NoError(t, err, "entriesUnlock should not fail to unlock the local entries")
				})

				records := records.LockForChanges(entries)
				internalRecords := records.tr

				var uid uint32
				var cleanup func()
				if uc.preAuth {
					uid, err = records.RegisterPreAuthUser(uc.name)
				} else {
					uid, cleanup, err = records.RegisterUser(uc.name)
				}

				if cleanup != nil {
					t.Cleanup(cleanup)
				}

				if uc.wantErr {
					require.Error(t, err, "User registration should return an error, but did not")
					continue
				}

				require.NoError(t, err, "User registration should not return an error, but did")
				t.Logf("Registered user %q (preauth: %v) with UID %d", uc.name, uc.preAuth, uid)

				isDuplicated := slices.ContainsFunc(tc.users[lastCleanupIdx:idx], func(u userTestData) bool {
					return u.name == uc.name
				})

				if uc.wantUID == 0 {
					uc.wantUID = tc.uidsToGenerate[idx]
				}

				if !isDuplicated && uc.preAuth {
					wantRegisteredUsers++
				}

				require.Equal(t, uc.wantUID, uid, "%q UID is not matching expected value", uc.name)
				require.Equal(t, wantRegisteredUsers, len(internalRecords.users),
					"Number of pre-auth registered, users should be %d", wantRegisteredUsers)

				if isDuplicated {
					require.Contains(t, registeredUIDs, uid, "UID %d has been already registered!", uid)
				} else {
					require.NotContains(t, registeredUIDs, uid, "UID %d has not been already registered!", uid)
				}

				registeredUIDs = append(registeredUIDs, uid)

				if uc.runCleanup {
					require.NotNil(t, cleanup, "Cleanup function is invalid!")
					cleanup()
					lastCleanupIdx = idx + 1

					if replacingPreAuthUser {
						wantRegisteredUsers--
						require.Equal(t, wantRegisteredUsers, len(internalRecords.users),
							"Number of pre-auth registered, users should be %d", wantRegisteredUsers)
					}
				}

				// Check that the user was registered
				user, err := internalRecords.userByLogin(uc.name)
				if uc.preAuth || (replacingPreAuthUser && !uc.runCleanup) {
					require.NoError(t, err, "UserByID should not return an error, but did")
				} else {
					require.ErrorIs(t, err, NoDataFoundError{})
					require.Zero(t, user, "User should be unset")
				}

				var goldenOptions []golden.Option
				userSuffix := ""
				if idx > 0 {
					userSuffix = fmt.Sprintf("_%s_%d", uc.name, idx)
					goldenOptions = append(goldenOptions, golden.WithSuffix(userSuffix))
				}

				if uc.preAuth {
					checkPreAuthUser(t, user, goldenOptions...)

					if uc.simulateUIDRace {
						t.Logf("Dropping the registered UID %d for %q", uid, uc.name)
						delete(internalRecords.preAuthUserRecords.users, uid)

						lastCleanupIdx = idx + 1
						registeredUIDs = slices.DeleteFunc(registeredUIDs,
							func(u uint32) bool { return u == uid })
						wantRegisteredUsers--
					}

					if uc.simulateNameRace {
						t.Logf("Dropping the registered login name %q for %d", uc.name, uid)
						delete(internalRecords.preAuthUserRecords.uidByLogin, uc.name)
					}

					// We don't have groups registration for the pre-check user.
					continue
				}

				checkUser(t, types.UserEntry{Name: uc.name, UID: uid, GID: uid}, goldenOptions...)

				if uc.simulateUIDRace {
					t.Logf("Dropping the registered UID %d for %q", uid, uc.name)
					delete(internalRecords.idTracker.ids, uid)
					continue
				}

				if uc.simulateNameRace {
					// We drop the registered name to check if the logic can handle such case.
					t.Logf("Dropping the registered user name %q for %d", uc.name, uid)
					delete(internalRecords.idTracker.userNames, uc.name)
					lastCleanupIdx = idx + 1
					continue
				}

				if uc.groups == nil {
					uc.groups = append(uc.groups, defaultGroupName)
				}
				if uc.wantGIDs == nil {
					uc.wantGIDs = tc.gidsToGenerate[idx:]
				}
				if uc.wantGroupErr == nil {
					uc.wantGroupErr = make([]bool, len(uc.groups))
				}

				numGroups := 0

				for idx, groupName := range uc.groups {
					groupName = uc.name + "+" + groupName
					t.Logf("Registering group %q", groupName)

					gid, cleanup, err := records.RegisterGroupForUser(uid, groupName)
					if uc.wantGroupErr[idx] {
						require.Error(t, err, "RegisterGroup should return an error, but did not")
						continue
					}

					require.NoError(t, err, "RegisterGroup should not return an error, but did")
					t.Logf("Registered group %q with GID %d", groupName, gid)
					t.Cleanup(cleanup)

					isDuplicated := slices.Contains(uc.groups[:idx], groupName)
					if !isDuplicated {
						numGroups++
						wantRegisteredGroups++
					}

					if isDuplicated {
						require.Contains(t, registeredGIDs, gid, "GID %d has been already registered!", gid)
					} else {
						require.NotContains(t, registeredGIDs, gid, "GID %d has not been already registered!", gid)
					}

					wantGID := uc.wantGIDs[numGroups-1]
					registeredGIDs = append(registeredGIDs, gid)

					require.NoError(t, err, "RegisterGroup should not return an error, but did")
					require.Equal(t, wantGID, gid, "%q GID is not matching expected value", groupName)
					require.Equal(t, wantRegisteredGroups, len(internalRecords.groups),
						"Number of groups registered, users should be %d", wantRegisteredGroups)

					// Check that the temporary group was created
					group, err := internalRecords.GroupByID(gid)
					require.NoError(t, err, "GroupByID should not return an error, but did")

					groupSuffix := groupName
					if idx > 0 {
						groupSuffix = fmt.Sprintf("%s_%d", uc.name, idx)
					}
					checkGroup(t, group,
						golden.WithSuffix("_"+groupSuffix))
				}
			}
		})
	}
}

func TestUserByIDAndName(t *testing.T) {
	t.Parallel()

	userName := "authd-temp-users-test"
	uidToGenerate := uint32(12345)

	tests := map[string]struct {
		registerUser       bool
		userAlreadyRemoved bool
		byName             bool

		wantErr bool
	}{
		"Error_when_user_is_not_registered_-_UserByID":   {wantErr: true},
		"Error_when_user_is_not_registered_-_UserByName": {byName: true, wantErr: true},
		"Error_when_user_is_already_removed_-_UserByID": {
			registerUser: true,
			wantErr:      true,
		},
		"Error_when_user_is_already_removed_-_UserByName": {
			registerUser: true,
			byName:       true,
			wantErr:      true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			idGeneratorMock := &idgenerator.IDGeneratorMock{UIDsToGenerate: []uint32{uidToGenerate}}
			records := NewTemporaryRecords(idGeneratorMock)

			if tc.registerUser {
				uid, cleanup, err := records.registerUser(userName)
				require.NoError(t, err, "RegisterUser should not return an error, but did")
				require.Equal(t, uidToGenerate, uid, "UID should be the one generated by the IDGenerator")
				t.Cleanup(cleanup)
			}

			var user types.UserEntry
			var err error
			if tc.byName {
				user, err = records.userByName(userName)
			} else {
				user, err = records.userByID(uidToGenerate)
			}

			if tc.wantErr {
				require.Error(t, err, "UserByID should return an error, but did not")
				return
			}
			require.NoError(t, err, "UserByID should not return an error, but did")
			checkUser(t, user)
		})
	}
}

func checkUser(t *testing.T, user types.UserEntry, options ...golden.Option) {
	t.Helper()

	golden.CheckOrUpdateYAML(t, user, options...)
}

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)

	userslocking.Z_ForTests_OverrideLocking()
	defer userslocking.Z_ForTests_RestoreLocking()

	m.Run()
}
