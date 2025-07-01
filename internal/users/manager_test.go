package users_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/internal/users/tempentries"
	userstestutils "github.com/ubuntu/authd/internal/users/testutils"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
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
		"Successfully_create_manager_with_default_config": {},
		"Successfully_create_manager_with_custom_config":  {uidMin: 10000, uidMax: 20000, gidMin: 10000, gidMax: 20000},

		// Corrupted databases
		"Error_when_database_is_corrupted":     {corruptedDbFile: true, wantErr: true},
		"Error_if_dbDir_does_not_exist":        {dbFile: "-", wantErr: true},
		"Error_if_UID_MIN_is_equal_to_UID_MAX": {uidMin: 1000, uidMax: 1000, wantErr: true},
		"Error_if_GID_MIN_is_equal_to_GID_MAX": {gidMin: 1000, gidMax: 1000, wantErr: true},
		"Error_if_UID_range_is_too_small":      {uidMin: 1000, uidMax: 2000, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			destGroupFile := localgroupstestutils.SetupGroupMock(t,
				filepath.Join("testdata", "groups", "users_in_groups.group"))

			dbDir := t.TempDir()
			if tc.dbFile == "" {
				tc.dbFile = "multiple_users_and_groups"
			}
			if tc.dbFile == "-" {
				err := os.RemoveAll(dbDir)
				require.NoError(t, err, "Setup: could not remove temporary db directory")
			} else if tc.dbFile != "" {
				err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
				require.NoError(t, err, "Setup: could not create database from testdata")
			}
			if tc.corruptedDbFile {
				err := os.WriteFile(filepath.Join(dbDir, consts.DefaultDatabaseFileName), []byte("Corrupted db"), 0600)
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

			m, err := users.NewManager(config, dbDir)
			if tc.wantErr {
				require.Error(t, err, "NewManager should return an error, but did not")
				return
			}
			require.NoError(t, err, "NewManager should not return an error, but did")

			got, err := db.Z_ForTests_DumpNormalizedYAML(userstestutils.GetManagerDB(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdate(t, got)

			localgroupstestutils.RequireGroupFile(t, destGroupFile, golden.Path(t))
		})
	}
}

func TestStop(t *testing.T) {
	dbDir := t.TempDir()
	m := newManagerForTests(t, dbDir)
	require.NoError(t, m.Stop(), "Stop should not return an error, but did")

	// Should fail, because the db is closed
	_, err := userstestutils.GetManagerDB(m).AllUsers()

	require.Error(t, err, "AllUsers should return an error, but did not")
}

type userCase struct {
	types.UserInfo
	UID uint32 // The UID to generate for this user
}

type groupCase struct {
	types.GroupInfo
	GID uint32 // The GID to generate for this group
}

