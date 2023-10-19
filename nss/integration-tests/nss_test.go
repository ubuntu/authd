package nss_test

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
)

var libPath string
var rustCovEnv []string

func TestIntegration(t *testing.T) {
	t.Parallel()

	buildRustNSSLib(t)

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

		// Even though those are "error" cases, the getent command won't fail since the other databases on the machine will return some entries.
		"Returns empty when getting all entries from passwd and daemon is not available": {db: "passwd", noDaemon: true},
		"Returns empty when getting all entries from group and daemon is not available":  {db: "group", noDaemon: true},
		"Returns empty when getting all entries from shadow and daemon is not available": {db: "shadow", noDaemon: true},

		"Returns empty when getting all entries from passwd after cleaning corrupted database": {db: "passwd", cacheDB: "invalid_entry_in_userByID", wantSecondCall: true},
		"Returns empty when getting all entries from group after cleaning corrupted database":  {db: "group", cacheDB: "invalid_entry_in_groupByID", wantSecondCall: true},
		"Returns empty when getting all entries from shadow after cleaning corrupted database": {db: "shadow", cacheDB: "invalid_entry_in_userByID", wantSecondCall: true},

		/* Error cases */
		// We can't assert on the returned error type since the error returned by getent will always be 2 (i.e. Not Found), even though the library returns other types.
		"Error when getting passwd by name and entry does not exist": {db: "passwd", key: "doesnotexit", wantErr: true},
		"Error when getting group by name and entry does not exist":  {db: "group", key: "doesnotexit", wantErr: true},
		"Error when getting shadow by name and entry does not exist": {db: "shadow", key: "doesnotexit", wantErr: true},

		"Error when getting passwd by id and entry does not exist": {db: "passwd", key: "404", wantErr: true},
		"Error when getting group by id and entry does not exist":  {db: "group", key: "404", wantErr: true},

		"Error when getting passwd by name and daemon is not available": {db: "passwd", key: "user1", noDaemon: true, wantErr: true},
		"Error when getting group by name and daemon is not available":  {db: "group", key: "group1", noDaemon: true, wantErr: true},
		"Error when getting shadow by name and daemon is not available": {db: "shadow", key: "user1", noDaemon: true, wantErr: true},

		"Error when getting passwd by id and daemon is not available": {db: "passwd", key: "1111", noDaemon: true, wantErr: true},
		"Error when getting group by id and daemon is not available":  {db: "group", key: "11111", noDaemon: true, wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tc.cacheDB == "" {
				tc.cacheDB = "multiple_users_and_groups"
			}

			var socketPath string
			var daemonStopped chan struct{}
			if !tc.noDaemon && !tc.noCustomSocket {
				ctx, cancel := context.WithCancel(context.Background())
				socketPath, daemonStopped = runDaemon(ctx, t, tc.cacheDB)
				t.Cleanup(func() {
					cancel()
					<-daemonStopped
				})
			}

			cmds := []string{"getent", tc.db}
			if tc.key != "" {
				cmds = append(cmds, tc.key)
			}

			got, err := outNSSCommandForLib(t, socketPath, originOuts[tc.db], cmds...)
			if tc.wantErr {
				require.Error(t, err, "Expected an error, but got none")
				return
			}
			require.NoError(t, err, "Expected no error, but got %v", err)

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Outputs must match")

			// This is to check that some cache tasks, such as cleaning a corrupted database, work as expected.
			if tc.wantSecondCall {
				got, err := outNSSCommandForLib(t, socketPath, originOuts[tc.db], cmds...)
				require.NoError(t, err, "Expected no error, but got %v", err)
				require.Empty(t, got, "Expected empty output, but got %q", got)
			}
		})
	}
}

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()
	flag.Parse()

	execPath, cleanup, err := buildDaemon()
	if err != nil {
		log.Printf("Setup: failed to build daemon: %v", err)
		os.Exit(1)
	}
	defer cleanup()
	daemonPath = execPath

	// Creates the directory to store the coverage files for the integration tests.
	if testutils.WantCoverage() {
		rawCovDir = os.Getenv("RAW_COVER_DIR")
		if rawCovDir == "" {
			dir, err := os.MkdirTemp("", "authd-coverage")
			if err != nil {
				log.Printf("Setup: failed to create temp dir for coverage: %v", err)
				cleanup()
				os.Exit(24)
			}
			defer os.RemoveAll(dir)
			rawCovDir = dir
		}
	}

	code := m.Run()
	if code == 0 && rawCovDir != "" {
		coverprofile := filepath.Join(rawCovDir, "integration-tests.coverprofile")
		// #nosec:G204 - we control the command arguments in tests
		cmd := exec.Command("go", "tool", "covdata", "textfmt", fmt.Sprintf("-i=%s", rawCovDir), fmt.Sprintf("-o=%s", coverprofile))
		if err := cmd.Run(); err != nil {
			log.Printf("Teardown: failed to parse coverage files: %v", err)
			cleanup()
			os.RemoveAll(rawCovDir)
			os.Exit(24)
		}
		testutils.MarkCoverageForMerging(coverprofile)
	}

	if err := testutils.MergeCoverages(); err != nil {
		log.Printf("Teardown: failed to merge coverage files: %v", err)

		// This ensures that we fail the test if we can't merge the coverage files, if the test
		// was successful, otherwise we exit with the code returned by m.Run()
		if code == 0 {
			cleanup()
			os.RemoveAll(rawCovDir)
			defer os.Exit(24)
		}
	}
}
