package users_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	testutils "github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/cache"
	cachetestutils "github.com/ubuntu/authd/internal/users/cache/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	userstestutils "github.com/ubuntu/authd/internal/users/testutils"
	"go.etcd.io/bbolt"
)

func TestNewManager(t *testing.T) {
	tests := map[string]struct {
		dbFile          string
		corruptedDbFile bool
		dirtyFlag       bool
		markDirty       bool

		expirationDate  string
		skipCleanOnNew  bool
		cleanupInterval int
		procDir         string

		localGroupsFile string

		wantErr bool
	}{
		"Successfully create a new manager": {},

		// Clean up routine tests
		"Clean up on interval": {expirationDate: "2020-01-01", cleanupInterval: 1, skipCleanOnNew: true},
		"Do not prevent manager creation if cache cleanup fails":     {procDir: "does-not-exist"},
		"Do not stop manager if cleanup routine fails":               {procDir: "does-not-exist", skipCleanOnNew: true, cleanupInterval: 1},
		"Do not touch local groups if no user is cleaned from cache": {expirationDate: "2004-01-01"},
		"Do not prevent manager creation if group cleanup fails":     {expirationDate: "2020-01-01", localGroupsFile: "gpasswdfail_in_deleted_group.group"},

		// Corrupted databases
		"New recreates any missing buckets and delete unknowns":          {dbFile: "database_with_unknown_bucket"},
		"Database flagged as dirty is cleared up":                        {dirtyFlag: true},
		"Corrupted database when opening is cleared up":                  {corruptedDbFile: true},
		"Do not prevent manager creation if clearing local groups fails": {corruptedDbFile: true, localGroupsFile: "gpasswdfail_in_deleted_group.group"},
		"Dynamically mark database as corrupted is cleared up":           {markDirty: true},

		"Error if cacheDir does not exist": {dbFile: "-", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.localGroupsFile == "" {
				tc.localGroupsFile = "users_in_groups.group"
			}
			destCmdsFile := localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "groups", tc.localGroupsFile))

			cacheDir := t.TempDir()
			if tc.dbFile == "" {
				tc.dbFile = "multiple_users_and_groups"
			}
			if tc.dbFile == "-" {
				err := os.RemoveAll(cacheDir)
				require.NoError(t, err, "Setup: could not remove temporary cache directory")
			} else if tc.dbFile != "" {
				cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			}
			if tc.dirtyFlag {
				err := os.WriteFile(filepath.Join(cacheDir, userstestutils.DirtyFlagName), nil, 0600)
				require.NoError(t, err, "Setup: could not create dirty flag file")
			}
			if tc.corruptedDbFile {
				err := os.WriteFile(filepath.Join(cacheDir, cachetestutils.DbName), []byte("Corrupted db"), 0600)
				require.NoError(t, err, "Setup: Can't update the file with invalid db content")
			}

			if tc.expirationDate == "" {
				tc.expirationDate = "2004-01-01"
			}
			expiration, err := time.Parse(time.DateOnly, tc.expirationDate)
			require.NoError(t, err, "Setup: could not calculate expiration date for tests")
			managerOpts := []users.Option{users.WithUserExpirationDate(expiration)}

			if tc.cleanupInterval > 0 {
				managerOpts = append(managerOpts, users.WithCacheCleanupInterval(time.Second*time.Duration(tc.cleanupInterval)))
			}
			if tc.skipCleanOnNew {
				managerOpts = append(managerOpts, users.WithoutCleaningCacheOnNew())
			}
			if tc.procDir != "" {
				managerOpts = append(managerOpts, users.WithProcDir(tc.procDir))
			}

			m, err := users.NewManager(cacheDir, managerOpts...)
			if tc.wantErr {
				require.Error(t, err, "NewManager should return an error, but did not")
				return
			}
			require.NoError(t, err, "NewManager should not return an error, but did")

			if tc.markDirty {
				m.RequestClearDatabase()
			}

			// Sync on the clean up routine
			m.WaitCleanupRoutineDone(t, users.WithCacheCleanupInterval(time.Second*time.Duration(tc.cleanupInterval)))

			got, err := cachetestutils.DumpToYaml(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

			requireNoDirtyFileInDir(t, cacheDir)
			if tc.corruptedDbFile {
				requireClearedDatabase(t, userstestutils.GetManagerCache(m))
			}

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, testutils.GoldenPath(t)+".gpasswd.output")
		})
	}
}

