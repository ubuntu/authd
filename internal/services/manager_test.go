package services_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/services"
	servicestests "github.com/ubuntu/authd/internal/services/tests"
	"github.com/ubuntu/authd/internal/testutils"
	usertests "github.com/ubuntu/authd/internal/users/tests"
	"google.golang.org/grpc"
)

func TestNewManager(t *testing.T) {
	tests := map[string]struct {
		cacheDir string

		systemBusSocket string
		cleanupInterval time.Duration
		groupsFile      string

		wantErr bool
	}{
		"Successfully create the manager":                        {},
		"Successfully create the manager cleaning system groups": {cleanupInterval: 2 * time.Second, groupsFile: "groups.group"},
		"Successfully create the manager even if cleanup fails":  {cleanupInterval: 2 * time.Second, groupsFile: "nonexistent.group"},

		"Error when can not create cache":          {cacheDir: "doesnotexist", wantErr: true},
		"Error when can not create broker manager": {systemBusSocket: "doesnotexist", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			if tc.cacheDir == "" {
				tc.cacheDir = t.TempDir()
			}
			if tc.systemBusSocket != "" {
				t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", tc.systemBusSocket)
			}

			destCmdsFile := filepath.Join(t.TempDir(), "gpasswd.output")
			if tc.cleanupInterval != 0 {
				groupFilePath := filepath.Join(testutils.TestFamilyPath(t), tc.groupsFile)
				cmdArgs := []string{"env", "GO_WANT_HELPER_PROCESS=1",
					fmt.Sprintf("GO_WANT_HELPER_PROCESS_DEST=%s", destCmdsFile),
					fmt.Sprintf("GO_WANT_HELPER_PROCESS_GROUPFILE=%s", groupFilePath),
					os.Args[0], "-test.run=TestMockgpasswd", "--"}
				usertests.OverrideDefaultOptions(t, groupFilePath, cmdArgs)
				servicestests.OverrideCleanupInterval(t, tc.cleanupInterval)
			}

			m, err := services.NewManager(context.Background(), tc.cacheDir, t.TempDir(), nil)
			if tc.wantErr {
				require.Error(t, err, "NewManager should have returned an error, but did not")
				return
			}
			require.NoError(t, err, "NewManager should not have returned an error, but did")

			if tc.cleanupInterval > 0 {
				// Sleep for a while to ensure the cleanup interval is triggered.
				time.Sleep(tc.cleanupInterval + time.Second)
			}

			if tc.groupsFile == "groups.group" {
				got := usertests.IdemnpotentOutputFromGPasswd(t, destCmdsFile)
				want := testutils.LoadWithUpdateFromGolden(t, got)
				require.Equal(t, want, got, "Clean up should do the expected gpasswd operation, but did not")
			}

			require.NoError(t, m.Stop(), "Teardown: Stop should not have returned an error, but did")
		})
	}
}

func TestRegisterGRPCServices(t *testing.T) {
	t.Parallel()

	m, err := services.NewManager(context.Background(), t.TempDir(), t.TempDir(), nil)
	require.NoError(t, err, "Setup: could not create manager for the test")
	defer require.NoError(t, m.Stop(), "Teardown: Stop should not have returned an error, but did")

	got := m.RegisterGRPCServices(context.Background()).GetServiceInfo()
	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
	requireEqualServices(t, want, got)
}

// requireEqualServices asserts that the grpc services were registered as expected.
//
// This is needed because the order of the methods and the services is not guaranteed.
func requireEqualServices(t *testing.T, want, got map[string]grpc.ServiceInfo) {
	t.Helper()

	for name, wantInfo := range want {
		gotInfo, ok := got[name]
		if !ok {
			t.Error("Expected services to match, but didn't")
			return
		}
		require.ElementsMatch(t, wantInfo.Methods, gotInfo.Methods, "Expected methods to match, but didn't")
		delete(got, name)
	}
	require.Empty(t, got, "Expected no extra services, but got %v", got)
}

func TestMockgpasswd(t *testing.T) {
	usertests.Mockgpasswd(t)
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	testutils.InstallUpdateFlag()
	flag.Parse()

	// Start system bus mock.
	cleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	m.Run()
}
