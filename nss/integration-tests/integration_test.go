package nss_test

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
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
	goldenTracker := testutils.NewGoldenTracker(t)

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
		db      string
		key     string
		cacheDB string

		noDaemon           bool
		currentUserNotRoot bool
		wantSecondCall     bool
		shouldPreCheck     bool

		wantStatus int
	}{
		"Get all entries from passwd":                    {db: "passwd"},
		"Get all entries from group":                     {db: "group"},
		"Get all entries from shadow if considered root": {db: "shadow"},

		"Get entry from passwd by name":                    {db: "passwd", key: "user1"},
		"Get entry from group by name":                     {db: "group", key: "group1"},
		"Get entry from shadow by name if considered root": {db: "shadow", key: "user1"},

		"Get entry from passwd by id": {db: "passwd", key: "1111"},
		"Get entry from group by id":  {db: "group", key: "11111"},

		"Check user with broker if not found in cache": {db: "passwd", key: "user-pre-check", shouldPreCheck: true},

		// Even though those are "error" cases, the getent command won't fail when trying to list content of a service.
		"Returns empty when getting all entries from shadow if regular user": {db: "shadow", currentUserNotRoot: true},

		"Returns empty when getting all entries from passwd and daemon is not available": {db: "passwd", noDaemon: true},
		"Returns empty when getting all entries from group and daemon is not available":  {db: "group", noDaemon: true},
		"Returns empty when getting all entries from shadow and daemon is not available": {db: "shadow", noDaemon: true},

		/* Error cases */
		// We can't assert on the returned error type since the error returned by getent will always be 2 (i.e. Not Found), even though the library returns other types.
		"Error when getting all entries from passwd and database is corrupted": {db: "passwd", cacheDB: "invalid_entry_in_userByID", wantSecondCall: true},
		"Error when getting all entries from group and database is corrupted":  {db: "group", cacheDB: "invalid_entry_in_groupByID", wantSecondCall: true},
		"Error when getting all entries from shadow and database is corrupted": {db: "shadow", cacheDB: "invalid_entry_in_userByID", wantSecondCall: true},

		"Error when getting shadow by name if regular user": {db: "shadow", key: "user1", currentUserNotRoot: true, wantStatus: codeNotFound},

		"Error when getting passwd by name and entry does not exist":                        {db: "passwd", key: "doesnotexit", wantStatus: codeNotFound},
		"Error when getting passwd by name entry exists in broker but precheck is disabled": {db: "passwd", key: "user-pre-check", wantStatus: codeNotFound},
		"Error when getting group by name and entry does not exist":                         {db: "group", key: "doesnotexit", wantStatus: codeNotFound},
		"Error when getting shadow by name and entry does not exist":                        {db: "shadow", key: "doesnotexit", wantStatus: codeNotFound},

		"Error when getting passwd by id and entry does not exist": {db: "passwd", key: "404", wantStatus: codeNotFound},
		"Error when getting group by id and entry does not exist":  {db: "group", key: "404", wantStatus: codeNotFound},

		"Error when getting passwd by name and daemon is not available": {db: "passwd", key: "user1", noDaemon: true, wantStatus: codeNotFound},
		"Error when getting group by name and daemon is not available":  {db: "group", key: "group1", noDaemon: true, wantStatus: codeNotFound},
		"Error when getting shadow by name and daemon is not available": {db: "shadow", key: "user1", noDaemon: true, wantStatus: codeNotFound},

		"Error when getting passwd by id and daemon is not available": {db: "passwd", key: "1111", noDaemon: true, wantStatus: codeNotFound},
		"Error when getting group by id and daemon is not available":  {db: "group", key: "11111", noDaemon: true, wantStatus: codeNotFound},

		/* Special cases */
		"Do not query the cache when user is pam_unix_non_existent": {db: "passwd", key: "pam_unix_non_existent:", cacheDB: "pam_unix_non_existent", wantStatus: codeNotFound},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socketPath := defaultSocket

			var useAlternativeDaemon bool
			if tc.cacheDB != "" || tc.currentUserNotRoot {
				useAlternativeDaemon = true
			} else {
				tc.cacheDB = defaultDbState
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
					testutils.WithPreviousDBState(tc.cacheDB),
					testutils.WithEnvironment(env...),
				)
				t.Cleanup(func() {
					cancel()
					<-daemonStopped
				})
			}

			cmds := []string{tc.db}
			if tc.key != "" {
				cmds = append(cmds, tc.key)
			}

			got, status := getentOutputForLib(t, libPath, socketPath, rustCovEnv, tc.shouldPreCheck, cmds...)
			require.Equal(t, tc.wantStatus, status, "Expected status %d, but got %d", tc.wantStatus, status)

			// If the exit status is NotFound, there is no need to create an empty golden file.
			// But we need to ensure that the output is indeed empty.
			if tc.wantStatus == codeNotFound {
				require.Empty(t, got, "Expected empty output, but got %q", got)
				return
			}

			want := testutils.LoadWithUpdateFromGolden(t, got,
				testutils.WithGoldenTracker(&goldenTracker))
			require.Equal(t, want, got, "Outputs must match")

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

	os.Exit(m.Run())
}