func TestStop(t *testing.T) {
	destCmdsFile := localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "groups", "users_in_groups.group"))

	cacheDir := t.TempDir()
	err := os.WriteFile(filepath.Join(cacheDir, cachetestutils.DbName), []byte("Corrupted db"), 0600)
	require.NoError(t, err, "Setup: Can't update the file with invalid db content")

	m := newManagerForTests(t, cacheDir)
	require.NoError(t, m.Stop(), "Stop should not return an error, but did")

	// Should fail, because the cache is closed
	_, err = userstestutils.GetManagerCache(m).AllUsers()
	require.ErrorIs(t, err, bbolt.ErrDatabaseNotOpen, "AllUsers should return an error, but did not")

	// Ensure that the manager only stopped after the routine was done.
	localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, testutils.GoldenPath(t)+".gpasswd.output")
}

func TestUpdateUser(t *testing.T) {
	userCases := map[string]users.UserInfo{
		"newuser": {
			Name: "newuser",
			UID:  1111,
		},
		"nameless": {
			Name: "",
			UID:  1111,
		},
		"user2": {
			Name: "user2",
			UID:  2222,
		},
	}

	groupsCases := map[string][]users.GroupInfo{
		"cloud-group": {{
			Name: "group1",
			GID:  ptrValue(11111),
		}},
		"local-group": {{
			Name: "localgroup1",
			GID:  nil,
		}},
		"mixed-groups-cloud-first": {{
			Name: "group1",
			GID:  ptrValue(11111),
		}, {
			Name: "localgroup1",
			GID:  nil,
		}},
		"mixed-groups-local-first": {{
			Name: "localgroup1",
			GID:  nil,
		}, {
			Name: "group1",
			GID:  ptrValue(11111),
		}},
		"mixed-groups-gpasswd-fail": {{
			Name: "group1",
			GID:  ptrValue(11111),
		}, {
			Name: "gpasswdfail",
			GID:  nil,
		}},
		"nameless-group": {{
			Name: "",
			GID:  ptrValue(11111),
		}},
		"no-groups": {},
	}

	tests := map[string]struct {
		userCase   string
		groupsCase string

		dbFile          string
		localGroupsFile string

		wantErr  error
		noOutput bool
	}{
		"Successfully update user":                       {groupsCase: "cloud-group"},
		"Successfully update user updating local groups": {groupsCase: "mixed-groups-cloud-first", localGroupsFile: "users_in_groups.group"},

		"Error if user has no username":             {userCase: "nameless", wantErr: shouldError{}, noOutput: true},
		"Error if group has no name":                {groupsCase: "nameless-group", wantErr: shouldError{}, noOutput: true},
		"Error if no groups were provided":          {groupsCase: "no-groups", wantErr: shouldError{}, noOutput: true},
		"Error if only local group was provided":    {groupsCase: "local-group", wantErr: shouldError{}, noOutput: true},
		"Error if local group is the default group": {groupsCase: "mixed-groups-local-first", wantErr: shouldError{}, noOutput: true},

		"Error when updating local groups remove user from db":                              {groupsCase: "mixed-groups-gpasswd-fail", localGroupsFile: "gpasswdfail_in_deleted_group.group", wantErr: shouldError{}},
		"Error when updating local groups remove user from db without touching other users": {dbFile: "multiple_users_and_groups", groupsCase: "mixed-groups-gpasswd-fail", localGroupsFile: "gpasswdfail_in_deleted_group.group", wantErr: shouldError{}},
		"Error when updating local groups remove user from db even if already existed":      {userCase: "user2", dbFile: "multiple_users_and_groups", groupsCase: "mixed-groups-gpasswd-fail", localGroupsFile: "gpasswdfail_in_deleted_group.group", wantErr: shouldError{}},

		"Invalid entry clears the database": {groupsCase: "cloud-group", dbFile: "invalid_entry_in_userToGroups", localGroupsFile: "users_in_groups.group", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.localGroupsFile == "" {
				t.Parallel()
			}

			var destCmdsFile string
			if tc.localGroupsFile != "" {
				destCmdsFile = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "groups", tc.localGroupsFile))
			}

			if tc.userCase == "" {
				tc.userCase = "newuser"
			}

			user := userCases[tc.userCase]
			user.Dir = "/home/" + user.Name
			user.Shell = "/bin/bash"
			user.Gecos = "gecos for " + user.Name
			user.Groups = groupsCases[tc.groupsCase]

			cacheDir := t.TempDir()
			if tc.dbFile != "" {
				cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			}
			m := newManagerForTests(t, cacheDir)

			err := m.UpdateUser(user)
			m.WaitCleanupRoutineDone(t)

			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil && tc.noOutput {
				return
			}

			got, err := cachetestutils.DumpToYaml(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, testutils.GoldenPath(t)+".gpasswd.output")
		})
	}
}

