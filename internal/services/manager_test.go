package services_test

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/services"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

			m, err := services.NewManager(context.Background(),
				tc.cacheDir, t.TempDir(), nil, users.DefaultConfig)
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

	m, err := services.NewManager(context.Background(), t.TempDir(), t.TempDir(), nil, users.DefaultConfig)
	require.NoError(t, err, "Setup: could not create manager for the test")
	defer require.NoError(t, m.Stop(), "Teardown: Stop should not have returned an error, but did")

	got := m.RegisterGRPCServices(context.Background()).GetServiceInfo()
	// Make the content of the golden file deterministic by sorting the methods by name.
	for _, info := range got {
		slices.SortFunc(info.Methods, func(a, b grpc.MethodInfo) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
	requireEqualServices(t, want, got)
}

func TestAccessAuthorization(t *testing.T) {
	t.Parallel()

	m, err := services.NewManager(context.Background(), t.TempDir(), t.TempDir(), nil, users.DefaultConfig)
	require.NoError(t, err, "Setup: could not create manager for the test")
	defer require.NoError(t, m.Stop(), "Teardown: Stop should not have returned an error, but did")

	grpcServer := m.RegisterGRPCServices(context.Background())

	// socket path is limited in length.
	tmpDir, err := os.MkdirTemp("", "authd-socket-dir")
	require.NoError(t, err, "Setup: could not setup temporary socket dir path")
	defer os.RemoveAll(tmpDir)
	socketPath := filepath.Join(tmpDir, "authd.sock")
	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Setup: could not create unix socket")
	defer lis.Close()

	serverDone := make(chan (error))
	go func() { serverDone <- grpcServer.Serve(lis) }()
	defer func() {
		grpcServer.Stop()
		require.NoError(t, <-serverDone, "gRPC server should not return an error from serving")
	}()

	conn, err := grpc.NewClient("unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(errmessages.FormatErrorMessage))
	require.NoError(t, err, "Setup: could not dial the server")

	// Global authorization for PAM is always denied for non root user.
	pamClient := authd.NewPAMClient(conn)
	_, err = pamClient.AvailableBrokers(context.Background(), &authd.Empty{})
	require.Error(t, err, "PAM calls are not allowed to any random user")

	// Global authorization for NSS is always granted for non root user.
	nssClient := authd.NewNSSClient(conn)
	_, err = nssClient.GetPasswdByName(context.Background(), &authd.GetPasswdByNameRequest{Name: ""})
	require.Error(t, err, "Expected a GRPC error from the server")

	err = conn.Close()
	require.NoError(t, err, "Teardown: could not close the client connection")
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
	// Start system bus mock.
	cleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	os.Exit(m.Run())
}
