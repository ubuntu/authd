package users_test

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/log"
)

func TestSetUserID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		nonExistentUser                  bool
		emptyUsername                    bool
		uidAlreadySet                    bool
		uidAlreadyInUseByAuthdUser       bool
		uidAlreadyInUseAsGIDofAuthdUser  bool
		uidAlreadyInUseBySystemUser      bool
		uidAlreadyInUseAsGIDofSystemUser bool
		uidTooHigh                       bool

		wantErr     bool
		wantErrType error
	}{
		"Successfully_set_UID":                                               {},
		"Successfully_set_UID_if_ID_is_already_set":                          {uidAlreadySet: true},
		"Successfully_set_UID_if_ID_is_already_in_use_as_GID_of_system_user": {uidAlreadyInUseAsGIDofSystemUser: true},
		"Successfully_set_UID_if_ID_is_already_in_use_as_GID_of_authd_user":  {uidAlreadyInUseAsGIDofAuthdUser: true},

		"Error_if_username_is_empty":               {emptyUsername: true, wantErr: true},
		"Error_if_user_does_not_exist":             {nonExistentUser: true, wantErrType: db.NoDataFoundError{}},
		"Error_if_UID_is_already_in_use_by_authd":  {uidAlreadyInUseByAuthdUser: true, wantErr: true},
		"Error_if_UID_is_already_in_use_by_system": {uidAlreadyInUseBySystemUser: true, wantErr: true},
		"Error_if_UID_is_too_high":                 {uidTooHigh: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !testutils.RunningInBubblewrap() {
				testutils.RunTestInBubbleWrap(t)
				return
			}

			dbDir := t.TempDir()
			err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", "multiple_users_and_groups.db.yaml"), dbDir)
			require.NoError(t, err, "Setup: could not create database from testdata")

			m := newManagerForTests(t, dbDir)

			username := "user1"
			if tc.nonExistentUser {
				username = "nonexistent"
			}
			if tc.emptyUsername {
				username = ""
			}

			newUID := 123456
			if tc.uidAlreadySet {
				newUID = 1111
			}
			if tc.uidAlreadyInUseByAuthdUser {
				newUID = 2222
			}
			if tc.uidAlreadyInUseAsGIDofAuthdUser {
				newUID = 22222
			}
			if tc.uidTooHigh {
				newUID = math.MaxInt32 + 1
			}
			if tc.uidAlreadyInUseBySystemUser {
				err = addUserToSystem(newUID)
				require.NoError(t, err, "Setup: could not add user to system")
			}
			if tc.uidAlreadyInUseAsGIDofSystemUser {
				err = addGroupToSystem(newUID)
				require.NoError(t, err, "Setup: could not add group to system")
			}

			//nolint:gosec // G115 we set the UID above to values that are valid uint32
			_, err = m.SetUserID(username, uint32(newUID))
			log.Infof(context.Background(), "SetUserID error: %v", err)

			if tc.wantErrType != nil {
				require.ErrorIs(t, err, tc.wantErrType, "SetUserID should return expected error")
				return
			}
			if tc.wantErr {
				require.Error(t, err, "SetUserID should return an error but didn't")
				return
			}
			require.NoError(t, err, "SetUserID should not return an error on existing user")
		})
	}
}

func addUserToSystem(uid int) error {
	//nolint:gosec // G204 we want to use exec.Command with variables here
	cmd := exec.Command(
		"useradd",
		"--uid", strconv.Itoa(uid),
		"--no-create-home",
		fmt.Sprintf("test-%d", uid),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("useradd failed: %w, output: %s", err, output)
	}
	return nil
}

func addGroupToSystem(gid int) error {
	//nolint:gosec // G204 we want to use exec.Command with variables here
	cmd := exec.Command(
		"groupadd",
		"--gid", strconv.Itoa(gid),
		fmt.Sprintf("test-%d", gid),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("groupadd failed: %w, output: %s", err, output)
	}
	return nil
}
