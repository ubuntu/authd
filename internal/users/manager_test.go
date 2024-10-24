package users_test

import (
	"os"
	"path/filepath"
	"testing"

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
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		dbFile          string
		corruptedDbFile bool
		uidMin          uint32
		uidMax          uint32
		gidMin          uint32
		gidMax          uint32

		wantErr bool
	}{
		"Successfully create a new manager": {},

		// Corrupted databases
		"New recreates any missing buckets and delete unknowns": {dbFile: "database_with_unknown_bucket"},

		"Error when database is corrupted":     {corruptedDbFile: true, wantErr: true},
		"Error if cacheDir does not exist":     {dbFile: "-", wantErr: true},
		"Error if UID_MIN is equal to UID_MAX": {uidMin: 1000, uidMax: 1000, wantErr: true},
		"Error if GID_MIN is equal to GID_MAX": {gidMin: 1000, gidMax: 1000, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			destCmdsFile := localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "groups", "users_in_groups.group"))

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
			if tc.corruptedDbFile {
				err := os.WriteFile(filepath.Join(cacheDir, cachetestutils.DbName), []byte("Corrupted db"), 0600)
				require.NoError(t, err, "Setup: Can't update the file with invalid db content")
			}

			config := users.DefaultConfig
			if tc.uidMin != 0 {
				config.UIDMin = tc.uidMin
			}
			if tc.uidMax != 0 {
				config.UIDMax = tc.uidMax
			}
			if tc.gidMin != 0 {
				config.GIDMin = tc.gidMin
			}
			if tc.gidMax != 0 {
				config.GIDMax = tc.gidMax
			}

			m, err := users.NewManager(config, cacheDir)
			if tc.wantErr {
				require.Error(t, err, "NewManager should return an error, but did not")
				return
			}
			require.NoError(t, err, "NewManager should not return an error, but did")

			got, err := cachetestutils.DumpToYaml(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "Did not get expected database content")

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, testutils.GoldenPath(t)+".gpasswd.output")
		})
	}
}

func TestStop(t *testing.T) {
	cacheDir := t.TempDir()
	m := newManagerForTests(t, cacheDir)
	require.NoError(t, m.Stop(), "Stop should not return an error, but did")

	// Should fail, because the cache is closed
	_, err := userstestutils.GetManagerCache(m).AllUsers()
	require.ErrorIs(t, err, bbolt.ErrDatabaseNotOpen, "AllUsers should return an error, but did not")
}

func TestUpdateUser(t *testing.T) {
	userCases := map[string]users.UserInfo{
		"user1": {
			Name: "user1",
			UID:  1111,
		},
		"user2": {
			Name: "user2",
			UID:  2222,
		},
		"same-name-different-uid": {
			Name: "user1",
			UID:  3333,
		},
		"different-name-same-uid": {
			Name: "newuser1",
			UID:  1111,
		},
	}

	groupsCases := map[string][]users.GroupInfo{
		"cloud-group": {{
			Name: "group1",
			GID:  ptrUint32(11111),
		}},
		"local-group": {{
			Name: "localgroup1",
			GID:  nil,
		}},
		"mixed-groups-cloud-first": {{
			Name: "group1",
			GID:  ptrUint32(11111),
		}, {
			Name: "localgroup1",
			GID:  nil,
		}},
		"mixed-groups-local-first": {{
			Name: "localgroup1",
			GID:  nil,
		}, {
			Name: "group1",
			GID:  ptrUint32(11111),
		}},
		"mixed-groups-gpasswd-fail": {{
			Name: "group1",
			GID:  ptrUint32(11111),
		}, {
			Name: "gpasswdfail",
			GID:  nil,
		}},
		"nameless-group": {{
			Name: "",
			GID:  ptrUint32(11111),
		}},
		"different-name-same-gid": {{
			Name: "newgroup1",
			GID:  ptrUint32(11111),
		}},
		"no-groups": {},
	}

	goldenTracker := testutils.NewGoldenTracker(t)

	tests := map[string]struct {
		userCase   string
		groupsCase string

		dbFile          string
		localGroupsFile string

		wantErr     bool
		noOutput    bool
		wantSameUID bool
	}{
		"Successfully update user":                       {groupsCase: "cloud-group"},
		"Successfully update user updating local groups": {groupsCase: "mixed-groups-cloud-first", localGroupsFile: "users_in_groups.group"},
		"UID does not change if user already exists":     {userCase: "same-name-different-uid", dbFile: "one_user_and_group", wantSameUID: true},

		"Error if user has no username":      {userCase: "nameless", wantErr: true, noOutput: true},
		"Error if user has conflicting uid":  {userCase: "different-name-same-uid", dbFile: "one_user_and_group", wantErr: true, noOutput: true},
		"Error if group has no name":         {groupsCase: "nameless-group", wantErr: true, noOutput: true},
		"Error if group has conflicting gid": {groupsCase: "different-name-same-gid", dbFile: "one_user_and_group", wantErr: true, noOutput: true},

		"Error when updating local groups remove user from db":                              {groupsCase: "mixed-groups-gpasswd-fail", localGroupsFile: "gpasswdfail_in_deleted_group.group", wantErr: true},
		"Error when updating local groups remove user from db without touching other users": {dbFile: "multiple_users_and_groups", groupsCase: "mixed-groups-gpasswd-fail", localGroupsFile: "gpasswdfail_in_deleted_group.group", wantErr: true},
		"Error when updating local groups remove user from db even if already existed":      {userCase: "user2", dbFile: "multiple_users_and_groups", groupsCase: "mixed-groups-gpasswd-fail", localGroupsFile: "gpasswdfail_in_deleted_group.group", wantErr: true},

		"Error on invalid entry": {groupsCase: "cloud-group", dbFile: "invalid_entry_in_userToGroups", localGroupsFile: "users_in_groups.group", wantErr: true, noOutput: true},
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
				tc.userCase = "user1"
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

			var oldUID uint32
			if tc.wantSameUID {
				oldUser, err := m.UserByName(user.Name)
				require.NoError(t, err, "UserByName should not return an error, but did")
				oldUID = oldUser.UID
			}

			err := m.UpdateUser(user)

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr && tc.noOutput {
				return
			}

			if tc.wantSameUID {
				newUser, err := m.UserByName(user.Name)
				require.NoError(t, err, "UserByName should not return an error, but did")
				require.Equal(t, oldUID, newUser.UID, "UID should not have changed")
			}

			got, err := cachetestutils.DumpToYaml(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "Did not get expected database content")

			gpasswdGolden := testutils.GoldenPath(t) + ".gpasswd.output"
			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, gpasswdGolden)
			goldenTracker.MarkUsed(t, testutils.WithGoldenPath(gpasswdGolden))
		})
	}
}