func TestUpdateUser(t *testing.T) {
	userCases := map[string]userCase{
		"user1":                             {UserInfo: types.UserInfo{Name: "user1"}, UID: 1111},
		"nameless":                          {UID: 1111},
		"user2":                             {UserInfo: types.UserInfo{Name: "user2"}, UID: 2222},
		"same-name-different-uid":           {UserInfo: types.UserInfo{Name: "user1"}, UID: 3333},
		"different-name-same-uid":           {UserInfo: types.UserInfo{Name: "newuser1"}, UID: 1111},
		"different-capitalization-same-uid": {UserInfo: types.UserInfo{Name: "User1"}, UID: 1111},
		"user-exists-on-system":             {UserInfo: types.UserInfo{Name: "root"}, UID: 1111},
	}

	groupsCases := map[string][]groupCase{
		"authd-group":                {{GroupInfo: types.GroupInfo{Name: "group1", UGID: "1"}, GID: 11111}},
		"local-group":                {{GroupInfo: types.GroupInfo{Name: "localgroup1", UGID: ""}}},
		"authd-group-with-uppercase": {{GroupInfo: types.GroupInfo{Name: "Group1", UGID: "1"}, GID: 11111}},
		"mixed-groups-authd-first": {
			{GroupInfo: types.GroupInfo{Name: "group1", UGID: "1"}, GID: 11111},
			{GroupInfo: types.GroupInfo{Name: "localgroup1", UGID: ""}},
		},
		"mixed-groups-local-first": {
			{GroupInfo: types.GroupInfo{Name: "localgroup1", UGID: ""}},
			{GroupInfo: types.GroupInfo{Name: "group1", UGID: "1"}, GID: 11111},
		},
		"nameless-group":          {{GroupInfo: types.GroupInfo{Name: "", UGID: "1"}, GID: 11111}},
		"different-name-same-gid": {{GroupInfo: types.GroupInfo{Name: "newgroup1", UGID: "1"}, GID: 11111}},
		"group-exists-on-system":  {{GroupInfo: types.GroupInfo{Name: "root", UGID: "1"}, GID: 11111}},
		"no-groups":               {},
		// This group case has no GID to generate, because it's expected that the GID of the old group is re-used
		"different-name-same-ugid": {{GroupInfo: types.GroupInfo{Name: "renamed-group", UGID: "12345678"}}},
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
		"Successfully_update_user":                                          {groupsCase: "authd-group"},
		"Successfully_update_user_updating_local_groups":                    {groupsCase: "mixed-groups-authd-first", localGroupsFile: "users_in_groups.group"},
		"Successfully_update_user_updating_local_groups_with_changes":       {groupsCase: "mixed-groups-authd-first", localGroupsFile: "user_mismatching_groups.group"},
		"UID_does_not_change_if_user_already_exists":                        {userCase: "same-name-different-uid", dbFile: "one_user_and_group", wantSameUID: true},
		"Successfully update user with different capitalization":            {userCase: "different-capitalization-same-uid", dbFile: "one_user_and_group"},
		"GID_does_not_change_if_group_with_same_UGID_exists":                {groupsCase: "different-name-same-ugid", dbFile: "one_user_and_group"},
		"GID_does_not_change_if_group_with_same_name_and_empty_UGID_exists": {groupsCase: "authd-group", dbFile: "group-with-empty-UGID"},
		"Removing_last_user_from_a_group_keeps_the_group_record":            {groupsCase: "no-groups", dbFile: "one_user_and_group"},
		"Names_of_authd_groups_are_stored_in_lowercase":                     {groupsCase: "authd-group-with-uppercase"},

		"Error_if_user_has_no_username":                           {userCase: "nameless", wantErr: true, noOutput: true},
		"Error_if_group_has_no_name":                              {groupsCase: "nameless-group", wantErr: true, noOutput: true},
		"Error_if_group_has_conflicting_gid":                      {groupsCase: "different-name-same-gid", dbFile: "one_user_and_group", wantErr: true, noOutput: true},
		"Error_if_group_with_same_name_but_different_UGID_exists": {groupsCase: "authd-group", dbFile: "one_user_and_group", wantErr: true, noOutput: true},
		"Error_if_user_exists_on_system":                          {userCase: "user-exists-on-system", wantErr: true, noOutput: true},
		"Error_if_group_exists_on_system":                         {groupsCase: "group-exists-on-system", wantErr: true, noOutput: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var destGroupFile string
			if tc.localGroupsFile != "" {
				destGroupFile = localgroupstestutils.SetupGroupMock(t,
					filepath.Join("testdata", "groups", tc.localGroupsFile))
			}

			if tc.userCase == "" {
				tc.userCase = "user1"
			}

			user := userCases[tc.userCase]
			user.Dir = "/home/" + user.Name
			user.Shell = "/bin/bash"
			user.Gecos = "gecos for " + user.Name
			for _, g := range groupsCases[tc.groupsCase] {
				user.Groups = append(user.Groups, g.GroupInfo)
			}

			dbDir := t.TempDir()
			if tc.dbFile != "" {
				err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
				require.NoError(t, err, "Setup: could not create database from testdata")
			}

			var gids []uint32
			for _, group := range groupsCases[tc.groupsCase] {
				if group.GID != 0 {
					gids = append(gids, group.GID)
				}
			}

			managerOpts := []users.Option{
				users.WithIDGenerator(&idgenerator.IDGeneratorMock{
					UIDsToGenerate: []uint32{user.UID},
					GIDsToGenerate: gids,
				}),
			}
			m := newManagerForTests(t, dbDir, managerOpts...)

			var oldUID uint32
			if tc.wantSameUID {
				oldUser, err := m.UserByName(user.Name)
				require.NoError(t, err, "UserByName should not return an error, but did")
				oldUID = oldUser.UID
			}

			err := m.UpdateUser(user.UserInfo)
			log.Debugf(context.Background(), "UpdateUser error: %v", err)

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr && tc.noOutput {
				return
			}

			if tc.wantSameUID {
				newUser, err := m.UserByName(user.Name)
				require.NoError(t, err, "UserByName should not return an error, but did")
				require.Equal(t, oldUID, newUser.UID, "UID should not have changed")
			}

			got, err := db.Z_ForTests_DumpNormalizedYAML(userstestutils.GetManagerDB(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdate(t, got)

			localgroupstestutils.RequireGroupFile(t, destGroupFile, golden.Path(t))
		})
	}
}

func TestRegisterUserPreauth(t *testing.T) {
	userCases := map[string]userCase{
		"user1":                   {UserInfo: types.UserInfo{Name: "user1"}, UID: 1111},
		"nameless":                {UID: 1111},
		"same-name-different-uid": {UserInfo: types.UserInfo{Name: "user1"}, UID: 3333},
		"user-exists-on-system":   {UserInfo: types.UserInfo{Name: "root"}, UID: 1111},
	}

	tests := map[string]struct {
		userCase string

		dbFile string

		wantUserInDB bool
		wantErr      bool
	}{
		"Successfully_update_user": {},
		"Successfully_if_user_already_exists_on_db": {
			userCase: "same-name-different-uid", dbFile: "one_user_and_group", wantUserInDB: true,
		},

		"Error_if_user_has_no_username":  {userCase: "nameless", wantErr: true},
		"Error_if_user_exists_on_system": {userCase: "user-exists-on-system", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.userCase == "" {
				tc.userCase = "user1"
			}

			user := userCases[tc.userCase]

			dbDir := t.TempDir()
			if tc.dbFile != "" {
				err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
				require.NoError(t, err, "Setup: could not create database from testdata")
			}

			managerOpts := []users.Option{
				users.WithIDGenerator(&idgenerator.IDGeneratorMock{
					UIDsToGenerate: []uint32{user.UID},
				}),
			}
			m := newManagerForTests(t, dbDir, managerOpts...)

			uid, err := m.RegisterUserPreAuth(user.Name)

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr {
				return
			}

			_, err = m.UserByName(user.Name)
			if tc.wantUserInDB {
				require.NoError(t, err, "UserByName should not return an error, but did")
			} else {
				require.Error(t, err, "UserByName should return an error, but did not")
			}

			newUser, err := m.UserByID(uid)
			require.NoError(t, err, "UserByID should not return an error, but did")

			require.Equal(t, uid, newUser.UID, "UID should not have changed")

			if tc.wantUserInDB {
				require.Equal(t, user.Name, newUser.Name, "User name does not match")
			} else {
				require.True(t, strings.HasPrefix(newUser.Name, tempentries.UserPrefix),
					"Pre-auth users should have %q as prefix: %q", tempentries.UserPrefix,
					newUser.Name)
				newUser.Name = tempentries.UserPrefix + "-{{random-suffix}}"
			}

			golden.CheckOrUpdateYAML(t, newUser)
		})
	}
}

