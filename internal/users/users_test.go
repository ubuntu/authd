package users_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
	usertests "github.com/ubuntu/authd/internal/users/tests"
)

func TestUpdateLocalGroups(t *testing.T) {
	t.Parallel()

	gid1 := 42424242

	tests := map[string]struct {
		user string

		groups        []users.GroupInfo
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
		"No-Op for user with no groups and was in none": {groups: []users.GroupInfo{}, groupFilePath: "no_users_in_our_groups.group"},
		"Remove user with no groups from existing ones": {groups: []users.GroupInfo{}, groupFilePath: "user_in_both_groups.group"},

		// Error cases
		"Error on missing groups file":                {groupFilePath: "does_not_exists.group", wantErr: true},
		"Error when groups file is malformed":         {groupFilePath: "malformed_file.group", wantErr: true},
		"Error on any unignored add gpasswd error":    {user: "gpasswdfail", groupFilePath: "no_users.group", wantErr: true},
		"Error on any unignored delete gpasswd error": {user: "gpasswdfail", groupFilePath: "gpasswdfail_in_deleted_group.group", wantErr: true},
		"Error on empty user name":                    {user: "-", groupFilePath: "no_users.group", wantErr: true},
		"Error on empty group name":                   {groups: []users.GroupInfo{{Name: ""}}, groupFilePath: "no_users.group", wantErr: true},
	}
	for name, tc := range tests {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.groups == nil {
				tc.groups = []users.GroupInfo{
					{ // this group should be ignored (still on the file to ensure we don’t match it)
						Name: "cloudgroup1",
						GID:  &gid1,
					},
					{
						Name: "localgroup1",
					},
					{
						Name: "localgroup3",
					},
				}
			}

			switch tc.user {
			case "":
				tc.user = "myuser"
			case "-":
				tc.user = ""
			}

			u := users.UserInfo{
				Name:   tc.user,
				Groups: tc.groups,
			}

			destCmdsFile := filepath.Join(t.TempDir(), "gpasswd.output")

			groupFilePath := filepath.Join("testdata", tc.groupFilePath)
			cmdArgs := []string{"env", "GO_WANT_HELPER_PROCESS=1",
				fmt.Sprintf("GO_WANT_HELPER_PROCESS_DEST=%s", destCmdsFile),
				fmt.Sprintf("GO_WANT_HELPER_PROCESS_GROUPFILE=%s", groupFilePath),
				os.Args[0], "-test.run=TestMockgpasswd", "--"}

			err := u.UpdateLocalGroups(users.WithGroupPath(groupFilePath), users.WithGpasswdCmd(cmdArgs))
			if tc.wantErr {
				require.Error(t, err, "UpdateLocalGroups should have failed")
			} else {
				require.NoError(t, err, "UpdateLocalGroups should not have failed")
			}

			// Always check the golden files missing for no-op too on error
			referenceFilePath := testutils.GoldenPath(t)
			if testutils.Update() {
				// The file may already not exists.
				_ = os.Remove(testutils.GoldenPath(t))
				referenceFilePath = destCmdsFile
			}

			var shouldExists bool
			if _, err := os.Stat(referenceFilePath); err == nil {
				shouldExists = true
			}
			if !shouldExists {
				require.NoFileExists(t, destCmdsFile, "UpdateLocalGroups should not call gpasswd by did")
				return
			}

			got := usertests.IdemnpotentOutputFromGPasswd(t, destCmdsFile)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "UpdateLocalGroups should do the expected gpasswd operation, but did not")
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	usertests.Mockgpasswd(t)
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	testutils.InstallUpdateFlag()
	flag.Parse()

	m.Run()
}
