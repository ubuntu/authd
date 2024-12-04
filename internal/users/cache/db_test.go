package cache_test

import (
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users/cache"
	cachetestutils "github.com/ubuntu/authd/internal/users/cache/testutils"
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
		"New without any initialized database":                   {},
		"New with already existing database":                     {dbFile: "multiple_users_and_groups"},
		"New recreates any missing buckets and delete unknowns":  {dbFile: "database_with_unknown_bucket"},
		"New removes orphaned user records from UserByID bucket": {dbFile: "orphaned_user_record"},

		"Error on cacheDir non existent cacheDir":      {dbFile: "-", wantErr: true},
		"Error on corrupted db file":                   {corruptedDbFile: true, wantErr: true},
		"Error on invalid permission on database file": {dbFile: "multiple_users_and_groups", perm: &perm0644, wantErr: true},
		"Error on unreadable database file":            {dbFile: "multiple_users_and_groups", perm: &perm0000, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			dbDestPath := filepath.Join(cacheDir, cachetestutils.DbName)

			if tc.dbFile == "-" {
				err := os.RemoveAll(cacheDir)
				require.NoError(t, err, "Setup: could not remove temporary cache directory")
			} else if tc.dbFile != "" {
				cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
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

			c, err := cache.New(cacheDir)
			if tc.wantErr {
				require.Error(t, err, "New should return an error but didn't")
				return
			}
			require.NoError(t, err)
			defer c.Close()

			got, err := cachetestutils.DumpNormalizedYAML(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			want := golden.LoadWithUpdate(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

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

	userCases := map[string]cache.UserDB{
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
	groupCases := map[string]cache.GroupDB{
		"group1":    cache.NewGroupDB("group1", 11111, nil),
		"newgroup1": cache.NewGroupDB("newgroup1", 11111, nil),
		"group2":    cache.NewGroupDB("group2", 22222, nil),
		"group3":    cache.NewGroupDB("group3", 33333, nil),
	}

	tests := map[string]struct {
		userCase   string
		groupCases []string
		dbFile     string

		wantErr bool
	}{
		// New user
		"Insert new user":                              {},
		"Update last login time for user":              {dbFile: "one_user_and_group"},
		"Insert new user without optional gecos field": {userCase: "user1-without-gecos"},

		// User and Group updates
		"Update user by changing attributes":                      {userCase: "user1-new-attributes", dbFile: "one_user_and_group"},
		"Update user does not change homedir if it exists":        {userCase: "user1-new-homedir", dbFile: "one_user_and_group"},
		"Update user by removing optional gecos field if not set": {userCase: "user1-without-gecos", dbFile: "one_user_and_group"},

		// Group updates
		"Update user by adding a new group":         {groupCases: []string{"group1", "group2"}, dbFile: "one_user_and_group"},
		"Update user by adding a new default group": {groupCases: []string{"group2", "group1"}, dbFile: "one_user_and_group"},
		"Remove group from user":                    {groupCases: []string{"group2"}, dbFile: "one_user_and_group"},

		// Multi users handling
		"Update only user even if we have multiple of them":     {dbFile: "multiple_users_and_groups"},
		"Add user to group from another user":                   {groupCases: []string{"group1", "group2"}, dbFile: "multiple_users_and_groups"},
		"Remove user from a group still part from another user": {userCase: "user3", groupCases: []string{"group3"}, dbFile: "multiple_users_and_groups"},

		// Allowed inconsistent cases
		"Invalid value entry in groupByName recreates entries":                         {dbFile: "invalid_entry_in_groupByName"},
		"Invalid value entry in userByName recreates entries":                          {dbFile: "invalid_entry_in_userByName"},
		"Invalid value entries in other user and groups do not impact current request": {dbFile: "invalid_entries_but_user_and_group1"},

		// Renaming errors
		"Error when user has conflicting uid":  {userCase: "user1-new-name", dbFile: "one_user_and_group", wantErr: true},
		"Error when group has conflicting gid": {groupCases: []string{"newgroup1"}, dbFile: "one_user_and_group", wantErr: true},

		// Error cases
		"Error on invalid value entry in groupByID":                                 {dbFile: "invalid_entry_in_groupByID", wantErr: true},
		"Error on invalid value entry in userByID":                                  {dbFile: "invalid_entry_in_userByID", wantErr: true},
		"Error on invalid value entry in userToGroups":                              {dbFile: "invalid_entry_in_userToGroups", wantErr: true},
		"Error on invalid value entry in groupToUsers":                              {dbFile: "invalid_entry_in_groupToUsers", wantErr: true},
		"Error on invalid value entry in groupToUsers for user dropping from group": {dbFile: "invalid_entry_in_groupToUsers_secondary_group", wantErr: true},
		"Error on invalid value entry in groupByID for user dropping from group":    {dbFile: "invalid_entry_in_groupByID_secondary_group", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

			if tc.userCase == "" {
				tc.userCase = "user1"
			}
			if tc.groupCases == nil {
				tc.groupCases = []string{"group1"}
			}

			user := userCases[tc.userCase]
			var groups []cache.GroupDB
			for _, g := range tc.groupCases {
				groups = append(groups, groupCases[g])
			}
			user.GID = groups[0].GID

			err := c.UpdateUserEntry(user, groups)
			if tc.wantErr {
				require.Error(t, err, "UpdateFromUserInfo should return an error but didn't")
				return
			}
			require.NoError(t, err)

			got, err := cachetestutils.DumpNormalizedYAML(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			want := golden.LoadWithUpdate(t, got)
			require.Equal(t, want, got, "Did not get expected database content")
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
		"Get existing user": {dbFile: "one_user_and_group"},

		"Error on missing user":           {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

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
		"Get existing user": {dbFile: "one_user_and_group"},

		"Error on missing user":           {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

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
		"Get one user":       {dbFile: "one_user_and_group"},
		"Get multiple users": {dbFile: "multiple_users_and_groups"},

		"Get users only rely on valid userByID": {dbFile: "partially_valid_multiple_users_and_groups_only_userByID"},

		"Error on some invalid users entry": {dbFile: "invalid_entries_but_user_and_group1", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

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
		"Get existing group": {dbFile: "one_user_and_group"},

		"Error on missing group":          {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_groupByID", wantErr: true},
		"Error as missing userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

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
		"Get existing group": {dbFile: "one_user_and_group"},

		"Error on missing group":          {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_groupByName", wantErr: true},
		"Error as missing userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

			got, err := c.GroupByName("group1")
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
		"Get one group":       {dbFile: "one_user_and_group"},
		"Get multiple groups": {dbFile: "multiple_users_and_groups"},

		"Get groups rely on groupByID groupToUsers UserByID": {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers_UserByID"},

		"Error on some invalid groups entry":     {dbFile: "invalid_entries_but_user_and_group1", wantErr: true},
		"Error as not only relying on groupByID": {dbFile: "partially_valid_multiple_users_and_groups_only_groupByID", wantErr: true},
		"Error as missing userByID":              {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

			got, err := c.AllGroups()
			requireGetAssertions(t, got, tc.wantErr, tc.wantErrType, err)
		})
	}
}

func TestUpdateBrokerForUser(t *testing.T) {
	t.Parallel()

	c := initCache(t, "one_user_and_group")

	// Update broker for existent user
	err := c.UpdateBrokerForUser("user1", "ExampleBrokerID")
	require.NoError(t, err, "UpdateBrokerForUser for an existent user should not return an error")

	// Error when updating broker for nonexistent user
	err = c.UpdateBrokerForUser("nonexistent", "ExampleBrokerID")
	require.Error(t, err, "UpdateBrokerForUser for a nonexistent user should return an error")
}

func TestBrokerForUser(t *testing.T) {
	t.Parallel()

	c := initCache(t, "multiple_users_and_groups")

	// Get existing BrokerForUser entry
	gotID, err := c.BrokerForUser("user1")
	require.NoError(t, err, "BrokerForUser for an existent user should not return an error")
	wantID := golden.LoadWithUpdate(t, gotID)
	require.Equal(t, wantID, gotID, "BrokerForUser should return expected broker ID")

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

	c := initCache(t, "multiple_users_and_groups")
	cacheDir := filepath.Dir(c.DbPath())

	// First call should return with no error.
	require.NoError(t, cache.RemoveDb(cacheDir), "RemoveDb should not return an error on the first call")
	require.NoFileExists(t, cacheDir, "RemoveDb should remove the database file")

	// Second call should return ErrNotExist as the database file was already removed.
	require.ErrorIs(t, cache.RemoveDb(cacheDir), fs.ErrNotExist, "RemoveDb should return os.ErrNotExist on the second call")
}

func TestDeleteUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Delete existing user":                            {dbFile: "one_user_and_group"},
		"Delete existing user keeping other users intact": {dbFile: "multiple_users_and_groups"},

		"Error on missing user":           {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := initCache(t, tc.dbFile)

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

			got, err := cachetestutils.DumpNormalizedYAML(c)
			require.NoError(t, err, "Created database should be valid yaml content")
			want := golden.LoadWithUpdate(t, got)
			require.Equal(t, want, got, "Did not get expected database content")
		})
	}
}

// initCache returns a new cache ready to be used alongside its cache directory.
func initCache(t *testing.T, dbFile string) (c *cache.Cache) {
	t.Helper()

	cacheDir := t.TempDir()
	if dbFile != "" {
		cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", dbFile+".db.yaml"), cacheDir)
	}

	c, err := cache.New(cacheDir)
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

	want := golden.LoadWithUpdateYAML(t, got)
	require.Equal(t, want, got, "Did not get expected database entry")
}