func TestBrokerForUser(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string
		dbFile   string

		wantBrokerID string
		wantErr      bool
		wantErrType  error
	}{
		"Successfully_get_broker_for_user":                     {username: "user1", dbFile: "multiple_users_and_groups", wantBrokerID: "broker-id"},
		"Return_no_broker_but_in_db_if_user_has_no_broker_yet": {username: "userwithoutbroker", dbFile: "multiple_users_and_groups", wantBrokerID: ""},

		"Error_if_user_does_not_exist": {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: db.NoDataFoundError{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")
			m := newManagerForTests(t, dbDir)

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
	t.Parallel()

	tests := map[string]struct {
		username string

		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully_update_broker_for_user": {},

		"Error_if_user_does_not_exist": {username: "doesnotexist", wantErrType: db.NoDataFoundError{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.username == "" {
				tc.username = "user1"
			}
			if tc.dbFile == "" {
				tc.dbFile = "multiple_users_and_groups"
			}

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")
			m := newManagerForTests(t, dbDir)

			err = m.UpdateBrokerForUser(tc.username, "ExampleBrokerID")

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			got, err := db.Z_ForTests_DumpNormalizedYAML(userstestutils.GetManagerDB(m))
			require.NoError(t, err, "Created database should be valid yaml content")

			golden.CheckOrUpdate(t, got)
		})
	}
}

func TestUserByIDAndName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		uid        uint32
		username   string
		dbFile     string
		isTempUser bool

		wantErr     bool
		wantErrType error
	}{
		"Successfully_get_user_by_ID":           {uid: 1111, dbFile: "multiple_users_and_groups"},
		"Successfully_get_user_by_name":         {username: "user1", dbFile: "multiple_users_and_groups"},
		"Successfully_get_temporary_user_by_ID": {dbFile: "multiple_users_and_groups", isTempUser: true},

		"Error_if_user_does_not_exist_-_by_ID":   {uid: 0, dbFile: "multiple_users_and_groups", wantErrType: db.NoDataFoundError{}},
		"Error_if_user_does_not_exist_-_by_name": {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: db.NoDataFoundError{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")

			m := newManagerForTests(t, dbDir)

			if tc.isTempUser {
				tc.uid, err = m.RegisterUserPreAuth("tempuser1")
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

			// Registering a temporary user creates it with a random UID, GID, and gecos, so we have to make it
			// deterministic before comparing it with the golden file
			if tc.isTempUser {
				require.True(t, strings.HasPrefix(user.Name, tempentries.UserPrefix))
				user.Name = tempentries.UserPrefix + "{{random-suffix}}"
				require.Equal(t, tc.uid, user.UID)
				user.UID = 0
				require.Equal(t, tc.uid, user.GID)
				user.GID = 0
				require.NotEmpty(t, user.Gecos)
				user.Gecos = ""
			}

			golden.CheckOrUpdateYAML(t, user)
		})
	}
}

func TestAllUsers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr     bool
		wantErrType error
	}{
		"Successfully_get_all_users": {dbFile: "multiple_users_and_groups"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")
			m := newManagerForTests(t, dbDir)

			got, err := m.AllUsers()

			requireErrorAssertions(t, err, tc.wantErrType, tc.wantErr)
			if tc.wantErrType != nil || tc.wantErr {
				return
			}

			golden.CheckOrUpdateYAML(t, got)
		})
	}
}

