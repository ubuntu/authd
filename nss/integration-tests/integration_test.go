package nss_test

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/testutils/golden"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
)

var daemonPath string

func TestIntegration(t *testing.T) {
	t.Parallel()

	// codeNotFound is the expected exit code for the getent subprocess in case of errors.
	const codeNotFound int = 2

	libPath, rustCovEnv := buildRustNSSLib(t)

	// Create a default daemon to use for most test cases.
	defaultSocket := filepath.Join(os.TempDir(), "nss-integration-tests.sock")
	defaultDbState := "multiple_users_and_groups"
	defaultOutputPath := filepath.Join(filepath.Dir(daemonPath), "gpasswd.output")
	defaultGroupsFilePath := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	env := append(localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, defaultOutputPath, defaultGroupsFilePath), "AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT=1")
	ctx, cancel := context.WithCancel(context.Background())
	_, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithSocketPath(defaultSocket),
		testutils.WithPreviousDBState(defaultDbState),
		testutils.WithEnvironment(env...),
	)

	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	tests := map[string]struct {
		getentDB string
		key      string
		dbState  string

		noDaemon           bool
		currentUserNotRoot bool
		wantSecondCall     bool
		shouldPreCheck     bool

		wantStatus int
	}{
		"Get_all_entries_from_passwd":                    {getentDB: "passwd"},
		"Get_all_entries_from_group":                     {getentDB: "group"},
		"Get_all_entries_from_shadow_if_considered_root": {getentDB: "shadow"},

		"Get_entry_from_passwd_by_name":                    {getentDB: "passwd", key: "user1"},
		"Get_entry_from_group_by_name":                     {getentDB: "group", key: "group1"},
		"Get_entry_from_shadow_by_name_if_considered_root": {getentDB: "shadow", key: "user1"},

		"Get_entry_from_passwd_by_id": {getentDB: "passwd", key: "1111"},
		"Get_entry_from_group_by_id":  {getentDB: "group", key: "11111"},

		"Check_user_with_broker_if_not_found_in_db": {getentDB: "passwd", key: "user-pre-check", shouldPreCheck: true},

		// Even though those are "error" cases, the getent command won't fail when trying to list content of a service.
		"Returns_empty_when_getting_all_entries_from_shadow_if_regular_user": {getentDB: "shadow", currentUserNotRoot: true},

		"Returns_empty_when_getting_all_entries_from_passwd_and_daemon_is_not_available": {getentDB: "passwd", noDaemon: true},
		"Returns_empty_when_getting_all_entries_from_group_and_daemon_is_not_available":  {getentDB: "group", noDaemon: true},
		"Returns_empty_when_getting_all_entries_from_shadow_and_daemon_is_not_available": {getentDB: "shadow", noDaemon: true},

		/* Error cases */
		"Error_when_getting_shadow_by_name_if_regular_user": {getentDB: "shadow", key: "user1", currentUserNotRoot: true, wantStatus: codeNotFound},

		"Error_when_getting_passwd_by_name_and_entry_does_not_exist":                        {getentDB: "passwd", key: "doesnotexit", wantStatus: codeNotFound},
		"Error_when_getting_passwd_by_name_entry_exists_in_broker_but_precheck_is_disabled": {getentDB: "passwd", key: "user-pre-check", wantStatus: codeNotFound},
		"Error_when_getting_group_by_name_and_entry_does_not_exist":                         {getentDB: "group", key: "doesnotexit", wantStatus: codeNotFound},
		"Error_when_getting_shadow_by_name_and_entry_does_not_exist":                        {getentDB: "shadow", key: "doesnotexit", wantStatus: codeNotFound},

		"Error_when_getting_passwd_by_id_and_entry_does_not_exist": {getentDB: "passwd", key: "404", wantStatus: codeNotFound},
		"Error_when_getting_group_by_id_and_entry_does_not_exist":  {getentDB: "group", key: "404", wantStatus: codeNotFound},

		"Error_when_getting_passwd_by_name_and_daemon_is_not_available": {getentDB: "passwd", key: "user1", noDaemon: true, wantStatus: codeNotFound},
		"Error_when_getting_group_by_name_and_daemon_is_not_available":  {getentDB: "group", key: "group1", noDaemon: true, wantStatus: codeNotFound},
		"Error_when_getting_shadow_by_name_and_daemon_is_not_available": {getentDB: "shadow", key: "user1", noDaemon: true, wantStatus: codeNotFound},

		"Error_when_getting_passwd_by_id_and_daemon_is_not_available": {getentDB: "passwd", key: "1111", noDaemon: true, wantStatus: codeNotFound},
		"Error_when_getting_group_by_id_and_daemon_is_not_available":  {getentDB: "group", key: "11111", noDaemon: true, wantStatus: codeNotFound},

		/* Special cases */
		"Do_not_query_the_db_when_user_is_pam_unix_non_existent": {getentDB: "passwd", key: "pam_unix_non_existent:", dbState: "pam_unix_non_existent", wantStatus: codeNotFound},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socketPath := defaultSocket

			var useAlternativeDaemon bool
			if tc.dbState != "" || tc.currentUserNotRoot {
				useAlternativeDaemon = true
			} else {
				tc.dbState = defaultDbState
			}

			// We don't check compatibility of arguments, have noDaemon taking precedences to the others.
			if tc.noDaemon {
				socketPath = ""
				useAlternativeDaemon = false
			}

			if useAlternativeDaemon {
				// Run a specific new daemon for special test cases.
				outPath := filepath.Join(t.TempDir(), "gpasswd.output")
				groupsFilePath := filepath.Join("testdata", "empty.group")

				var daemonStopped chan struct{}
				ctx, cancel := context.WithCancel(context.Background())
				env := localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, outPath, groupsFilePath)
				if !tc.currentUserNotRoot {
					env = append(env, "AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT=1")
				}
				socketPath, daemonStopped = testutils.RunDaemon(ctx, t, daemonPath,
					testutils.WithPreviousDBState(tc.dbState),
					testutils.WithEnvironment(env...),
				)
				t.Cleanup(func() {
					cancel()
					<-daemonStopped
				})
			}

			cmds := []string{tc.getentDB}
			if tc.key != "" {
				cmds = append(cmds, tc.key)
			}

			got, status := getentOutputForLib(t, libPath, socketPath, rustCovEnv, tc.shouldPreCheck, cmds...)
			require.Equal(t, tc.wantStatus, status, "Expected status %d, but got %d", tc.wantStatus, status)

			if tc.shouldPreCheck && tc.getentDB == "passwd" {
				// When pre-checking, the `getent passwd` output contains a randomly generated UID.
				// To make the test deterministic, we replace the UID with a placeholder.
				// The output looks something like this:
				//     user-pre-check:x:1776689191:0:gecos for user-pre-check:/home/user-pre-check:/usr/bin/bash\n
				fields := strings.Split(got, ":")
				require.Len(t, fields, 7, "Invalid number of fields in the output: %q", got)
				// The UID is the third field.
				fields[2] = "{{UID}}"
				got = strings.Join(fields, ":")
			}

			// If the exit status is NotFound, there is no need to create an empty golden file.
			// But we need to ensure that the output is indeed empty.
			if tc.wantStatus == codeNotFound {
				require.Empty(t, got, "Expected empty output, but got %q", got)
				return
			}

			golden.CheckOrUpdate(t, got)

			// This is to check that some cache tasks, such as cleaning a corrupted database, work as expected.
			if tc.wantSecondCall {
				got, status := getentOutputForLib(t, libPath, socketPath, rustCovEnv, tc.shouldPreCheck, cmds...)
				require.NotEqual(t, codeNotFound, status, "Expected no error, but got %v", status)
				require.Empty(t, got, "Expected empty output, but got %q", got)
			}
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
}

func TestMain(m *testing.M) {
	// Needed to skip the test setup when running the gpasswd mock.
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	execPath, cleanup, err := testutils.BuildDaemon("-tags=withexamplebroker,integrationtests")
	if err != nil {
		log.Printf("Setup: failed to build daemon: %v", err)
		os.Exit(1)
	}
	defer cleanup()
	daemonPath = execPath

	m.Run()
}
