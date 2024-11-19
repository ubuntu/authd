package localgroups_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/golden"
	"github.com/ubuntu/authd/internal/users/localgroups"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
)

func TestUpdateLocalGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		groups        []string
		groupFilePath string

		wantErr bool
	}{
		// First insertion cases
		"Insert new user in existing files with no users in our group":             {groupFilePath: "no_users_in_our_groups.group"},
		"Insert new user when no users in any group":                               {groupFilePath: "no_users.group"},
		"Insert new user in existing files with other users in our group":          {groupFilePath: "users_in_our_groups.group"},
		"Insert new user in existing files with multiple other users in our group": {groupFilePath: "multiple_users_in_our_groups.group"},

		// Users in existing groups
		"No-Op for user is already present in both local groups":                  {groupFilePath: "user_in_both_groups.group"},
		"Insert user in the only local group when not present":                    {groupFilePath: "user_in_one_group.group"},
		"Insert user in the only local group when not present even with multiple": {groupFilePath: "user_and_others_in_one_groups.group"},
		"Remove user from an additional group, being alone":                       {groupFilePath: "user_in_second_local_group.group"},
		"Remove user from an additional group, multiple users in group":           {groupFilePath: "user_in_second_local_group_with_others.group"},
		"Add and remove user from multiple groups, one remaining":                 {groupFilePath: "user_in_many_groups.group"},

		// Flexible accepted cases
		"Missing group is ignored":              {groupFilePath: "missing_group.group"},
		"Group file with empty line is ignored": {groupFilePath: "empty_line.group"},

		// No group
		"No-Op for user with no groups and was in none": {groups: []string{}, groupFilePath: "no_users_in_our_groups.group"},
		"Remove user with no groups from existing ones": {groups: []string{}, groupFilePath: "user_in_both_groups.group"},

		// Error cases
		"Error on missing groups file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error when groups file is malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error on any unignored add gpasswd error":    {username: "gpasswdfail", groupFilePath: "no_users.group", wantErr: true},
		"Error on any unignored delete gpasswd error": {username: "gpasswdfail", groupFilePath: "gpasswdfail_in_deleted_group.group", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.groups == nil {
				tc.groups = []string{"localgroup1", "localgroup3"}
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

			err := localgroups.Update(tc.username, tc.groups, localgroups.WithGroupPath(groupFilePath), localgroups.WithGpasswdCmd(cmdArgs))
			if tc.wantErr {
				require.Error(t, err, "UpdateLocalGroups should have failed")
			} else {
				require.NoError(t, err, "UpdateLocalGroups should not have failed")
			}

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t))
		})
	}
}

func TestCleanLocalGroups(t *testing.T) {
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

		"Error if there's no active user":             {groupFilePath: "user_in_many_groups.group", getUsersReturn: []string{}, wantErr: true},
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

			cleanupOptions := []localgroups.Option{
				localgroups.WithGpasswdCmd(gpasswdCmd),
				localgroups.WithGroupPath(groupFilePath),
				localgroups.WithGetUsersFunc(func() []string { return tc.getUsersReturn }),
			}
			err := localgroups.Clean(cleanupOptions...)
			if tc.wantErr {
				require.Error(t, err, "CleanupLocalGroups should have failed")
			} else {
				require.NoError(t, err, "CleanupLocalGroups should not have failed")
			}

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t))
		})
	}
}

func TestCleanUserFromLocalGroups(t *testing.T) {
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

			cleanupOptions := []localgroups.Option{
				localgroups.WithGpasswdCmd(gpasswdCmd),
				localgroups.WithGroupPath(groupFilePath),
			}
			err := localgroups.CleanUser(tc.username, cleanupOptions...)
			if tc.wantErr {
				require.Error(t, err, "CleanUserFromLocalGroups should have failed")
			} else {
				require.NoError(t, err, "CleanUserFromLocalGroups should not have failed")
			}

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t))
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
