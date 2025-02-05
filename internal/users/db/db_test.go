package db_test

import (
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users/db"
)

func TestNew(t *testing.T) {
	t.Parallel()

	perm0644 := os.FileMode(0644)
	perm0000 := os.FileMode(0000)

	tests := map[string]struct {
		dbFile          string
		perm            *fs.FileMode
		corruptedDbFile bool

		wantErr bool
	}{
		"New_without_any_initialized_database":                   {},
		"New_with_already_existing_database":                     {dbFile: "multiple_users_and_groups"},
		"New_recreates_any_missing_buckets_and_delete_unknowns":  {dbFile: "database_with_unknown_bucket"},
		"New_removes_orphaned_user_records_from_UserByID_bucket": {dbFile: "orphaned_user_record"},

		"Error_on_non_existent_db_dir":                 {dbFile: "-", wantErr: true},
		"Error_on_corrupted_db_file":                   {corruptedDbFile: true, wantErr: true},
		"Error_on_invalid_permission_on_database_file": {dbFile: "multiple_users_and_groups", perm: &perm0644, wantErr: true},
		"Error_on_unreadable_database_file":            {dbFile: "multiple_users_and_groups", perm: &perm0000, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			dbDestPath := filepath.Join(dbDir, db.Z_ForTests_DBName())

			if tc.dbFile == "-" {
				err := os.RemoveAll(dbDir)
				require.NoError(t, err, "Setup: could not remove temporary database directory")
			} else if tc.dbFile != "" {
				db.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), dbDir)
			}
			if tc.corruptedDbFile {
				err := os.WriteFile(dbDestPath, []byte("corrupted"), 0600)
				require.NoError(t, err, "Setup: could not write corrupted database file")
			}

			if tc.perm != nil {
				err := os.Chmod(dbDestPath, *tc.perm)
				require.NoError(t, err, "Setup: could not change mode of database file")

				if *tc.perm == perm0644 {
					currentUser, err := user.Current()
					require.NoError(t, err)
					if os.Getenv("AUTHD_SKIP_ROOT_TESTS") != "" && currentUser.Username == "root" {
						t.Skip("Can't do permission checks as root")
					}
				}
			}

			c, err := db.New(dbDir)
			if tc.wantErr {
				require.Error(t, err, "New should return an error but didn't")
				return
			}
			require.NoError(t, err)
			defer c.Close()

			got, err := db.Z_ForTests_DumpNormalizedYAML(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdate(t, got)

			// check database permission
			fileInfo, err := os.Stat(dbDestPath)
			require.NoError(t, err, "Failed to stat database")
			perm := fileInfo.Mode().Perm()
			require.Equal(t, fs.FileMode(0600), perm, "Database permission should be 0600")
		})
	}
}

