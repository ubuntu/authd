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
		"Insert new user in existing files with no users in our group":             {groupFilePath: "no_users_in_our_groups.group"},
		"Insert new user when no users in any group":                               {groupFilePath: "no_users.group"},
		"Insert new user in existing files with other users in our group":          {groupFilePath: "users_in_our_groups.group"},
		"Insert new user in existing files with multiple other users in our group": {groupFilePath: "multiple_users_in_our_groups.group"},

		// User already in groups
		"No-Op for user is already present in both local groups":                  {groupFilePath: "user_in_both_groups.group"},
		"Insert user in the only local group when not present":                    {groupFilePath: "user_in_one_group.group"},
		"Insert user in the only local group when not present even with multiple": {groupFilePath: "user_and_others_in_one_groups.group"},
		"Remove user from an additional group without other users":                {groupFilePath: "user_in_second_local_group.group"},
		"Remove user from an additional group with other users":                   {groupFilePath: "user_in_second_local_group_with_others.group"},
		"Add and remove user from multiple groups with one remaining":             {groupFilePath: "user_in_many_groups.group"},

		// Flexible accepted cases
		"Missing group is ignored":              {groupFilePath: "missing_group.group"},
		"Group file with empty line is ignored": {groupFilePath: "empty_line.group"},

		// No new groups
		"No-Op for user with no groups and was in none": {newGroups: []string{}, groupFilePath: "no_users_in_our_groups.group"},
		"Remove user with no groups from existing ones": {newGroups: []string{}, groupFilePath: "user_in_both_groups.group"},

		// User removed from groups
		"User is added to group they were added to before":          {newGroups: []string{"localgroup1"}, oldGroups: []string{"localgroup1"}, groupFilePath: "no_users.group"},
		"User is removed from old groups but not from other groups": {newGroups: []string{}, oldGroups: []string{"localgroup3"}, groupFilePath: "user_in_both_groups.group"},
		"User is not removed from groups they are not part of":      {newGroups: []string{}, oldGroups: []string{"localgroup2"}, groupFilePath: "user_in_one_group.group"},

		// Error cases
		"Error on missing groups file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error when groups file is malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error on any unignored add gpasswd error":    {username: "gpasswdfail", groupFilePath: "no_users.group", wantErr: true},
		"Error on any unignored delete gpasswd error": {username: "gpasswdfail", groupFilePath: "gpasswdfail_in_deleted_group.group", wantErr: true},
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
		"No-op when there are no inactive users":        {groupFilePath: "user_in_many_groups.group"},
		"Cleans up user from group":                     {groupFilePath: "inactive_user_in_one_group.group"},
		"Cleans up user from multiple groups":           {groupFilePath: "inactive_user_in_many_groups.group"},
		"Cleans up multiple users from group":           {groupFilePath: "inactive_users_in_one_group.group"},
		"Cleans up multiple users from multiple groups": {groupFilePath: "inactive_users_in_many_groups.group"},

		"Error if there is no active user":            {groupFilePath: "user_in_many_groups.group", getUsersReturn: []string{}, wantErr: true},
		"Error on missing groups file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error when groups file is malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error on any unignored delete gpasswd error": {groupFilePath: "gpasswdfail_in_deleted_group.group", wantErr: true},
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
		"Cleans up user from group":                   {},
		"Cleans up user from multiple groups":         {groupFilePath: "user_in_many_groups.group"},
		"No op if user does not belong to any groups": {username: "groupless"},

		"Error on missing groups file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error when groups file is malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error on any unignored delete gpasswd error": {wantMockFailure: true, wantErr: true},
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
