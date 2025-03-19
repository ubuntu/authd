package nss_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/services/nss"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/idgenerator"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
	"github.com/ubuntu/authd/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestNewService(t *testing.T) {
	t.Parallel()

	m, err := users.NewManager(users.DefaultConfig, t.TempDir())
	require.NoError(t, err, "Setup: could not create user manager")
	t.Cleanup(func() { _ = m.Stop() })

	b, err := brokers.NewManager(context.Background(), t.TempDir(), nil)
	require.NoError(t, err, "Setup: could not create broker manager")

	pm := permissions.New()
	s := nss.NewService(context.Background(), m, b, &pm)

	require.NotNil(t, s, "NewService should return a service")
}

func TestGetPasswdByName(t *testing.T) {
	tests := map[string]struct {
		username string

		sourceDB       string
		shouldPreCheck bool

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return_existing_user":                               {username: "user1"},
		"Return existing user with different capitalization": {username: "User1"},

		"Precheck_user_if_not_in_db": {username: "user-pre-check", shouldPreCheck: true},
		"Prechecked_user_with_upper_cases_in_username_has_same_id_as_lower_case": {username: "User-Pre-Check", shouldPreCheck: true},

		"Error_with_typed_GRPC_notfound_code_on_unexisting_user": {username: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error_on_missing_name":                                  {wantErr: true},

		"Error_if_user_not_in_db_and_precheck_is_disabled": {username: "user-pre-check", wantErr: true, wantErrNotExists: true},
		"Error_if_user_not_in_db_and_precheck_fails":       {username: "does-not-exist", sourceDB: "empty.db.yaml", shouldPreCheck: true, wantErr: true, wantErrNotExists: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, false)

			got, err := client.GetPasswdByName(context.Background(), &authd.GetPasswdByNameRequest{Name: tc.username, ShouldPreCheck: tc.shouldPreCheck})
			requireExpectedResult(t, "GetPasswdByName", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetPasswdByUID(t *testing.T) {
	tests := map[string]struct {
		uid uint32

		sourceDB string

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return_existing_user": {uid: 1111},

		"Error_with_typed_GRPC_notfound_code_on_unexisting_user": {uid: 4242, wantErr: true, wantErrNotExists: true},
		"Error_on_missing_uid": {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, false)

			got, err := client.GetPasswdByUID(context.Background(), &authd.GetByIDRequest{Id: tc.uid})
			requireExpectedResult(t, "GetPasswdByUID", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetPasswdEntries(t *testing.T) {
	tests := map[string]struct {
		sourceDB string

		wantErr bool
	}{
		"Return_all_users": {},
		"Return_no_users":  {sourceDB: "empty.db.yaml"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, false)

			got, err := client.GetPasswdEntries(context.Background(), &authd.Empty{})
			requireExpectedEntriesResult(t, "GetPasswdEntries", got.GetEntries(), err, tc.wantErr)
		})
	}
}

func TestGetGroupByName(t *testing.T) {
	tests := map[string]struct {
		groupname string

		sourceDB string

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return_existing_group": {groupname: "group1"},

		"Error_with_typed_GRPC_notfound_code_on_unexisting_user": {groupname: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error_on_missing_name":                                  {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, false)

			got, err := client.GetGroupByName(context.Background(), &authd.GetGroupByNameRequest{Name: tc.groupname})
			requireExpectedResult(t, "GetGroupByName", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetGroupByGID(t *testing.T) {
	tests := map[string]struct {
		gid uint32

		sourceDB string

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return_existing_group": {gid: 11111},

		"Error_with_typed_GRPC_notfound_code_on_unexisting_user": {gid: 4242, wantErr: true, wantErrNotExists: true},
		"Error_on_missing_uid": {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, false)

			got, err := client.GetGroupByGID(context.Background(), &authd.GetByIDRequest{Id: tc.gid})
			requireExpectedResult(t, "GetGroupByGID", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetGroupEntries(t *testing.T) {
	tests := map[string]struct {
		sourceDB string

		wantErr bool
	}{
		"Return_all_groups": {},
		"Return_no_groups":  {sourceDB: "empty.db.yaml"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, false)

			got, err := client.GetGroupEntries(context.Background(), &authd.Empty{})
			requireExpectedEntriesResult(t, "GetGroupEntries", got.GetEntries(), err, tc.wantErr)
		})
	}
}

func TestGetShadowByName(t *testing.T) {
	tests := map[string]struct {
		username string

		sourceDB           string
		currentUserNotRoot bool

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return_existing_user": {username: "user1"},

		"Error_when_not_root": {currentUserNotRoot: true, username: "user1", wantErr: true},
		"Error_with_typed_GRPC_notfound_code_on_unexisting_user": {username: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error_on_missing_name":                                  {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, tc.currentUserNotRoot)

			got, err := client.GetShadowByName(context.Background(), &authd.GetShadowByNameRequest{Name: tc.username})
			requireExpectedResult(t, "GetShadowByName", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetShadowEntries(t *testing.T) {
	tests := map[string]struct {
		sourceDB           string
		currentUserNotRoot bool

		wantErr bool
	}{
		"Return_all_users": {},
		"Return_no_users":  {sourceDB: "empty.db.yaml"},

		"Error_when_not_root": {currentUserNotRoot: true, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the db unit tests.
			_ = localgroupstestutils.SetupGPasswdMock(t, filepath.Join("testdata", "empty.group"))

			client := newNSSClient(t, tc.sourceDB, tc.currentUserNotRoot)

			got, err := client.GetShadowEntries(context.Background(), &authd.Empty{})
			requireExpectedEntriesResult(t, "GetShadowEntries", got.GetEntries(), err, tc.wantErr)
		})
	}
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
}

// newNSSClient returns a new GRPC PAM client for tests with the provided sourceDB as its initial database.
func newNSSClient(t *testing.T, sourceDB string, currentUserNotRoot bool) (client authd.NSSClient) {
	t.Helper()

	// socket path is limited in length.
	tmpDir, err := os.MkdirTemp("", "authd-socket-dir")
	require.NoError(t, err, "Setup: could not setup temporary socket dir path")
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "authd.sock")

	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Setup: could not create unix socket")

	var opts []permissions.Option
	if !currentUserNotRoot {
		opts = append(opts, permissions.Z_ForTests_WithCurrentUserAsRoot())
	}
	pm := permissions.New(opts...)

	service := nss.NewService(context.Background(), newUserManagerForTests(t, sourceDB), newBrokersManagerForTests(t), &pm)

	grpcServer := grpc.NewServer(permissions.WithUnixPeerCreds(), grpc.ChainUnaryInterceptor(enableCheckGlobalAccess(service), errmessages.RedactErrorInterceptor))
	authd.RegisterNSSServer(grpcServer, service)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		<-done
	})

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "Setup: Could not connect to gRPC server")

	t.Cleanup(func() { _ = conn.Close() }) // We don't care about the error on cleanup

	return authd.NewNSSClient(conn)
}

func enableCheckGlobalAccess(s nss.Service) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := s.CheckGlobalAccess(ctx, info.FullMethod); err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}
}

// newUserManagerForTests returns a user manager object cleaned up with the test ends.
func newUserManagerForTests(t *testing.T, sourceDB string) *users.Manager {
	t.Helper()

	dbDir := t.TempDir()
	if sourceDB == "" {
		sourceDB = "default.db.yaml"
	}
	err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", sourceDB), dbDir)
	require.NoError(t, err, "Setup: could not create database from testdata")

	managerOpts := []users.Option{
		users.WithIDGenerator(&idgenerator.IDGeneratorMock{
			UIDsToGenerate: []uint32{1234},
			GIDsToGenerate: []uint32{1234},
		}),
	}

	m, err := users.NewManager(users.DefaultConfig, dbDir, managerOpts...)
	require.NoError(t, err, "Setup: could not create user manager")

	t.Cleanup(func() { _ = m.Stop() })
	return m
}

// newBrokersManagerForTests returns a new broker manager with a broker mock for tests, it's cleaned when the test ends.
func newBrokersManagerForTests(t *testing.T) *brokers.Manager {
	t.Helper()

	cfg, cleanup, err := testutils.StartBusBrokerMock(t.TempDir(), "BrokerMock")
	require.NoError(t, err, "Setup: could not start bus broker mock")
	t.Cleanup(cleanup)

	m, err := brokers.NewManager(context.Background(), filepath.Dir(cfg), nil)
	require.NoError(t, err, "Setup: could not create broker manager")
	t.Cleanup(m.Stop)

	return m
}

// requireExpectedResult asserts expected behaviour from any get* NSS requests and can update them from golden content.
func requireExpectedResult[T authd.PasswdEntry | authd.GroupEntry | authd.ShadowEntry](t *testing.T, funcName string, got *T, err error, wantErr, wantErrNotExists bool) {
	t.Helper()

	if wantErr {
		require.Error(t, err, fmt.Sprintf("%s should return an error but did not", funcName))
		s, ok := status.FromError(err)
		require.True(t, ok, "The error is always a gRPC error")
		if wantErrNotExists {
			require.Equal(t, codes.NotFound, s.Code(), fmt.Sprintf("%s should return NotFound error", funcName))
		}
		return
	}
	require.NoError(t, err, fmt.Sprintf("%s should not return an error, but did", funcName))

	golden.CheckOrUpdateYAML(t, got)
}

// requireExpectedEntriesResult asserts expected behaviour from any get* NSS request returning a list and can update them from golden content.
func requireExpectedEntriesResult[T authd.PasswdEntry | authd.GroupEntry | authd.ShadowEntry](t *testing.T, funcName string, got []*T, err error, wantErr bool) {
	t.Helper()

	if wantErr {
		require.Error(t, err, fmt.Sprintf("%s should return an error but did not", funcName))
		s, ok := status.FromError(err)
		require.True(t, ok, "The error is always a gRPC error")
		require.NotEqual(t, codes.NotFound, s.Code(), fmt.Sprintf("%s should never return NotFound error even with empty list", funcName))
		return
	}
	require.NoError(t, err, fmt.Sprintf("%s should not return an error, but did", funcName))

	golden.CheckOrUpdateYAML(t, got)
}

func TestMain(m *testing.M) {
	// Needed to skip the test setup when running the gpasswd mock.
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	log.SetLevel(log.DebugLevel)

	cleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Println("Error starting system bus mock:", err)
		os.Exit(1)
	}
	defer cleanup()

	m.Run()
}
