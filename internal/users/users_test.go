package users_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
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
			f, err := os.Create(destCmdsFile)
			require.NoError(t, err, "Setup: dest trace file was not created successfully")
			require.NoError(t, f.Close(), "Setup: could not close dest trace file")

			groupFilePath := filepath.Join("testdata", tc.groupFilePath)
			cmdArgs := []string{"env", fmt.Sprintf("GO_WANT_HELPER_PROCESS_DEST=%s", destCmdsFile),
				fmt.Sprintf("GO_WANT_HELPER_PROCESS_GROUPFILE=%s", groupFilePath),
				os.Args[0], "-test.run=TestMockgpasswd", "--"}

			err = u.UpdateLocalGroups(users.WithGroupPath(groupFilePath), users.WithGpasswdCmd(cmdArgs))
			if tc.wantErr {
				require.Error(t, err, "UpdateLocalGroups should have failed")
			} else {
				require.NoError(t, err, "UpdateLocalGroups should not have failed")
			}

			// Always check the golden files for no-op too on error
			got, err := os.ReadFile(destCmdsFile)
			require.NoError(t, err, "Teardown: could not read dest trace file")

			// need to sort out got operation
			ops := strings.Split(string(got), "\n")
			slices.Sort(ops)
			gotStr := strings.TrimSpace(strings.Join(ops, "\n"))

			want := testutils.LoadWithUpdateFromGolden(t, gotStr)
			require.Equal(t, want, gotStr, "UpdateLocalGroups should do the expected gpasswd operation, but did not")
		})
	}
}

func TestMockgpasswd(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_DEST") == "" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] != "--" {
			args = args[1:]
			continue
		}
		args = args[1:]
		break
	}

	d, err := os.ReadFile(os.Getenv("GO_WANT_HELPER_PROCESS_GROUPFILE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mock: error reading group file: %v", err)
		os.Exit(1)
	}

	// Error if the group is not in the groupfile (we don’t handle substrings in the mock)
	group := args[len(args)-1]
	if !strings.Contains(string(d), group+":") {
		fmt.Fprintf(os.Stderr, "Error: %s in not in the group file", group)
		os.Exit(3)
	}

	// Other error
	if args[1] == "gpasswdfail" {
		fmt.Fprint(os.Stderr, "Error requested in mock")
		os.Exit(1)
	}

	dest := os.Getenv("GO_WANT_HELPER_PROCESS_DEST")
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mock: error opening file in append mode: %v", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Write([]byte(strings.Join(args, " ") + "\n")); err != nil {
		fmt.Fprintf(os.Stderr, "Mock: error while writing in file: %v", err)
		os.Exit(1)
	}
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()
	flag.Parse()

	m.Run()
}
