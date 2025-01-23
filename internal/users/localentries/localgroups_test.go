package localentries_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users/localentries"
	localentriestestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
)

func TestUpdatelocalentries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		newGroups     []string
		oldGroups     []string
		groupFilePath string

		wantErr bool
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
		"Error_on_missing_groups_file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error_when_groups_file_is_malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error_on_any_unignored_add_gpasswd_error":    {username: "gpasswdfail", groupFilePath: "no_users.group", wantErr: true},
		"Error_on_any_unignored_delete_gpasswd_error": {username: "gpasswdfail", groupFilePath: "gpasswdfail_in_deleted_group.group", wantErr: true},
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

			destCmdsFile := filepath.Join(t.TempDir(), "gpasswd.output")

			groupFilePath := filepath.Join("testdata", tc.groupFilePath)
			cmdArgs := []string{"env", "GO_WANT_HELPER_PROCESS=1",
				os.Args[0], "-test.run=TestMockgpasswd", "--",
				groupFilePath, destCmdsFile,
			}

			err := localentries.Update(tc.username, tc.newGroups, tc.oldGroups, localentries.WithGroupPath(groupFilePath), localentries.WithGpasswdCmd(cmdArgs))
			if tc.wantErr {
				require.Error(t, err, "Updatelocalentries should have failed")
			} else {
				require.NoError(t, err, "Updatelocalentries should not have failed")
			}

			localentriestestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t))
		})
	}
}

func TestCleanlocalentries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		groupFilePath string

		getUsersReturn []string

		wantErr bool
	}{
		"No-op_when_there_are_no_inactive_users":        {groupFilePath: "user_in_many_groups.group"},
		"Cleans_up_user_from_group":                     {groupFilePath: "inactive_user_in_one_group.group"},
		"Cleans_up_user_from_multiple_groups":           {groupFilePath: "inactive_user_in_many_groups.group"},
		"Cleans_up_multiple_users_from_group":           {groupFilePath: "inactive_users_in_one_group.group"},
		"Cleans_up_multiple_users_from_multiple_groups": {groupFilePath: "inactive_users_in_many_groups.group"},

		"Error_if_there_is_no_active_user":            {groupFilePath: "user_in_many_groups.group", getUsersReturn: []string{}, wantErr: true},
		"Error_on_missing_groups_file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error_when_groups_file_is_malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error_on_any_unignored_delete_gpasswd_error": {groupFilePath: "gpasswdfail_in_deleted_group.group", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			destCmdsFile := filepath.Join(t.TempDir(), "gpasswd.output")
			groupFilePath := filepath.Join("testdata", tc.groupFilePath)
			gpasswdCmd := []string{"env", "GO_WANT_HELPER_PROCESS=1",
				os.Args[0], "-test.run=TestMockgpasswd", "--",
				groupFilePath, destCmdsFile,
			}

			if tc.getUsersReturn == nil {
				tc.getUsersReturn = []string{"myuser", "otheruser", "otheruser2", "otheruser3", "otheruser4"}
			}

			cleanupOptions := []localentries.Option{
				localentries.WithGpasswdCmd(gpasswdCmd),
				localentries.WithGroupPath(groupFilePath),
				localentries.WithGetUsersFunc(func() ([]string, error) { return tc.getUsersReturn, nil }),
			}
			err := localentries.Clean(cleanupOptions...)
			if tc.wantErr {
				require.Error(t, err, "Cleanuplocalentries should have failed")
			} else {
				require.NoError(t, err, "Cleanuplocalentries should not have failed")
			}

			localentriestestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t))
		})
	}
}

func TestCleanUserFromlocalentries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		groupFilePath   string
		wantMockFailure bool

		wantErr bool
	}{
		"Cleans_up_user_from_group":                   {},
		"Cleans_up_user_from_multiple_groups":         {groupFilePath: "user_in_many_groups.group"},
		"No_op_if_user_does_not_belong_to_any_groups": {username: "groupless"},

		"Error_on_missing_groups_file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error_when_groups_file_is_malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error_on_any_unignored_delete_gpasswd_error": {wantMockFailure: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.username == "" {
				tc.username = "myuser"
			}
			if tc.groupFilePath == "" {
				tc.groupFilePath = "user_in_one_group.group"
			}

			destCmdsFile := filepath.Join(t.TempDir(), "gpasswd.output")
			groupFilePath := filepath.Join("testdata", tc.groupFilePath)
			gpasswdCmd := []string{"env", "GO_WANT_HELPER_PROCESS=1",
				os.Args[0], "-test.run=TestMockgpasswd", "--",
				groupFilePath, destCmdsFile,
			}
			if tc.wantMockFailure {
				gpasswdCmd = append(gpasswdCmd, "gpasswdfail")
			}

			cleanupOptions := []localentries.Option{
				localentries.WithGpasswdCmd(gpasswdCmd),
				localentries.WithGroupPath(groupFilePath),
			}
			err := localentries.CleanUser(tc.username, cleanupOptions...)
			if tc.wantErr {
				require.Error(t, err, "CleanUserFromlocalentries should have failed")
			} else {
				require.NoError(t, err, "CleanUserFromlocalentries should not have failed")
			}

			localentriestestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t))
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localentriestestutils.Mockgpasswd(t)
}
