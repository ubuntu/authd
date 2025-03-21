package user_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/services/user"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/db"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestNewService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userManager := &users.Manager{}

	s := user.NewService(ctx, userManager)

	require.NotNil(t, s, "NewService should return a non-nil Service")
}

//nolint:dupl // This is not a duplicate test
func TestListUsers(t *testing.T) {
	tests := map[string]struct {
		dbFile string

		wantErr bool
	}{
		"Return_all_users": {},
		"Return_no_users":  {dbFile: "empty.db.yaml"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			if tc.dbFile == "" {
				tc.dbFile = "default.db.yaml"
			}

			client := newUserServiceClient(t, tc.dbFile)

			resp, err := client.ListUsers(context.Background(), &authd.Empty{})
			if tc.wantErr {
				require.Error(t, err, "ListUsers should return an error")
				s, ok := status.FromError(err)
				require.True(t, ok, "ListUsers should return a gRPC error")
				require.NotEqual(t, codes.NotFound, s.Code(), "ListUsers should not return NotFound error even with empty list")
				return
			}

			golden.CheckOrUpdateYAML(t, resp)
		})
	}
}

//nolint:dupl // This is not a duplicate test
func TestListGroups(t *testing.T) {
	tests := map[string]struct {
		dbFile string

		wantErr bool
	}{
		"Return_all_groups": {},
		"Return_no_groups":  {dbFile: "empty.db.yaml"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			if tc.dbFile == "" {
				tc.dbFile = "default.db.yaml"
			}

			client := newUserServiceClient(t, tc.dbFile)

			resp, err := client.ListGroups(context.Background(), &authd.Empty{})
			if tc.wantErr {
				require.Error(t, err, "ListGroups should return an error")
				s, ok := status.FromError(err)
				require.True(t, ok, "ListGroups should return a gRPC error")
				require.NotEqual(t, codes.NotFound, s.Code(), "ListGroups should not return NotFound error even with empty list")
				return
			}

			golden.CheckOrUpdateYAML(t, resp)
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
}

// newUserServiceClient returns a new gRPC client for the CLI service.
func newUserServiceClient(t *testing.T, dbFile string) (client authd.UserServiceClient) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "authd-socket-dir")
	require.NoError(t, err, "Setup: could not setup temporary socket dir path")
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "authd.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Setup: could not create unix socket")

	dbDir := t.TempDir()
	if dbFile != "" {
		err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", dbFile), dbDir)
		require.NoError(t, err, "Setup: could not create database from testdata")
	}

	userManager, err := users.NewManager(users.DefaultConfig, dbDir)
	require.NoError(t, err, "Setup: could not create users manager")

	service := user.NewService(context.Background(), userManager)

	grpcServer := grpc.NewServer(permissions.WithUnixPeerCreds(), grpc.UnaryInterceptor(errmessages.RedactErrorInterceptor))
	authd.RegisterUserServiceServer(grpcServer, service)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		<-done
	})

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "Setup: Could not connect to gRPC server")

	t.Cleanup(func() { _ = conn.Close() }) // We don't care about the error on cleanup

	return authd.NewUserServiceClient(conn)
}
