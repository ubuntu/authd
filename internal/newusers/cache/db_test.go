package cache_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
	_ "unsafe"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/newusers/cache"
	cachetests "github.com/ubuntu/authd/internal/newusers/cache/tests"
	"github.com/ubuntu/authd/internal/testutils"
)

func TestNew(t *testing.T) {
	t.Parallel()

	perm0644 := os.FileMode(0644)
	perm0000 := os.FileMode(0000)

	tests := map[string]struct {
		dbFile          string
		dirtyFlag       bool
		perm            *fs.FileMode
		corruptedDbFile bool
		markDirty       bool

		expirationDate  string
		skipCleanOnNew  bool
		cleanupInterval int
		procDir         string

		wantErr bool
	}{
		"New without any initialized database": {},
		"New with already existing database":   {dbFile: "multiple_users_and_groups"},

		"Error on cacheDir non existent cacheDir":      {dbFile: "-", wantErr: true},
		"Error on invalid permission on database file": {dbFile: "multiple_users_and_groups", perm: &perm0644, wantErr: true},
		"Error on unreadable database file":            {dbFile: "multiple_users_and_groups", perm: &perm0000, wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			dbDestPath := filepath.Join(cacheDir, cachetests.DbName)

			if tc.dbFile == "-" {
				err := os.RemoveAll(cacheDir)
				require.NoError(t, err, "Setup: could not remove temporary cache directory")
			} else if tc.dbFile != "" {
				createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
			}

			if tc.perm != nil {
				err := os.Chmod(dbDestPath, *tc.perm)
				require.NoError(t, err, "Setup: could not change mode of database file")
			}

			c, err := cache.New(cacheDir)
			if tc.wantErr {
				require.Error(t, err, "New should return an error but didn't")
				return
			}
			require.NoError(t, err)
			defer c.Close()

			if tc.cleanupInterval > 0 {
				// Wait for the clean up routine to start
				time.Sleep(time.Duration(tc.cleanupInterval*2) * time.Second)
			}

			got, err := cachetests.DumpToYaml(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

			// check database permission
			fileInfo, err := os.Stat(dbDestPath)
			require.NoError(t, err, "Failed to stat database")
			perm := fileInfo.Mode().Perm()
			require.Equal(t, fs.FileMode(0600), perm, "Database permission should be 0600")

			// database should not be marked as dirty
			requireNoDirtyFileInDir(t, cacheDir)
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
			Name:  "newuser1",
			UID:   1111,
			Gecos: "New user1 gecos",
			Dir:   "/home/newuser1",
			Shell: "/bin/dash",
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

		wantClearDB bool
		wantErr     bool
	}{
		// New user
		"Insert new user":                              {},
		"Update last login time for user":              {dbFile: "one_user_and_group"},
		"Insert new user without optional gecos field": {userCase: "user1-without-gecos"},

		// User and Group renames
		"Update user by changing attributes":                      {userCase: "user1-new-attributes", dbFile: "one_user_and_group"},
		"Update user by removing optional gecos field if not set": {userCase: "user1-without-gecos", dbFile: "one_user_and_group"},
		"Update group by changing attributes":                     {groupCases: []string{"newgroup1"}, dbFile: "one_user_and_group"},

		// Group updates
		"Update user by adding a new group":         {groupCases: []string{"group1", "group2"}, dbFile: "one_user_and_group"},
		"Update user by adding a new default group": {groupCases: []string{"group2", "group1"}, dbFile: "one_user_and_group"},
		"Remove group from user":                    {groupCases: []string{"group2"}, dbFile: "one_user_and_group"},

		// Multi users handling
		"Update only user even if we have multiple of them":     {dbFile: "multiple_users_and_groups"},
		"Add user to group from another user":                   {groupCases: []string{"group1", "group2"}, dbFile: "multiple_users_and_groups"},
		"Remove user from a group still part from another user": {userCase: "user3", groupCases: []string{"group3"}, dbFile: "multiple_users_and_groups"},

		// Allowed inconsistent cases
		"Invalid value entry in groupByID but user restating group recreates entries": {dbFile: "invalid_entry_in_groupByID"},
		"Invalid value entry in userByID recreates entries":                           {dbFile: "invalid_entry_in_userByID"},
		"Invalid value entry in groupByName recreates entries":                        {dbFile: "invalid_entry_in_groupByName"},
		"Invalid value entry in userByName recreates entries":                         {dbFile: "invalid_entry_in_userByName"},
		"Invalid value entries in other user and groups don't impact current request": {dbFile: "invalid_entries_but_user_and_group1"},

		// Error cases
		"Error on invalid value entry in userToGroups clear database":                              {dbFile: "invalid_entry_in_userToGroups", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupToUsers clear database":                              {dbFile: "invalid_entry_in_groupToUsers", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupToUsers for user dropping from group clear database": {dbFile: "invalid_entry_in_groupToUsers_secondary_group", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupByID for user dropping from group clear database":    {dbFile: "invalid_entry_in_groupByID_secondary_group", wantErr: true, wantClearDB: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

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
				if tc.wantClearDB {
					require.ErrorIs(t, err, cache.ErrNeedsClearing, "UpdateFromUserInfo should return ErrNeedsClearing")
				}
				return
			}
			require.NoError(t, err)

			requireNoDirtyFileInDir(t, cacheDir)

			got, err := cachetests.DumpToYaml(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Did not get expected database content")
		})
	}
}

func TestUserByID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
	}{
		"Get existing user": {dbFile: "one_user_and_group"},

		"Error on missing user":           {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByID", wantErrType: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.UserByID(1111)
			requireGetAssertions(t, got, tc.wantErrType, err, cacheDir)
		})
	}
}

func TestUserByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
	}{
		"Get existing user": {dbFile: "one_user_and_group"},

		"Error on missing user":           {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByName", wantErrType: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.UserByName("user1")
			requireGetAssertions(t, got, tc.wantErrType, err, cacheDir)
		})
	}
}

func TestAllUsers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
	}{
		"Get one user":       {dbFile: "one_user_and_group"},
		"Get multiple users": {dbFile: "multiple_users_and_groups"},

		"Get users only rely on valid userByID": {dbFile: "partially_valid_multiple_users_and_groups_only_userByID"},

		"Error on some invalid users entry": {dbFile: "invalid_entries_but_user_and_group1", wantErrType: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.AllUsers()
			requireGetAssertions(t, got, tc.wantErrType, err, cacheDir)
		})
	}
}

func TestGroupByID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
	}{
		"Get existing group": {dbFile: "one_user_and_group"},

		"Error on missing group":          {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_groupByID", wantErrType: cache.ErrNeedsClearing},
		"Error as missing userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErrType: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.GroupByID(11111)
			requireGetAssertions(t, got, tc.wantErrType, err, cacheDir)
		})
	}
}

func TestGroupByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
	}{
		"Get existing group": {dbFile: "one_user_and_group"},

		"Error on missing group":          {wantErrType: cache.NoDataFoundError{}},
		"Error on invalid database entry": {dbFile: "invalid_entry_in_groupByName", wantErrType: cache.ErrNeedsClearing},
		"Error as missing userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErrType: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.GroupByName("group1")
			requireGetAssertions(t, got, tc.wantErrType, err, cacheDir)
		})
	}
}

func TestAllGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErrType error
	}{
		"Get one group":       {dbFile: "one_user_and_group"},
		"Get multiple groups": {dbFile: "multiple_users_and_groups"},

		"Get groups rely on groupByID, groupToUsers, UserByID": {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers_UserByID"},

		"Error on some invalid groups entry":     {dbFile: "invalid_entries_but_user_and_group1", wantErrType: cache.ErrNeedsClearing},
		"Error as not only relying on groupByID": {dbFile: "partially_valid_multiple_users_and_groups_only_groupByID", wantErrType: cache.ErrNeedsClearing},
		"Error as missing userByID":              {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErrType: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.AllGroups()
			requireGetAssertions(t, got, tc.wantErrType, err, cacheDir)
		})
	}
}

func TestUpdateBrokerForUser(t *testing.T) {
	t.Parallel()

	c, _ := initCache(t, "one_user_and_group")

	// Update broker for existent user
	err := c.UpdateBrokerForUser("user1", "ExampleBrokerID")
	require.NoError(t, err, "UpdateBrokerForUser for an existent user should not return an error")

	// Error when updating broker for nonexistent user
	err = c.UpdateBrokerForUser("nonexistent", "ExampleBrokerID")
	require.Error(t, err, "UpdateBrokerForUser for a nonexistent user should return an error")
}

func TestBrokerForUser(t *testing.T) {
	t.Parallel()

	c, _ := initCache(t, "multiple_users_and_groups_with_brokers")

	// Get existing BrokerForUser entry
	gotID, err := c.BrokerForUser("user1")
	require.NoError(t, err, "BrokerForUser for an existent entry should not return an error")
	wantID := testutils.LoadWithUpdateFromGolden(t, gotID)
	require.Equal(t, wantID, gotID, "BrokerForUser should return expected broker ID")

	// Error when entry does not exist
	gotID, err = c.BrokerForUser("nonexistent")
	require.Error(t, err, "BrokerForUser for a nonexistent entry should return an error")
	require.Empty(t, gotID, "BrokerForUser should return empty string when entry does not exist")
}

func createDBFile(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = cachetests.DbfromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}

func requireNoDirtyFileInDir(t *testing.T, cacheDir string) {
	t.Helper()

	require.NoFileExists(t, filepath.Join(cacheDir, cachetests.DirtyFlagDbName), "Dirty flag should have been removed")
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()

	os.Exit(m.Run())
}

// initCache returns a new cache ready to be used alongside its cache directory.
func initCache(t *testing.T, dbFile string) (c *cache.Cache, cacheDir string) {
	t.Helper()

	cacheDir = t.TempDir()
	if dbFile != "" {
		createDBFile(t, filepath.Join("testdata", dbFile+".db.yaml"), cacheDir)
	}

	c, err := cache.New(cacheDir)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	return c, cacheDir
}

func requireGetAssertions[E any](t *testing.T, got E, wantErrType, err error, cacheDir string) {
	t.Helper()

	if wantErrType != nil {
		if (errors.Is(wantErrType, cache.NoDataFoundError{})) {
			require.ErrorIs(t, err, wantErrType, "Should return no data found")
			return
		}
		require.Error(t, err, "Should return an error but didn't")
		time.Sleep(10 * time.Millisecond)
		requireNoDirtyFileInDir(t, cacheDir)
		return
	}
	require.NoError(t, err)

	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
	require.Equal(t, want, got, "Did not get expected database entry")
}
