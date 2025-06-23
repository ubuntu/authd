package localentries_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/fileutils"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users/localentries"
	localentriestestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/log"
)

func TestUpdatelocalentries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		newGroups     []string
		oldGroups     []string
		groupFilePath string

		wantLockErr   bool
		wantUpdateErr bool
	}{
		// First insertion cases
		"Insert_new_user_in_existing_files_with_no_users_in_our_group":             {groupFilePath: "no_users_in_our_groups.group"},
		"Insert_new_user_when_no_users_in_any_group":                               {groupFilePath: "no_users.group"},
		"Insert_new_user_in_existing_files_with_other_users_in_our_group":          {groupFilePath: "users_in_our_groups.group"},
		"Insert_new_user_in_existing_files_with_multiple_other_users_in_our_group": {groupFilePath: "multiple_users_in_our_groups.group"},

		// User already in groups
		"No-Op_for_user_is_already_present_in_both_local_groups":                  {groupFilePath: "user_in_both_groups.group"},
		"Insert_user_in_the_only_local_group_when_not_present":                    {groupFilePath: "user_in_one_group.group"},
		"Insert_user_in_the_only_local_group_when_not_present_even_with_multiple": {groupFilePath: "user_and_others_in_one_groups.group"},
		"Remove_user_from_an_additional_group_without_other_users":                {groupFilePath: "user_in_second_local_group.group"},
		"Remove_user_from_an_additional_group_with_other_users":                   {groupFilePath: "user_in_second_local_group_with_others.group"},
		"Add_and_remove_user_from_multiple_groups_with_one_remaining":             {groupFilePath: "user_in_many_groups.group"},

		// Flexible accepted cases
		"Missing_group_is_ignored":              {groupFilePath: "missing_group.group"},
		"Group_file_with_empty_line_is_ignored": {groupFilePath: "empty_line.group"},

		// No new groups
		"No-Op_for_user_with_no_groups_and_was_in_none": {newGroups: []string{}, groupFilePath: "no_users_in_our_groups.group"},
		"Remove_user_with_no_groups_from_existing_ones": {newGroups: []string{}, groupFilePath: "user_in_both_groups.group"},

		// User removed from groups
		"User_is_added_to_group_they_were_added_to_before":          {newGroups: []string{"localgroup1"}, oldGroups: []string{"localgroup1"}, groupFilePath: "no_users.group"},
		"User_is_removed_from_old_groups_but_not_from_other_groups": {newGroups: []string{}, oldGroups: []string{"localgroup3"}, groupFilePath: "user_in_both_groups.group"},
		"User_is_not_removed_from_groups_they_are_not_part_of":      {newGroups: []string{}, oldGroups: []string{"localgroup2"}, groupFilePath: "user_in_one_group.group"},

		// Error cases
		"Error_on_missing_groups_file": {
			groupFilePath: "does_not_exists.group",
			wantLockErr:   true,
		},
		"Error_on_invalid_user_name": {
			groupFilePath: "no_users_in_our_groups.group",
			username:      "no,commas,please",
			wantUpdateErr: true,
		},
		"Error_when_groups_file_has_missing_fields": {
			groupFilePath: "malformed_file_missing_field.group",
			wantLockErr:   true,
		},
		"Error_when_groups_file_has_invalid_gid": {
			groupFilePath: "malformed_file_invalid_gid.group",
			wantLockErr:   true,
		},
		"Error_when_groups_file_has_no_group_name": {
			groupFilePath: "malformed_file_no_group_name.group",
			wantLockErr:   true,
		},
		"Error_when_groups_file_has_a_duplicated_group": {
			groupFilePath: "malformed_file_duplicated.group",
			wantLockErr:   true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.newGroups == nil {
				tc.newGroups = []string{"localgroup1", "localgroup3"}
			}

			switch tc.username {
			case "":
				tc.username = "myuser"
			case "-":
				tc.username = ""
			}

			inputGroupFilePath := filepath.Join("testdata", tc.groupFilePath)
			outputGroupFilePath := filepath.Join(t.TempDir(), "group")

			if exists, _ := fileutils.FileExists(inputGroupFilePath); exists {
				tempGroupFile := filepath.Join(t.TempDir(), "group")
				err := fileutils.CopyFile(inputGroupFilePath, tempGroupFile)
				require.NoError(t, err, "failed to copy group file for testing")
				inputGroupFilePath = tempGroupFile
			}

			defer localentriestestutils.RequireGroupFile(t, outputGroupFilePath, golden.Path(t))

			lg, cleanup, err := localentries.GetGroupsWithLock(
				localentries.WithGroupInputPath(inputGroupFilePath),
				localentries.WithGroupOutputPath(outputGroupFilePath),
			)
			if tc.wantLockErr {
				require.Error(t, err, "GetGroupsWithLock should have failed")
				return
			}
			require.NoError(t, err, "Setup: failed to lock the users group")
			t.Cleanup(func() { require.NoError(t, cleanup(), "Releasing unlocked groups") })

			err = lg.Update(tc.username, tc.newGroups, tc.oldGroups)
			if tc.wantUpdateErr {
				require.Error(t, err, "Updatelocalentries should have failed")
			} else {
				require.NoError(t, err, "Updatelocalentries should not have failed")
			}
		})
	}
}

//nolint:tparallel // This can't be parallel, but subtests can.
func TestRacingLockingActions(t *testing.T) {
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
			useTestGroupFile := idx%3 == 0

			if useTestGroupFile {
				// Mix the requests with test-only code paths...
				opts = append(opts, localentries.WithGroupPath(testFilePath))
			}

			lg, unlock, err := localentries.GetGroupsWithLock(opts...)
			require.NoError(t, err, "Failed to lock the users group (test groups: %v)", useTestGroupFile)
			err = lg.Update("", nil, nil)
			require.NoError(t, err, "Update should not fail (test groups: %v)", useTestGroupFile)
			err = unlock()
			require.NoError(t, err, "Unlock should not fail to lock the users group (test groups: %v)", useTestGroupFile)
		})
	}

	t.Run("final_check", func(t *testing.T) {
		t.Parallel()
		wg.Wait()

		// Get a last unlock function, to see if we're all good...
		lg, unlock, err := localentries.GetGroupsWithLock()
		require.NoError(t, err, "Failed to lock the users group")
		err = unlock()
		require.NoError(t, err, "Unlock should not fail to lock the users group")

		// Ensure that we had cleaned up all the locks correctly!
		require.Panics(t, func() { _ = lg.Update("", nil, nil) })
	})
}

func TestLockedInvalidActions(t *testing.T) {
	// This cannot be parallel

	lg, unlock, err := localentries.GetGroupsWithLock()

	require.NoError(t, err, "Setup: failed to lock the users group")
	err = unlock()
	require.NoError(t, err, "Unlock should not fail to lock the users group")

	err = unlock()
	require.Error(t, err, "Unlocking twice should fail")

	require.Panics(t, func() { _ = lg.Update("", nil, nil) },
		"Update should panic but did not")

	// This is to ensure that we're in a good state, despite the actions above
	for range 10 {
		lg, unlock, err = localentries.GetGroupsWithLock()
		require.NoError(t, err, "Failed to lock the users group")
		defer func() {
			err := unlock()
			require.NoError(t, err, "Unlock should not fail to lock the users group")
		}()
	}
}

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)

	userslocking.Z_ForTests_OverrideLocking()
	defer userslocking.Z_ForTests_RestoreLocking()

	m.Run()
}