func TestGroupByIDAndName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		gid       uint32
		groupname string
		dbFile    string

		wantErr     bool
		wantErrType error
	}{
		"Successfully_get_group_by_ID":   {gid: 11111, dbFile: "multiple_users_and_groups"},
		"Successfully_get_group_by_name": {groupname: "group1", dbFile: "multiple_users_and_groups"},

		"Error_if_group_does_not_exist_-_by_ID":   {gid: 0, dbFile: "multiple_users_and_groups", wantErrType: db.NoDataFoundError{}},
		"Error_if_group_does_not_exist_-_by_name": {groupname: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: db.NoDataFoundError{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")
			m := newManagerForTests(t, dbDir)

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

			golden.CheckOrUpdateYAML(t, group)
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
		"Successfully_get_all_groups": {dbFile: "multiple_users_and_groups"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")

			m := newManagerForTests(t, dbDir)

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
	t.Parallel()

	tests := map[string]struct {
		username string
		dbFile   string

		wantErr     bool
		wantErrType error
	}{
		"Successfully_get_shadow_by_name": {username: "user1", dbFile: "multiple_users_and_groups"},

		"Error_if_shadow_does_not_exist": {username: "doesnotexist", dbFile: "multiple_users_and_groups", wantErrType: db.NoDataFoundError{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")

			m := newManagerForTests(t, dbDir)

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
	t.Parallel()

	tests := map[string]struct {
		dbFile string

		wantErr bool
	}{
		"Successfully_get_all_users": {dbFile: "multiple_users_and_groups"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", tc.dbFile+".db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")

			m := newManagerForTests(t, dbDir)

			got, err := m.AllShadows()

			requireErrorAssertions(t, err, nil, tc.wantErr)
			if tc.wantErr {
				return
			}

			golden.CheckOrUpdateYAML(t, got)
		})
	}
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

func newManagerForTests(t *testing.T, dbDir string, opts ...users.Option) *users.Manager {
	t.Helper()

	m, err := users.NewManager(users.DefaultConfig, dbDir, opts...)
	require.NoError(t, err, "NewManager should not return an error, but did")

	return m
}

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)

	userslocking.Z_ForTests_OverrideLocking()
	defer userslocking.Z_ForTests_RestoreLocking()

	m.Run()
}
