package users_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/cache"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
	userstestutils "github.com/ubuntu/authd/internal/users/testutils"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"go.etcd.io/bbolt"
)

func TestNewManager(t *testing.T) {
	tests := map[string]struct {
		dbFile          string
		corruptedDbFile bool
		uidMin          uint32
		uidMax          uint32
		gidMin          uint32
		gidMax          uint32

		wantErr bool
	}{
		"Successfully create manager with default config": {},
		"Successfully create manager with custom config":  {uidMin: 10000, uidMax: 20000, gidMin: 10000, gidMax: 20000},

		// Corrupted databases
		"New recreates any missing buckets and delete unknowns": {dbFile: "database_with_unknown_bucket"},

		"Error when database is corrupted":     {corruptedDbFile: true, wantErr: true},
		"Error if cacheDir does not exist":     {dbFile: "-", wantErr: true},
		"Error if UID_MIN is equal to UID_MAX": {uidMin: 1000, uidMax: 1000, wantErr: true},
		"Error if GID_MIN is equal to GID_MAX": {gidMin: 1000, gidMax: 1000, wantErr: true},
		"Error if UID range is too small":      {uidMin: 1000, uidMax: 2000, wantErr: true},
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
				cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			}
			if tc.corruptedDbFile {
				err := os.WriteFile(filepath.Join(cacheDir, cache.Z_ForTests_DBName()), []byte("Corrupted db"), 0600)
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

			got, err := cache.Z_ForTests_DumpNormalizedYAML(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdate(t, got)

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t)+".gpasswd.output")
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

type userCase struct {
	types.UserInfo
	UID uint32
}

func TestUpdateUser(t *testing.T) {
	userCases := map[string]userCase{
		"user1":                   {UserInfo: types.UserInfo{Name: "user1"}, UID: 1111},
		"nameless":                {UID: 1111},
		"user2":                   {UserInfo: types.UserInfo{Name: "user2"}, UID: 2222},
		"same-name-different-uid": {UserInfo: types.UserInfo{Name: "user1"}, UID: 3333},
		"different-name-same-uid": {UserInfo: types.UserInfo{Name: "newuser1"}, UID: 1111},
	}

	groupsCases := map[string][]types.GroupInfo{
		"cloud-group": {{Name: "group1", GID: ptrUint32(11111), UGID: "1"}},
		"local-group": {{Name: "localgroup1", GID: nil, UGID: ""}},
		"mixed-groups-cloud-first": {
			{Name: "group1", GID: ptrUint32(11111), UGID: "1"},
			{Name: "localgroup1", GID: nil, UGID: ""},
		},
		"mixed-groups-local-first": {
			{Name: "localgroup1", GID: nil, UGID: ""},
			{Name: "group1", GID: ptrUint32(11111), UGID: "1"},
		},
		"mixed-groups-gpasswd-fail": {
			{Name: "group1", GID: ptrUint32(11111), UGID: "1"},
			{Name: "gpasswdfail", GID: nil, UGID: ""},
		},
		"nameless-group":          {{Name: "", GID: ptrUint32(11111), UGID: "1"}},
		"different-name-same-gid": {{Name: "newgroup1", GID: ptrUint32(11111), UGID: "1"}},
		"no-groups":               {},
	}

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
				cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			}

			gids := []uint32{user.UID}
			for _, group := range user.Groups {
				if group.GID != nil {
					gids = append(gids, *group.GID)
				}
			}
			managerOpts := []users.Option{
				users.WithIDGenerator(&idgenerator.IDGeneratorMock{
					UIDsToGenerate: []uint32{user.UID},
					GIDsToGenerate: gids,
				}),
			}
			m := newManagerForTests(t, cacheDir, managerOpts...)

			var oldUID uint32
			if tc.wantSameUID {
				oldUser, err := m.UserByName(user.Name)
				require.NoError(t, err, "UserByName should not return an error, but did")
				oldUID = oldUser.UID
			}

			err := m.UpdateUser(user.UserInfo)

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr && tc.noOutput {
				return
			}

			if tc.wantSameUID {
				newUser, err := m.UserByName(user.Name)
				require.NoError(t, err, "UserByName should not return an error, but did")
				require.Equal(t, oldUID, newUser.UID, "UID should not have changed")
			}

			got, err := cache.Z_ForTests_DumpNormalizedYAML(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdateYAML(t, got)

			localgroupstestutils.RequireGPasswdOutput(t, destCmdsFile, golden.Path(t)+".gpasswd.output")
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
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
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
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			err := m.UpdateBrokerForUser(tc.username, "ExampleBrokerID")

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			got, err := cache.Z_ForTests_DumpNormalizedYAML(userstestutils.GetManagerCache(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdateYAML(t, got)
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestUserByIDAndName(t *testing.T) {
	tests := map[string]struct {
		uid        uint32
		username   string
		dbFile     string
		isTempUser bool

		wantErr     bool
		wantErrType error
	}{
		"Successfully get user by ID":             {uid: 1111, dbFile: "multiple_users_and_groups"},
		"Successfully get user by name":           {username: "user1", dbFile: "multiple_users_and_groups"},
		"Successfully get temporary user by ID":   {dbFile: "multiple_users_and_groups", isTempUser: true},
		"Successfully get temporary user by name": {username: "tempuser1", dbFile: "multiple_users_and_groups", isTempUser: true},

		"Error if user does not exist - by ID":    {uid: 0, dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if user does not exist - by name":  {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry - by ID":   {uid: 1111, dbFile: "invalid_entry_in_userByID", wantErr: true},
		"Error if db has invalid entry - by name": {username: "user1", dbFile: "invalid_entry_in_userByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			var err error
			if tc.isTempUser {
				tc.uid, _, err = m.TemporaryRecords().RegisterUser("tempuser1")
				require.NoError(t, err, "RegisterUser should not return an error, but did")
			}

			var user types.UserEntry
			if tc.username != "" {
				user, err = m.UserByName(tc.username)
			} else {
				user, err = m.UserByID(tc.uid)
			}

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			// Registering a temporary user creates it with a random UID and random gecos, so we have to make it
			// deterministic before comparing it with the golden file
			if tc.isTempUser {
				require.Equal(t, tc.uid, user.UID)
				user.UID = 0
				require.NotEmpty(t, user.Gecos)
				user.Gecos = ""
			}

			golden.CheckOrUpdateYAML(t, user)
		})
	}
}

func TestAllUsers(t *testing.T) {
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
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			got, err := m.AllUsers()

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			golden.CheckOrUpdateYAML(t, got)
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestGroupByIDAndName(t *testing.T) {
	tests := map[string]struct {
		gid         uint32
		groupname   string
		dbFile      string
		isTempGroup bool

		wantErr     bool
		wantErrType error
	}{
		"Successfully get group by ID":             {gid: 11111, dbFile: "multiple_users_and_groups"},
		"Successfully get group by name":           {groupname: "group1", dbFile: "multiple_users_and_groups"},
		"Successfully get temporary group by ID":   {dbFile: "multiple_users_and_groups", isTempGroup: true},
		"Successfully get temporary group by name": {groupname: "tempgroup1", dbFile: "multiple_users_and_groups", isTempGroup: true},

		"Error if group does not exist - by ID":   {gid: 0, dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if group does not exist - by name": {groupname: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: cache.NoDataFoundError{}},
		"Error if db has invalid entry - by ID":   {gid: 11111, dbFile: "invalid_entry_in_groupByID", wantErr: true},
		"Error if db has invalid entry - by name": {groupname: "group1", dbFile: "invalid_entry_in_groupByName", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about the output of gpasswd in this test, but we still need to mock it.
			_ = localgroupstestutils.SetupGPasswdMock(t, "empty.group")

			cacheDir := t.TempDir()
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)
			m := newManagerForTests(t, cacheDir)

			var err error
			if tc.isTempGroup {
				tc.gid, _, err = m.TemporaryRecords().RegisterGroup("tempgroup1")
				require.NoError(t, err, "RegisterGroup should not return an error, but did")
			}

			var group types.GroupEntry
			if tc.groupname != "" {
				group, err = m.GroupByName(tc.groupname)
			} else {
				group, err = m.GroupByID(tc.gid)
			}

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			// Registering a temporary group creates it with a random GID and random passwd, so we have to make it
			// deterministic before comparing it with the golden file
			if tc.isTempGroup {
				require.Equal(t, tc.gid, group.GID)
				group.GID = 0
				require.NotEmpty(t, group.Passwd)
				group.Passwd = ""
			}

			golden.CheckOrUpdateYAML(t, group)
		})
	}
}

func TestAllGroups(t *testing.T) {
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
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllGroups()

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			golden.CheckOrUpdateYAML(t, got)
		})
	}
}

func TestShadowByName(t *testing.T) {
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
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.ShadowByName(tc.username)

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			golden.CheckOrUpdateYAML(t, got)
		})
	}
}

func TestAllShadows(t *testing.T) {
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
			cache.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), cacheDir)

			m := newManagerForTests(t, cacheDir)

			got, err := m.AllShadows()

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr {
				return
			}

			golden.CheckOrUpdateYAML(t, got)
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

func newManagerForTests(t *testing.T, cacheDir string, opts ...users.Option) *users.Manager {
	t.Helper()

	m, err := users.NewManager(users.DefaultConfig, cacheDir, opts...)
	require.NoError(t, err, "NewManager should not return an error, but did")

	return m
}

func ptrUint32(v uint32) *uint32 {
	return &v
}

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)
	m.Run()
}
