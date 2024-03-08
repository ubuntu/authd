package services_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/services"
	"github.com/ubuntu/authd/internal/testutils"
	"google.golang.org/grpc"
)

func TestNewManager(t *testing.T) {
	tests := map[string]struct {
		cacheDir string

		systemBusSocket string

		wantErr bool
	}{
		"Successfully create the manager": {},

		"Error when can not create cache":          {cacheDir: "doesnotexist", wantErr: true},
		"Error when can not create broker manager": {systemBusSocket: "doesnotexist", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.cacheDir == "" {
				tc.cacheDir = t.TempDir()
			}
			if tc.systemBusSocket != "" {
				t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", tc.systemBusSocket)
			}

			m, err := services.NewManager(context.Background(), tc.cacheDir, t.TempDir(), nil)
			if tc.wantErr {
				require.Error(t, err, "NewManager should have returned an error, but did not")
				return
			}
			defer require.NoError(t, m.Stop(), "Teardown: Stop should not have returned an error, but did")

			require.NoError(t, err, "NewManager should not have returned an error, but did")
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

func TestMain(m *testing.M) {
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