func TestUpdateUserEntry(t *testing.T) {
	t.Parallel()

	userCases := map[string]db.UserDB{
		"user1": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			// These values don't matter. We just want to make sure they are the same as the ones provided by the manager.
			LastPwdChange: -1, MaxPwdAge: -1, PwdWarnPeriod: -1, PwdInactivity: -1, MinPwdAge: -1, ExpirationDate: -1,
		},
		"user1-new-attributes": {
			Name:  "user1",
			UID:   1111,
			Gecos: "New user1 gecos",
			Dir:   "/home/user1",
			Shell: "/bin/dash",
			// These values don't matter. We just want to make sure they are the same as the ones provided by the manager.
			LastPwdChange: -1, MaxPwdAge: -1, PwdWarnPeriod: -1, PwdInactivity: -1, MinPwdAge: -1, ExpirationDate: -1,
		},
		"user1-new-name": {
			Name:  "newuser1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			// These values don't matter. We just want to make sure they are the same as the ones provided by the manager.
			LastPwdChange: -1, MaxPwdAge: -1, PwdWarnPeriod: -1, PwdInactivity: -1, MinPwdAge: -1, ExpirationDate: -1,
		},
		"user1-new-homedir": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/new/home/user1",
			Shell: "/bin/bash",
			// These values don't matter. We just want to make sure they are the same as the ones provided by the manager.
			LastPwdChange: -1, MaxPwdAge: -1, PwdWarnPeriod: -1, PwdInactivity: -1, MinPwdAge: -1, ExpirationDate: -1,
		},
		"user1-without-gecos": {
			Name:  "user1",
			UID:   1111,
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			// These values don't matter. We just want to make sure they are the same as the ones provided by the manager.
			LastPwdChange: -1, MaxPwdAge: -1, PwdWarnPeriod: -1, PwdInactivity: -1, MinPwdAge: -1, ExpirationDate: -1,
		},
		"user3": {
			Name:  "user3",
			UID:   3333,
			Gecos: "User3 gecos",
			Dir:   "/home/user3",
			Shell: "/bin/zsh",
			// These values don't matter. We just want to make sure they are the same as the ones provided by the manager.
			LastPwdChange: -1, MaxPwdAge: -1, PwdWarnPeriod: -1, PwdInactivity: -1, MinPwdAge: -1, ExpirationDate: -1,
		},
	}
	groupCases := map[string]db.GroupDB{
		"group1":              db.NewGroupDB("group1", 11111, "12345678", nil),
		"newgroup1-same-ugid": db.NewGroupDB("newgroup1-same-ugid", 11111, "12345678", nil),
		"newgroup1-diff-ugid": db.NewGroupDB("newgroup1-diff-ugid", 11111, "99999999", nil),
		"group2":              db.NewGroupDB("group2", 22222, "56781234", nil),
		"group3":              db.NewGroupDB("group3", 33333, "34567812", nil),
	}

	tests := map[string]struct {
		userCase    string
		groupCases  []string
		localGroups []string
		dbFile      string

		wantErr bool
	}{
		// New user
		"Insert_new_user":                              {},
		"Update_last_login_time_for_user":              {dbFile: "one_user_and_group"},
		"Insert_new_user_without_optional_gecos_field": {userCase: "user1-without-gecos"},

		// User and Group updates
		"Update_user_by_changing_attributes":                      {userCase: "user1-new-attributes", dbFile: "one_user_and_group"},
		"Update_user_does_not_change_homedir_if_it_exists":        {userCase: "user1-new-homedir", dbFile: "one_user_and_group"},
		"Update_user_by_removing_optional_gecos_field_if_not_set": {userCase: "user1-without-gecos", dbFile: "one_user_and_group"},

		// Group updates
		"Update_user_by_adding_a_new_group":         {groupCases: []string{"group1", "group2"}, dbFile: "one_user_and_group"},
		"Update_user_by_adding_a_new_default_group": {groupCases: []string{"group2", "group1"}, dbFile: "one_user_and_group"},
		"Update_user_by_renaming_a_group":           {groupCases: []string{"newgroup1-same-ugid"}, dbFile: "one_user_and_group"},
		"Remove_group_from_user":                    {groupCases: []string{"group2"}, dbFile: "one_user_and_group"},
		"Update_user_by_adding_a_new_local_group":   {localGroups: []string{"localgroup1"}, dbFile: "one_user_and_group"},

		// Multi users handling
		"Update_only_user_even_if_we_have_multiple_of_them":     {dbFile: "multiple_users_and_groups"},
		"Add_user_to_group_from_another_user":                   {groupCases: []string{"group1", "group2"}, dbFile: "multiple_users_and_groups"},
		"Remove_user_from_a_group_still_part_from_another_user": {userCase: "user3", groupCases: []string{"group3"}, dbFile: "multiple_users_and_groups"},

		// Allowed inconsistent cases
		"Invalid_value_entry_in_groupByName_recreates_entries":                         {dbFile: "invalid_entry_in_groupByName"},
		"Invalid_value_entry_in_userByName_recreates_entries":                          {dbFile: "invalid_entry_in_userByName"},
		"Invalid_value_entries_in_other_user_and_groups_do_not_impact_current_request": {dbFile: "invalid_entries_but_user_and_group1"},

		// Renaming errors
		"Error_when_user_has_conflicting_uid": {userCase: "user1-new-name", dbFile: "one_user_and_group", wantErr: true},

		// Error cases
		"Error_on_invalid_value_entry_in_groupByID":                                 {dbFile: "invalid_entry_in_groupByID", wantErr: true},
		"Error_on_invalid_value_entry_in_userByID":                                  {dbFile: "invalid_entry_in_userByID", wantErr: true},
		"Error_on_invalid_value_entry_in_userToGroups":                              {dbFile: "invalid_entry_in_userToGroups", wantErr: true},
		"Error_on_invalid_value_entry_in_groupToUsers":                              {dbFile: "invalid_entry_in_groupToUsers", wantErr: true},
		"Error_on_invalid_value_entry_in_groupToUsers_for_user_dropping_from_group": {dbFile: "invalid_entry_in_groupToUsers_secondary_group", wantErr: true},
		"Error_on_invalid_value_entry_in_groupByID_for_user_dropping_from_group":    {dbFile: "invalid_entry_in_groupByID_secondary_group", wantErr: true},
		"Error_when_group_has_conflicting_gid":                                      {groupCases: []string{"newgroup1-diff-ugid"}, dbFile: "one_user_and_group", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			if tc.userCase == "" {
				tc.userCase = "user1"
			}
			if tc.groupCases == nil {
				tc.groupCases = []string{"group1"}
			}

			user := userCases[tc.userCase]
			var groups []db.GroupDB
			for _, g := range tc.groupCases {
				groups = append(groups, groupCases[g])
			}
			user.GID = groups[0].GID

			err := c.UpdateUserEntry(user, groups, tc.localGroups)
			if tc.wantErr {
				require.Error(t, err, "UpdateFromUserInfo should return an error but didn't")
				return
			}
			require.NoError(t, err)

			got, err := db.Z_ForTests_DumpNormalizedYAML(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdate(t, got)
		})
	}
}

