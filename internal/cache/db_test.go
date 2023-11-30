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
	"github.com/ubuntu/authd/internal/cache"
	cachetests "github.com/ubuntu/authd/internal/cache/tests"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
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

		// Clean up tests
		"Clean up all users":  {dbFile: "only_old_users", expirationDate: "2020-01-01"},
		"Clean up some users": {dbFile: "multiple_users_and_groups", expirationDate: "2020-01-01"},
		"Clean up also cleans last selected broker for user":     {dbFile: "multiple_users_and_groups_with_brokers", expirationDate: "2020-01-01"},
		"Clean up on interval":                                   {dbFile: "multiple_users_and_groups", expirationDate: "2020-01-01", cleanupInterval: 1, skipCleanOnNew: true},
		"Clean up as much as possible if db has invalid entries": {dbFile: "invalid_entries_but_user_and_group1", expirationDate: "2020-01-01"},
		"Clean up user even if it is not listed on the group":    {dbFile: "user_not_in_groupToUsers", expirationDate: "2020-01-01"},
		"Do not clean any user":                                  {dbFile: "multiple_users_and_groups"},
		"Do not clean active user":                               {dbFile: "active_user", expirationDate: "2020-01-01"},
		"Do not prevent cache creation if cleanup fails":         {dbFile: "multiple_users_and_groups", procDir: "does-not-exist"},
		"Do not stop cache if cleanup routine fails":             {dbFile: "multiple_users_and_groups", procDir: "does-not-exist", skipCleanOnNew: true, cleanupInterval: 1},
		"Do not clean user if can not get groups":                {dbFile: "invalid_entry_in_userToGroups", expirationDate: "2020-01-01"},
		"Do not clean user if can not delete user from group":    {dbFile: "invalid_entry_in_groupByID", expirationDate: "2020-01-01"},

		// Corrupted databases
		"New recreates any missing buckets and delete unknowns": {dbFile: "database_with_unknown_bucket"},
		"Database flagged as dirty is cleared up":               {dbFile: "multiple_users_and_groups", dirtyFlag: true},
		"Corrupted database when opening is cleared up":         {corruptedDbFile: true},
		"Dynamically mark database as corrupted is cleared up":  {markDirty: true},

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
			if tc.dirtyFlag {
				err := os.WriteFile(filepath.Join(cacheDir, cachetests.DirtyFlagDbName), nil, 0600)
				require.NoError(t, err, "Setup: could not create dirty flag file")
			}
			if tc.perm != nil {
				err := os.Chmod(dbDestPath, *tc.perm)
				require.NoError(t, err, "Setup: could not change mode of database file")
			}
			if tc.corruptedDbFile {
				err := os.WriteFile(filepath.Join(cacheDir, cachetests.DbName), []byte("Corrupted db"), 0600)
				require.NoError(t, err, "Setup: Can't update the file with invalid db content")
			}

			if tc.expirationDate == "" {
				tc.expirationDate = "2004-01-01"
			}
			expiration, err := time.Parse(time.DateOnly, tc.expirationDate)
			require.NoError(t, err, "Setup: could not calculate expiration date for tests")
			cacheOpts := []cache.Option{cache.WithExpirationDate(expiration)}

			if tc.cleanupInterval > 0 {
				cacheOpts = append(cacheOpts, cache.WithCleanupInterval(time.Second*time.Duration(tc.cleanupInterval)))
			}
			if tc.skipCleanOnNew {
				cacheOpts = append(cacheOpts, cache.WithoutCleaningOnNew())
			}
			if tc.procDir != "" {
				cacheOpts = append(cacheOpts, cache.WithProcDir(tc.procDir))
			}

			c, err := cache.New(cacheDir, cacheOpts...)
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

			if tc.markDirty {
				// Mark the database to be cleared. This is not part of the API and only for tests.
				cache.RequestClearDatabase(c)
				// Let the cache cleanup start proceeding
				time.Sleep(time.Millisecond)
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

func TestUpdateFromUserInfo(t *testing.T) {
	t.Parallel()

	userCases := map[string]users.UserInfo{
		"user1": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []users.GroupInfo{
				{Name: "group1", GID: ptrValue(11111)},
			},
		},
		"user1-new-attributes": {
			Name:  "newuser1",
			UID:   1111,
			Gecos: "New user1 gecos",
			Dir:   "/home/newuser1",
			Shell: "/bin/dash",
			Groups: []users.GroupInfo{
				{Name: "group1", GID: ptrValue(11111)},
			},
		},
		"group1-new-attributes": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []users.GroupInfo{
				{Name: "newgroup1", GID: ptrValue(11111)},
			},
		},
		"user1-without-groups": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
		},
		"user1-with-new-group": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []users.GroupInfo{
				{Name: "group1", GID: ptrValue(11111)},
				{Name: "group2", GID: ptrValue(22222)},
			},
		},
		"user1-with-new-default-group": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []users.GroupInfo{
				{Name: "group2", GID: ptrValue(22222)},
				{Name: "group1", GID: ptrValue(11111)},
			},
		},
		"user1-with-only-new-group": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []users.GroupInfo{
				{Name: "group2", GID: ptrValue(22222)},
			},
		},
		"user1-with-local-group": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []users.GroupInfo{
				{Name: "group1", GID: ptrValue(11111)},
				{Name: "local-group"},
			},
		},
		"user3-without-common-group": {
			Name:  "user3",
			UID:   3333,
			Gecos: "User3 gecos",
			Dir:   "/home/user3",
			Shell: "/bin/zsh",
			Groups: []users.GroupInfo{
				{Name: "group3", GID: ptrValue(33333)},
			},
		},
	}

	tests := map[string]struct {
		userCase string
		dbFile   string

		wantClearDB bool
		wantErr     bool
	}{
		// New user
		"Insert new user":                 {userCase: "user1"},
		"Update last login time for user": {userCase: "user1", dbFile: "one_user_and_group"},

		// User and Group renames
		"Update user by changing attributes":  {userCase: "user1-new-attributes", dbFile: "one_user_and_group"},
		"Update group by changing attributes": {userCase: "group1-new-attributes", dbFile: "one_user_and_group"},

		// Group updates
		"Update user and keep existing groups without specifying them": {userCase: "user1-without-groups", dbFile: "one_user_and_group"},
		"Update user by adding a new group":                            {userCase: "user1-with-new-group", dbFile: "one_user_and_group"},
		"Update user by adding a new default group":                    {userCase: "user1-with-new-default-group", dbFile: "one_user_and_group"},
		"Remove group from user":                                       {userCase: "user1-with-only-new-group", dbFile: "one_user_and_group"},

		// Multi users handling
		"Update only user even if we have multiple of them":     {userCase: "user1", dbFile: "multiple_users_and_groups"},
		"Add user to group from another user":                   {userCase: "user1-with-new-group", dbFile: "multiple_users_and_groups"},
		"Remove user from a group still part from another user": {userCase: "user3-without-common-group", dbFile: "multiple_users_and_groups"},

		// Local group with no gid
		"Local groups are filtered": {userCase: "user1-with-local-group"},

		// Allowed inconsistent cases
		"Invalid value entry in groupByID but user restating group recreates entries":       {userCase: "user1", dbFile: "invalid_entry_in_groupByID"},
		"Invalid value entry in userByID recreates entries":                                 {userCase: "user1", dbFile: "invalid_entry_in_userByID"},
		"Invalid value entry in groupByName recreates entries":                              {userCase: "user1", dbFile: "invalid_entry_in_groupByName"},
		"Invalid value entry in groupByName recreates entries even without restating group": {userCase: "user1-without-groups", dbFile: "invalid_entry_in_groupByName"},
		"Invalid value entry in userByName recreates entries":                               {userCase: "user1", dbFile: "invalid_entry_in_userByName"},
		"Invalid value entries in other user and groups don't impact current request":       {userCase: "user1", dbFile: "invalid_entries_but_user_and_group1"},

		// Error cases
		"Error on new user without any groups":                                                     {userCase: "user1-without-groups", wantErr: true},
		"Error on invalid value entry in userToGroups clear database":                              {userCase: "user1", dbFile: "invalid_entry_in_userToGroups", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupByID with user not restating groups clear database":  {userCase: "user1-without-groups", dbFile: "invalid_entry_in_groupByID", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupToUsers clear database":                              {userCase: "user1", dbFile: "invalid_entry_in_groupToUsers", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupToUsers for user dropping from group clear database": {userCase: "user1", dbFile: "invalid_entry_in_groupToUsers_secondary_group", wantErr: true, wantClearDB: true},
		"Error on invalid value entry in groupByID for user dropping from group clear database":    {userCase: "user1", dbFile: "invalid_entry_in_groupByID_secondary_group", wantErr: true, wantClearDB: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			userInfo := userCases[tc.userCase]
			err := c.UpdateFromUserInfo(userInfo)
			if tc.wantErr {
				require.Error(t, err, "UpdateFromUserInfo should return an error but didn't")

				if tc.wantClearDB {
					time.Sleep(10 * time.Millisecond)
					requireNoDirtyFileInDir(t, cacheDir)
					requireClearedDatabase(t, c)
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
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByID", wantErrType: shouldError{}},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.UserByID(1111)
			requireGetAssertions(t, got, tc.wantErrType, err, c, cacheDir)
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
		"Error on invalid database entry": {dbFile: "invalid_entry_in_userByName", wantErrType: shouldError{}},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.UserByName("user1")
			requireGetAssertions(t, got, tc.wantErrType, err, c, cacheDir)
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

		"Error on some invalid users entry": {dbFile: "invalid_entries_but_user_and_group1", wantErrType: shouldError{}},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.AllUsers()
			requireGetAssertions(t, got, tc.wantErrType, err, c, cacheDir)
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
		"Error on invalid database entry": {dbFile: "invalid_entry_in_groupByID", wantErrType: shouldError{}},
		"Error as missing userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErrType: shouldError{}},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.GroupByID(11111)
			requireGetAssertions(t, got, tc.wantErrType, err, c, cacheDir)
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
		"Error on invalid database entry": {dbFile: "invalid_entry_in_groupByName", wantErrType: shouldError{}},
		"Error as missing userByID":       {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErrType: shouldError{}},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.GroupByName("group1")
			requireGetAssertions(t, got, tc.wantErrType, err, c, cacheDir)
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

		"Error on some invalid groups entry":     {dbFile: "invalid_entries_but_user_and_group1", wantErrType: shouldError{}},
		"Error as not only relying on groupByID": {dbFile: "partially_valid_multiple_users_and_groups_only_groupByID", wantErrType: shouldError{}},
		"Error as missing userByID":              {dbFile: "partially_valid_multiple_users_and_groups_groupByID_groupToUsers", wantErrType: shouldError{}},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, cacheDir := initCache(t, tc.dbFile)

			got, err := c.AllGroups()
			requireGetAssertions(t, got, tc.wantErrType, err, c, cacheDir)
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

func requireClearedDatabase(t *testing.T, c *cache.Cache) {
	t.Helper()

	want := `GroupByID: {}
GroupByName: {}
GroupToUsers: {}
UserByID: {}
UserByName: {}
UserToBroker: {}
UserToGroups: {}
`

	got, err := cachetests.DumpToYaml(c)
	require.NoError(t, err, "Created database should be valid yaml content")
	require.Equal(t, want, got, "Database should only have empty buckets")
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()

	os.Exit(m.Run())
}

type shouldError struct{}

func (shouldError) Error() string { return "" }

// initCache returns a new cache ready to be used alongside its cache directory.
func initCache(t *testing.T, dbFile string) (c *cache.Cache, cacheDir string) {
	t.Helper()

	cacheDir = t.TempDir()
	if dbFile != "" {
		createDBFile(t, filepath.Join("testdata", dbFile+".db.yaml"), cacheDir)
	}

	expiration, err := time.Parse(time.DateOnly, "2004-01-01")
	require.NoError(t, err, "Setup: could not parse time for testing")

	c, err = cache.New(cacheDir, cache.WithExpirationDate(expiration))
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	return c, cacheDir
}

func requireGetAssertions[E any](t *testing.T, got E, wantErrType, err error, c *cache.Cache, cacheDir string) {
	t.Helper()

	if wantErrType != nil {
		if (errors.Is(wantErrType, cache.NoDataFoundError{})) {
			require.ErrorIs(t, err, wantErrType, "Should return no data found")
			return
		}
		require.Error(t, err, "Should return an error but didn't")
		time.Sleep(10 * time.Millisecond)
		requireNoDirtyFileInDir(t, cacheDir)
		requireClearedDatabase(t, c)
		return
	}
	require.NoError(t, err)

	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
	require.Equal(t, want, got, "Did not get expected database entry")
}

// ptrValue returns a pointer to the given value to simplify the syntax for const.
func ptrValue[T any](value T) *T {
	return &value
}