func TestBrokerForUser(t *testing.T) {
	tests := map[string]struct {
		username string
		dbFile   string

		wantBrokerID string
		wantErr      bool
		wantErrType  error
	}{
		"Successfully get broker for user":                        {username: "user1", dbFile: "multiple_users_and_groups", wantBrokerID: "broker-id"},
		"Return no broker but in cache if user has no broker yet": {username: "userwithoutbroker", dbFile: "multiple_users_and_groups", wantBrokerID: ""},

		"Error if user does not exist":  {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			brokerID, err := m.BrokerForUser(tc.username)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			require.Equal(t, tc.wantBrokerID, brokerID, "BrokerForUser should return the expected brokerID, but did not")
		})
	}
}

func TestUpdateBrokerForUser(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		username string

		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully update broker for user": {},

		"Error if user does not exist":  {username: "doesnotexist", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByName", wantErr: true},
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

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			got, err := cachetestutils.DumpToYaml(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "Did not get expected database content")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestUserByName(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		username string
		dbFile   string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get user by name": {username: "user1", dbFile: "multiple_users_and_groups"},

		"Error if user does not exist":  {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.UserByName(tc.username)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "UserByName should return the expected user, but did not")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestUserByID(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		uid    uint32
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get user by ID": {uid: 1111, dbFile: "multiple_users_and_groups"},

		"Error if user does not exist":  {uid: 0, dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {uid: 1111, dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.UserByID(tc.uid)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "UserByID should return the expected user, but did not")
		})
	}
}

func TestAllUsers(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get all users": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.AllUsers()

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "AllUsers should return the expected users, but did not")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestGroupByName(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		groupname string
		dbFile    string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get group by name": {groupname: "group1", dbFile: "multiple_users_and_groups"},

		"Error if group does not exist": {groupname: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {groupname: "group1", dbFile: "invalid_entry_in_groupByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.GroupByName(tc.groupname)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "GroupByName should return the expected group, but did not")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestGroupByID(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		gid    uint32
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get group by ID": {gid: 11111, dbFile: "multiple_users_and_groups"},

		"Error if group does not exist": {gid: 0, dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry": {gid: 11111, dbFile: "invalid_entry_in_groupByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.GroupByID(tc.gid)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "GroupByID should return the expected group, but did not")
		})
	}
}

func TestAllGroups(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get all groups": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_groupByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllGroups()

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "AllGroups should return the expected groups, but did not")
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestShadowByName(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		username string
		dbFile   string

		wantErr     bool
		wantErrType error
	}{
		"Successfully get shadow by name": {username: "user1", dbFile: "multiple_users_and_groups"},

		"Error if shadow does not exist": {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry":  {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.ShadowByName(tc.username)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "ShadowByName should return the expected user, but did not")
		})
	}
}

func TestAllShadows(t *testing.T) {
	goldenTracker := testutils.NewGoldenTracker(t)
	tests := map[string]struct {
		dbFile string

		wantErr bool
	}{
		"Successfully get all users": {dbFile: "multiple_users_and_groups"},

		"Error if db has invalid entry": {dbFile: "invalid_entry_in_userByID", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllShadows()

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr {
				return
			}

			want := testutils.LoadWithUpdateFromGoldenYAML(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "AllShadows should return the expected users, but did not")
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
}

func requireErrorAssertions(t *testing.T, gotErr, wantErrType error, wantErr bool) {
	t.Helper()

	if wantErrType != nil {
		require.ErrorIs(t, gotErr, wantErrType, "Should return expected error")
		return
	}
	if wantErr {
		require.Error(t, gotErr, "Error should be returned")
		return
	}
	require.NoError(t, gotErr, "Error should not be returned")
}

func newManagerForTests(t *testing.T, cacheDir string) *users.Manager {
	t.Helper()

	m, err := users.NewManager(users.DefaultConfig, cacheDir)
	require.NoError(t, err, "NewManager should not return an error, but did")

	return m
}

func ptrUint32(v uint32) *uint32 {
	return &v
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
