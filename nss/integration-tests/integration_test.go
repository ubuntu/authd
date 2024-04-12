package nss_test

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	grouptests "github.com/ubuntu/authd/internal/users/localgroups/tests"
)

var daemonPath string

func TestIntegration(t *testing.T) {
	t.Parallel()

	libPath, rustCovEnv := buildRustNSSLib(t)

	// Create a default daemon to use for most test cases.
	defaultSocket := filepath.Join(os.TempDir(), "nss-integration-tests.sock")
	defaultDbState := "multiple_users_and_groups"
	defaultOutputPath := filepath.Join(filepath.Dir(daemonPath), "gpasswd.output")
	defaultGroupsFilePath := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	ctx, cancel := context.WithCancel(context.Background())
	_, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithSocketPath(defaultSocket),
		testutils.WithPreviousDBState(defaultDbState),
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, defaultOutputPath, defaultGroupsFilePath)...),
	)

	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	originOuts := map[string]string{}
	for _, db := range []string{"passwd", "group", "shadow"} {
		//#nosec:G204 - We control the cmd arguments in tests.
		data, err := exec.Command("getent", db).CombinedOutput()
		require.NoError(t, err, "Setup: can't run getent to get original output from system")
		originOuts[db] = string(data)
	}

	tests := map[string]struct {
		db      string
		key     string
		cacheDB string

		noDaemon       bool
		noCustomSocket bool
		wantSecondCall bool
		shouldPreCheck bool

		wantErr bool
	}{
		"Get all entries from passwd": {db: "passwd"},
		"Get all entries from group":  {db: "group"},
		"Get all entries from shadow": {db: "shadow"},

		"Get entry from passwd by name": {db: "passwd", key: "user1"},
		"Get entry from group by name":  {db: "group", key: "group1"},
		"Get entry from shadow by name": {db: "shadow", key: "user1"},

		"Get entry from passwd by id": {db: "passwd", key: "1111"},
		"Get entry from group by id":  {db: "group", key: "11111"},

		"Check user with broker if not found in cache": {db: "passwd", key: "user-pre-check", shouldPreCheck: true},

		// Even though those are "error" cases, the getent command won't fail since the other databases on the machine will return some entries.
		"Returns empty when getting all entries from passwd and daemon is not available": {db: "passwd", noDaemon: true},
		"Returns empty when getting all entries from group and daemon is not available":  {db: "group", noDaemon: true},
		"Returns empty when getting all entries from shadow and daemon is not available": {db: "shadow", noDaemon: true},

		"Returns empty when getting all entries from passwd after cleaning corrupted database": {db: "passwd", cacheDB: "invalid_entry_in_userByID", wantSecondCall: true},
		"Returns empty when getting all entries from group after cleaning corrupted database":  {db: "group", cacheDB: "invalid_entry_in_groupByID", wantSecondCall: true},
		"Returns empty when getting all entries from shadow after cleaning corrupted database": {db: "shadow", cacheDB: "invalid_entry_in_userByID", wantSecondCall: true},

		/* Error cases */
		// We can't assert on the returned error type since the error returned by getent will always be 2 (i.e. Not Found), even though the library returns other types.
		"Error when getting passwd by name and entry does not exist":                        {db: "passwd", key: "doesnotexit", wantErr: true},
		"Error when getting passwd by name entry exists in broker but precheck is disabled": {db: "passwd", key: "user-pre-check", wantErr: true},
		"Error when getting group by name and entry does not exist":                         {db: "group", key: "doesnotexit", wantErr: true},
		"Error when getting shadow by name and entry does not exist":                        {db: "shadow", key: "doesnotexit", wantErr: true},

		"Error when getting passwd by id and entry does not exist": {db: "passwd", key: "404", wantErr: true},
		"Error when getting group by id and entry does not exist":  {db: "group", key: "404", wantErr: true},

		"Error when getting passwd by name and daemon is not available": {db: "passwd", key: "user1", noDaemon: true, wantErr: true},
		"Error when getting group by name and daemon is not available":  {db: "group", key: "group1", noDaemon: true, wantErr: true},
		"Error when getting shadow by name and daemon is not available": {db: "shadow", key: "user1", noDaemon: true, wantErr: true},

		"Error when getting passwd by id and daemon is not available": {db: "passwd", key: "1111", noDaemon: true, wantErr: true},
		"Error when getting group by id and daemon is not available":  {db: "group", key: "11111", noDaemon: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.cacheDB == "" {
				tc.cacheDB = defaultDbState
			}

			var socketPath string
			if !tc.noDaemon && !tc.noCustomSocket {
				socketPath = defaultSocket
				if tc.cacheDB != defaultDbState {
					// Run a new daemon with a different cache state for special test cases.
					outPath := filepath.Join(t.TempDir(), "gpasswd.output")
					groupsFilePath := filepath.Join("testdata", "empty.group")

					var daemonStopped chan struct{}
					ctx, cancel := context.WithCancel(context.Background())
					socketPath, daemonStopped = testutils.RunDaemon(ctx, t, daemonPath,
						testutils.WithPreviousDBState(tc.cacheDB),
						testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, outPath, groupsFilePath)...),
					)
					t.Cleanup(func() {
						cancel()
						<-daemonStopped
					})
				}
			}

			cmds := []string{"getent", tc.db}
			if tc.key != "" {
				cmds = append(cmds, tc.key)
			}

			got, err := outNSSCommandForLib(t, libPath, socketPath, rustCovEnv, originOuts[tc.db], tc.shouldPreCheck, cmds...)
			if tc.wantErr {
				require.Error(t, err, "Expected an error, but got none")
				return
			}
			require.NoError(t, err, "Expected no error, but got %v", err)

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Outputs must match")

			// This is to check that some cache tasks, such as cleaning a corrupted database, work as expected.
			if tc.wantSecondCall {
				got, err := outNSSCommandForLib(t, libPath, socketPath, rustCovEnv, originOuts[tc.db], tc.shouldPreCheck, cmds...)
				require.NoError(t, err, "Expected no error, but got %v", err)
				require.Empty(t, got, "Expected empty output, but got %q", got)
			}
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	grouptests.Mockgpasswd(t)
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
