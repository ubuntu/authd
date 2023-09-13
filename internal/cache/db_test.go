package cache_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
	_ "unsafe"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/cache"
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

		wantErr bool
	}{
		"New without any initialized database": {},
		"New with already existing database":   {dbFile: "multiple_users_and_groups"},

		// Corrupted databases
		"Database flagged as dirty is cleared up":              {dbFile: "multiple_users_and_groups", dirtyFlag: true},
		"Corrupted database when opening is cleared up":        {corruptedDbFile: true},
		"Dynamically mark database as corrupted is cleared up": {markDirty: true},

		"Error on cacheDir non existent cacheDir":      {dbFile: "-", wantErr: true},
		"Error on invalid permission on database file": {dbFile: "multiple_users_and_groups", perm: &perm0644, wantErr: true},
		"Error on unreadable database file":            {dbFile: "multiple_users_and_groups", perm: &perm0000, wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			dbDestPath := filepath.Join(cacheDir, cache.DbName)

			if tc.dbFile == "-" {
				err := os.RemoveAll(cacheDir)
				require.NoError(t, err, "Setup: could not remove temporary cache directory")
			} else if tc.dbFile != "" {
				createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
			}
			if tc.dirtyFlag {
				err := os.WriteFile(filepath.Join(cacheDir, cache.DirtyFlagDbName), nil, 0600)
				require.NoError(t, err, "Setup: could not create dirty flag file")
			}
			if tc.perm != nil {
				err := os.Chmod(dbDestPath, *tc.perm)
				require.NoError(t, err, "Setup: could not change mode of database file")
			}

			if tc.corruptedDbFile {
				err := os.WriteFile(filepath.Join(cacheDir, cache.DbName), []byte("Corrupted db"), 0600)
				require.NoError(t, err, "Setup: Can't update the file with invalid db content")
			}

			c, err := cache.New(cacheDir)
			if tc.wantErr {
				require.Error(t, err, "New should return an error but didn't")
				return
			}
			require.NoError(t, err)
			defer c.Close()

			if tc.markDirty {
				// Mark the database to be cleared. This is not part of the API and only for tests.
				cache.RequestClearDatabase(c)
				// Let the cache cleanup start proceeding
				time.Sleep(time.Millisecond)
			}

			got, err := dumpToYaml(c)
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

	userCases := map[string]cache.UserInfo{
		"user1": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []cache.GroupInfo{
				{Name: "group1", GID: 11111},
			},
		},
		"user1-new-attributes": {
			Name:  "newuser1",
			UID:   1111,
			Gecos: "New user1 gecos",
			Dir:   "/home/newuser1",
			Shell: "/bin/dash",
			Groups: []cache.GroupInfo{
				{Name: "group1", GID: 11111},
			},
		},
		"group1-new-attributes": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []cache.GroupInfo{
				{Name: "newgroup1", GID: 11111},
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
			Groups: []cache.GroupInfo{
				{Name: "group1", GID: 11111},
				{Name: "group2", GID: 22222},
			},
		},
		"user1-with-new-default-group": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []cache.GroupInfo{
				{Name: "group2", GID: 22222},
				{Name: "group1", GID: 11111},
			},
		},
		"user1-with-only-new-group": {
			Name:  "user1",
			UID:   1111,
			Gecos: "User1 gecos\nOn multiple lines",
			Dir:   "/home/user1",
			Shell: "/bin/bash",
			Groups: []cache.GroupInfo{
				{Name: "group2", GID: 22222},
			},
		},
		"user3-without-common-group": {
			Name:  "user3",
			UID:   3333,
			Gecos: "User3 gecos",
			Dir:   "/home/user3",
			Shell: "/bin/zsh",
			Groups: []cache.GroupInfo{
				{Name: "group3", GID: 33333},
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

			cacheDir := t.TempDir()
			if tc.dbFile != "" {
				createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
			}

			c, err := cache.New(cacheDir)
			require.NoError(t, err)
			defer c.Close()

			userInfo := userCases[tc.userCase]
			err = c.UpdateFromUserInfo(userInfo)
			if tc.wantErr {
				require.Error(t, err, "UpdateFromUserInfo should return an error but didn't")

				if tc.wantClearDB {
					time.Sleep(10 * time.Millisecond)
					requireNoDirtyFileInDir(t, cacheDir)
					requireClearedDatabase(t, c)
				}
				return
			} else {
				require.NoError(t, err)
			}

			requireNoDirtyFileInDir(t, cacheDir)

			got, err := dumpToYaml(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

		})
	}
}

func createDBFile(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = dbfromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}

func requireNoDirtyFileInDir(t *testing.T, cacheDir string) {
	t.Helper()

	_, err := os.Stat(filepath.Join(cacheDir, cache.DirtyFlagDbName))
	require.ErrorIs(t, err, fs.ErrNotExist, "Dirty flag should have been removed")
}

func requireClearedDatabase(t *testing.T, c *cache.Cache) {
	t.Helper()

	want := `GroupByID: {}
GroupByName: {}
GroupToUsers: {}
UserByID: {}
UserByName: {}
UserToGroups: {}
`

	got, err := dumpToYaml(c)
	require.NoError(t, err, "Created database should be valid yaml content")
	require.Equal(t, want, string(got), "Database should only have empty buckets")
}

//go:linkname dumpToYaml github.com/ubuntu/authd/internal/cache.(*Cache).dumpToYaml
func dumpToYaml(c *cache.Cache) (string, error)

//go:linkname dbfromYAML github.com/ubuntu/authd/internal/cache.dbfromYAML
func dbfromYAML(r io.Reader, destDir string) error

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()

	os.Exit(m.Run())
}