func TestUserByID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Get_existing_user": {dbFile: "one_user_and_group"},

		"Error_on_missing_user":           {wantErrType: db.NoDataFoundError{}},
		"Error_on_invalid_database_entry": {dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.UserByID(1111)
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestUserByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
		wantErr     bool
	}{
		"Get_existing_user": {dbFile: "one_user_and_group"},

		"Error_on_missing_user":           {wantErrType: db.NoDataFoundError{}},
		"Error_on_invalid_database_entry": {dbFile: "invalid_entry_in_userByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.UserByName("user1")
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestAllUsers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr bool
	}{
		"Get_one_user":       {dbFile: "one_user_and_group"},
		"Get_multiple_users": {dbFile: "multiple_users_and_groups"},

		"Get_users_only_rely_on_valid_userByID": {dbFile: "partially_valid_multiple_users_and_groups_only_userByID"},

		"Error_on_some_invalid_users_entry": {dbFile: "invalid_entries_but_user_and_group1", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.AllUsers()
			requireGetAssertions(t, got, tc.wantErr, nil, err)
		})
	}
}

func TestGroupByID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Get_existing_group": {dbFile: "one_user_and_group"},

		"Error_on_missing_group":          {wantErrType: db.NoDataFoundError{}},
		"Error_on_invalid_database_entry": {dbFile: "invalid_entry_in_groupByID", wantErr: true},
		"Error_as_missing_userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.GroupByID(11111)
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestGroupByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Get_existing_group": {dbFile: "one_user_and_group"},

		"Error_on_missing_group":          {wantErrType: db.NoDataFoundError{}},
		"Error_on_invalid_database_entry": {dbFile: "invalid_entry_in_groupByName", wantErr: true},
		"Error_as_missing_userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.GroupByName("group1")
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestUserGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Get_groups_of_existing_user": {dbFile: "one_user_and_group"},

		"Error_on_missing_user":           {wantErrType: db.NoDataFoundError{}},
		"Error_on_invalid_database_entry": {dbFile: "invalid_entry_in_userToGroups", wantErr: true},
		"Error_on_missing_groupByID":      {dbFile: "invalid_entry_in_groupByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.UserGroups(1111)
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestAllGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Get_one_group":       {dbFile: "one_user_and_group"},
		"Get_multiple_groups": {dbFile: "multiple_users_and_groups"},

		"Get_groups_rely_on_groupByID_groupToUsers_UserByID": {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers_UserByID"},

		"Error_on_some_invalid_groups_entry":     {dbFile: "invalid_entries_but_user_and_group1", wantErr: true},
		"Error_as_not_only_relying_on_groupByID": {dbFile: "partially_valid_multiple_users_and_groups_only_groupByID", wantErr: true},
		"Error_as_missing_userByID":              {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			got, err := c.AllGroups()
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestUpdateBrokerForUser(t *testing.T) {
	t.Parallel()

	c := initDB(t, "one_user_and_group")

	// Update broker for existent user
	err := c.UpdateBrokerForUser("user1", "ExampleBrokerID")
	require.NoError(t, err, "UpdateBrokerForUser for an existent user should not return an error")

	// Error when updating broker for nonexistent user
	err = c.UpdateBrokerForUser("nonexistent", "ExampleBrokerID")
	require.Error(t, err, "UpdateBrokerForUser for a nonexistent user should return an error")
}

func TestBrokerForUser(t *testing.T) {
	t.Parallel()

	c := initDB(t, "multiple_users_and_groups")

	// Get existing BrokerForUser entry
	gotID, err := c.BrokerForUser("user1")
	require.NoError(t, err, "BrokerForUser for an existent user should not return an error")
	golden.CheckOrUpdate(t, gotID)

	// Get unassigned broker to existent user
	gotID, err = c.BrokerForUser("userwithoutbroker")
	require.NoError(t, err, "BrokerForUser for an existent user should not return an error")
	require.Empty(t, gotID, "BrokerForUser should return empty broker ID for unassigned broker to existent user")

	// Error when user does not exist
	gotID, err = c.BrokerForUser("nonexistent")
	require.Error(t, err, "BrokerForUser for a nonexistent user should return an error")
	require.Empty(t, gotID, "BrokerForUser should return empty broker ID when user entry does not exist")
}

func TestRemoveDb(t *testing.T) {
	t.Parallel()

	c := initDB(t, "multiple_users_and_groups")
	dbDir := filepath.Dir(c.DbPath())

	// First call should return with no error.
	require.NoError(t, db.RemoveDb(dbDir), "RemoveDb should not return an error on the first call")
	require.NoFileExists(t, dbDir, "RemoveDb should remove the database file")

	// Second call should return ErrNotExist as the database file was already removed.
	require.ErrorIs(t, db.RemoveDb(dbDir), fs.ErrNotExist, "RemoveDb should return os.ErrNotExist on the second call")
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Deleting_last_user_from_a_group_keeps_the_group_record":  {dbFile: "one_user_and_group"},
		"Deleting_existing_user_keeps_other_group_members_intact": {dbFile: "multiple_users_and_groups"},

		"Error_on_missing_user":           {wantErrType: db.NoDataFoundError{}},
		"Error_on_invalid_database_entry": {dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initDB(t, tc.dbFile)

			err := c.DeleteUser(1111)
			if tc.wantErr {
				require.Error(t, err, "DeleteUser should return an error but didn't")
				return
			}
			if tc.wantErrType != nil {
				require.ErrorIs(t, err, tc.wantErrType, "DeleteUser should return expected error")
				return
			}
			require.NoError(t, err)

			got, err := db.Z_ForTests_DumpNormalizedYAML(c)
			require.NoError(t, err, "Created database should be valid yaml content")
			golden.CheckOrUpdate(t, got)
		})
	}
}

// initDB returns a new database ready to be used alongside its database directory.
func initDB(t *testing.T, dbFile string) (c *db.Database) {
	t.Helper()

	dbDir := t.TempDir()
	if dbFile != "" {
		db.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", dbFile+".db.yaml"), dbDir)
	}

	c, err := db.New(dbDir)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	return c
}

func requireGetAssertions[E any](t *testing.T, got E, wantErr bool, wantErrType, err error) {
	t.Helper()

	if wantErrType != nil {
		require.ErrorIs(t, err, wantErrType, "Get should return expected error")
		return
	}
	if wantErr {
		require.Error(t, err, "Get should return an error")
		return
	}
	require.NoError(t, err)

	golden.CheckOrUpdateYAML(t, got)
}