func TestBrokerForUser(t *testing.T) {
	tests := map[string]struct {
		username string
		dbFile   string

		wantBrokerID string
		wantErr      error
	}{
		"Successfully get broker for user":                        {username: "user1", dbFile: "multiple_users_and_groups", wantBrokerID: "broker-id"},
		"Return no broker but in cache if user has no broker yet": {username: "userwithoutbroker", dbFile: "multiple_users_and_groups", wantBrokerID: ""},

		"Error if user does not exist":  {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErr: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			brokerID, err := m.BrokerForUser(tc.username)
			m.WaitCleanupRoutineDone(t)

			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			require.Equal(t, tc.wantBrokerID, brokerID, "BrokerForUser should return the expected brokerID, but did not")
		})
	}
}

func TestUpdateBrokerForUser(t *testing.T) {
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
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			if tc.username == "" {
				tc.username = "user1"
			}
			if tc.dbFile == "" {
				tc.dbFile = "multiple_users_and_groups"
			}

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			err := m.UpdateBrokerForUser(tc.username, "ExampleBrokerID")
			m.WaitCleanupRoutineDone(t)

			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			got, err := cachetestutils.DumpToYaml(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "Did not get expected database content")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestUserByName(t *testing.T) {
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
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.UserByName(tc.username)
			m.WaitCleanupRoutineDone(t)

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
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.UserByID(tc.uid)
			m.WaitCleanupRoutineDone(t)

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
	tests := map[string]struct {
		dbFile string

		wantErr error
	}{
		"Successfully get all users": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.AllUsers()
			m.WaitCleanupRoutineDone(t)

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
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.GroupByName(tc.groupname)
			m.WaitCleanupRoutineDone(t)

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
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.GroupByID(tc.gid)
			m.WaitCleanupRoutineDone(t)

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
	tests := map[string]struct {
		dbFile string

		wantErr error
	}{
		"Successfully get all groups": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_groupByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllGroups()
			m.WaitCleanupRoutineDone(t)

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
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.ShadowByName(tc.username)
			m.WaitCleanupRoutineDone(t)

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
	tests := map[string]struct {
		dbFile string

		wantErr error
	}{
		"Successfully get all users": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByID", wantErr: cache.ErrNeedsClearing},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllShadows()
			m.WaitCleanupRoutineDone(t)

			requireErrorAssertions(t, err, tc.wantErr, cacheDir)
			if tc.wantErr != nil {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
			require.Equal(t, want, got, "AllShadows should return the expected users, but did not")
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
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

	require.NoFileExists(t, filepath.Join(cacheDir, userstestutils.DirtyFlagName), "Dirty flag should have been removed")
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

	got, err := cachetestutils.DumpToYaml(c)
	require.NoError(t, err, "Created database should be valid yaml content")
	require.Equal(t, want, got, "Database should only have empty buckets")
}

func newManagerForTests(t *testing.T, cacheDir string) *users.Manager {
	t.Helper()

	expiration, err := time.Parse(time.DateOnly, "2004-01-01")
	require.NoError(t, err, "Setup: could not calculate expiration date for tests")

	m, err := users.NewManager(cacheDir, users.WithUserExpirationDate(expiration), users.WithoutCleaningCacheOnNew())
	require.NoError(t, err, "NewManager should not return an error, but did")

	return m
}

func ptrValue[T any](v T) *T {
	return &v
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	m.Run()
}
