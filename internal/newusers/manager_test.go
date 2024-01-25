package newusers_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/newusers"
	"github.com/ubuntu/authd/internal/newusers/cache"
	cachetests "github.com/ubuntu/authd/internal/newusers/cache/tests"
	newusertests "github.com/ubuntu/authd/internal/newusers/tests"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
)

func TestNewManager(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile          string
		corruptedDbFile bool
		dirtyFlag       bool
		markDirty       bool

		expirationDate  string
		skipCleanOnNew  bool
		cleanupInterval int
		procDir         string

		wantErr bool
	}{
		"Successfully create a new manager": {},

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

		"Error if cacheDir does not exist": {dbFile: "-", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			if tc.dbFile == "-" {
				err := os.RemoveAll(cacheDir)
				require.NoError(t, err, "Setup: could not remove temporary cache directory")
			} else if tc.dbFile != "" {
				createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
			}
			if tc.dirtyFlag {
				err := os.WriteFile(filepath.Join(cacheDir, newusertests.DirtyFlagName), nil, 0600)
				require.NoError(t, err, "Setup: could not create dirty flag file")
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
			managerOpts := []newusers.Option{newusers.WithUserExpirationDate(expiration)}

			if tc.cleanupInterval > 0 {
				managerOpts = append(managerOpts, newusers.WithCacheCleanupInterval(time.Second*time.Duration(tc.cleanupInterval)))
			}
			if tc.skipCleanOnNew {
				managerOpts = append(managerOpts, newusers.WithoutCleaningCacheOnNew())
			}
			if tc.procDir != "" {
				managerOpts = append(managerOpts, newusers.WithProcDir(tc.procDir))
			}

			m, err := newusers.NewManager(cacheDir, managerOpts...)
			if tc.wantErr {
				require.Error(t, err, "NewManager should return an error, but did not")
				return
			}
			require.NoError(t, err, "NewManager should not return an error, but did")
			t.Cleanup(func() { _ = m.Stop() })

			// Wait for the cleanup routine
			time.Sleep(3 * time.Second)

			got, err := cachetests.DumpToYaml(newusertests.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

			requireNoDirtyFileInDir(t, cacheDir)
			if tc.corruptedDbFile {
				requireClearedDatabase(t, newusertests.GetManagerCache(m))
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	t.Parallel()

	gid := 11111

	groupsCases := map[string][]users.GroupInfo{
		"cloud-group": {{
			Name: "group1",
			GID:  &gid,
		}},
		"local-group": {{
			Name: "localgroup1",
			GID:  nil,
		}},
		"mixed-groups-cloud-first": {{
			Name: "group1",
			GID:  &gid,
		}, {
			Name: "localgroup1",
			GID:  nil,
		}},
		"mixed-groups-local-first": {{
			Name: "localgroup1",
			GID:  nil,
		}, {
			Name: "group1",
			GID:  &gid,
		}},
		"no-groups": {},
	}

	tests := map[string]struct {
		groupsCase string

		dbFile string

		wantErr error
	}{
		"Successfully update user": {groupsCase: "cloud-group"},
		"Local groups get ignored": {groupsCase: "mixed-groups-cloud-first"},

		"Error if no groups were provided":          {groupsCase: "no-groups", wantErr: shouldError{}},
		"Error if only local group was provided":    {groupsCase: "local-group", wantErr: shouldError{}},
		"Error if local group is the default group": {groupsCase: "mixed-groups-local-first", wantErr: shouldError{}},

		"Invalid entry clears the database": {groupsCase: "cloud-group", dbFile: "invalid_entry_in_userToGroups", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			user := users.UserInfo{
				Name:   "user1",
				UID:    1111,
				Gecos:  "gecos for user1",
				Dir:    "/home/user1",
				Shell:  "/bin/bash",
				Groups: groupsCases[tc.groupsCase],
			}

			cacheDir := t.TempDir()
			if tc.dbFile != "" {
				createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
			}

			m := newManagerForTests(t, cacheDir)

			err := m.UpdateUser(user)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			got, err := cachetests.DumpToYaml(newusertests.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "Did not get expected database content")
		})
	}
}

func TestBrokerForUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string
		dbFile   string

		wantErr error
	}{
		"Successfully get broker for user": {username: "user1", dbFile: "multiple_users_and_groups_with_brokers"},

		"Error if user does not exist":  {username: "doesnotexist", dbFile: "multiple_users_and_groups_with_brokers", wantErr: cache.NoDataFoundError{}},
		"Error if user has no broker":   {username: "user4", dbFile: "multiple_users_and_groups_with_brokers", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			brokerID, err := m.BrokerForUser(tc.username)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}
			require.Equal(t, "ExampleBrokerID", brokerID, "BrokerForUser should return the expected brokerID, but did not")
		})
	}
}

func TestUpdateBrokerForUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		dbFile string

		wantErr error
	}{
		"Successfully update broker for user": {},

		"Error if user does not exist":  {username: "doesnotexist", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByName", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.username == "" {
				tc.username = "user1"
			}
			if tc.dbFile == "" {
				tc.dbFile = "multiple_users_and_groups"
			}

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			err := m.UpdateBrokerForUser(tc.username, "ExampleBrokerID")
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			got, err := cachetests.DumpToYaml(newusertests.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "Did not get expected database content")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestUserByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string
		dbFile   string

		wantErr error
	}{
		"Successfully get user by name": {username: "user1", dbFile: "multiple_users_and_groups"},

		"Error if user does not exist":  {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.UserByName(tc.username)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "UserByName should return the expected user, but did not")
		})
	}
}

func TestUserByID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		uid    int
		dbFile string

		wantErr error
	}{
		"Successfully get user by ID": {uid: 1111, dbFile: "multiple_users_and_groups"},

		"Error if user does not exist":  {uid: -1, dbFile: "multiple_users_and_groups", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {uid: 1111, dbFile: "invalid_entry_in_userByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.UserByID(tc.uid)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "UserByID should return the expected user, but did not")
		})
	}
}

func TestAllUsers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr error
	}{
		"Successfully get all users": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllUsers()
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "AllUsers should return the expected users, but did not")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestGroupByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		groupname string
		dbFile    string

		wantErr error
	}{
		"Successfully get group by name": {groupname: "group1", dbFile: "multiple_users_and_groups"},

		"Error if group does not exist": {groupname: "doesnotexist", dbFile: "multiple_users_and_groups", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {groupname: "group1", dbFile: "invalid_entry_in_groupByName", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.GroupByName(tc.groupname)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "GroupByName should return the expected group, but did not")
		})
	}
}

func TestGroupByID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		gid    int
		dbFile string

		wantErr error
	}{
		"Successfully get group by ID": {gid: 11111, dbFile: "multiple_users_and_groups"},

		"Error if group does not exist": {gid: -1, dbFile: "multiple_users_and_groups", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {gid: 11111, dbFile: "invalid_entry_in_groupByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.GroupByID(tc.gid)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "GroupByID should return the expected group, but did not")
		})
	}
}

func TestAllGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr error
	}{
		"Successfully get all groups": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_groupByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllGroups()
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "AllGroups should return the expected groups, but did not")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestShadowByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string
		dbFile   string

		wantErr error
	}{
		"Successfully get shadow by name": {username: "user1", dbFile: "multiple_users_and_groups"},

		"Error if shadow does not exist": {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry":  {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.ShadowByName(tc.username)
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "ShadowByName should return the expected user, but did not")
		})
	}
}

func TestAllShadows(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr error
	}{
		"Successfully get all users": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllShadows()
			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "AllShadows should return the expected users, but did not")
		})
	}
}

func createDBFile(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()
	require.NoError(t, cachetests.DbfromYAML(f, destDir), "Setup: should be able to write database file")
}

type shouldError struct{}

func (shouldError) Error() string { return "" }

func requireErrorAssertions(t *testing.T, gotErr, wantErr error, cacheDir string) {
	t.Helper()

	if wantErr != nil {
		if errors.Is(wantErr, cache.NoDataFoundError{}) {
			require.ErrorIs(t, gotErr, cache.NoDataFoundError{}, "Error should be of the expected type")
			return
		}
		if errors.Is(wantErr, cache.ErrNeedsClearing) {
			require.ErrorIs(t, gotErr, cache.ErrNeedsClearing, "Error should be of the expected type")
			// Give some time for the cleanup routine to run.
			time.Sleep(100 * time.Millisecond)
			requireNoDirtyFileInDir(t, cacheDir)
			return
		}
		require.Error(t, gotErr, "Error should be returned")
		return
	}
	require.NoError(t, gotErr, "Error should not be returned")
}

func requireNoDirtyFileInDir(t *testing.T, cacheDir string) {
	t.Helper()

	require.NoFileExists(t, filepath.Join(cacheDir, newusertests.DirtyFlagName), "Dirty flag should have been removed")
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

func newManagerForTests(t *testing.T, cacheDir string) *newusers.Manager {
	t.Helper()

	expiration, err := time.Parse(time.DateOnly, "2004-01-01")
	require.NoError(t, err, "Setup: could not calculate expiration date for tests")

	m, err := newusers.NewManager(cacheDir, newusers.WithUserExpirationDate(expiration), newusers.WithoutCleaningCacheOnNew())
	require.NoError(t, err, "NewManager should not return an error, but did")
	t.Cleanup(func() { _ = m.Stop() })

	return m
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()
	flag.Parse()

	m.Run()
}
