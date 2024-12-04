package nss_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/services/nss"
	"github.com/ubuntu/authd/internal/services/permissions"
	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/testutils/golden"
	"github.com/ubuntu/authd/internal/users"
	cachetestutils "github.com/ubuntu/authd/internal/users/cache/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
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
		"Return existing user": {username: "user1"},

		"Precheck user if not in cache":                                          {username: "user-pre-check", shouldPreCheck: true},
		"Prechecked user with upper cases in username has same id as lower case": {username: "User-Pre-Check", shouldPreCheck: true},

		"Error in database fetched content":                      {username: "user1", sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {username: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error on missing name":                                  {wantErr: true},

		"Error in database fetched content does not trigger precheck": {username: "user1", sourceDB: "invalid.db.yaml", shouldPreCheck: true, wantErr: true},
		"Error if user not in cache and precheck is disabled":         {username: "user-pre-check", wantErr: true, wantErrNotExists: true},
		"Error if user not in cache and precheck fails":               {username: "does-not-exist", sourceDB: "empty.db.yaml", shouldPreCheck: true, wantErr: true, wantErrNotExists: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return existing user": {uid: 1111},

		"Error in database fetched content":                      {uid: 1111, sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {uid: 4242, wantErr: true, wantErrNotExists: true},
		"Error on missing uid":                                   {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return all users": {},
		"Return no users":  {sourceDB: "empty.db.yaml"},

		"Error in database fetched content": {sourceDB: "invalid.db.yaml", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return existing group": {groupname: "group1"},

		"Error in database fetched content":                      {groupname: "group1", sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {groupname: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error on missing name":                                  {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return existing group": {gid: 11111},

		"Error in database fetched content":                      {gid: 1111, sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {gid: 4242, wantErr: true, wantErrNotExists: true},
		"Error on missing uid":                                   {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return all groups": {},
		"Return no groups":  {sourceDB: "empty.db.yaml"},

		"Error in database fetched content": {sourceDB: "invalid.db.yaml", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return existing user": {username: "user1"},

		"Error when not root":                                    {currentUserNotRoot: true, username: "user1", wantErr: true},
		"Error in database fetched content":                      {username: "user1", sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {username: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error on missing name":                                  {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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
		"Return all users": {},
		"Return no users":  {sourceDB: "empty.db.yaml"},

		"Error when not root":               {currentUserNotRoot: true, wantErr: true},
		"Error in database fetched content": {sourceDB: "invalid.db.yaml", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// We don't care about gpasswd output here as it's already covered in the cache unit tests.
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

// newNSSClient returns a new GRPC PAM client for tests with the provided sourceDB as its initial cache.
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
		opts = append(opts, permissionstestutils.WithCurrentUserAsRoot())
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
	require.NoError(t, err, "Setup: Could not connect to GRPC server")

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

// newUserManagerForTests returns a cache object cleaned up with the test ends.
func newUserManagerForTests(t *testing.T, sourceDB string) *users.Manager {
	t.Helper()

	cacheDir := t.TempDir()
	if sourceDB == "" {
		sourceDB = "cache.db.yaml"
	}
	cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", sourceDB), cacheDir)

	m, err := users.NewManager(users.DefaultConfig, cacheDir)
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
		require.True(t, ok, "The error is always a GRPC error")
		if wantErrNotExists {
			require.Equal(t, codes.NotFound, s.Code(), fmt.Sprintf("%s should return NotFound error", funcName))
		}
		return
	}
	require.NoError(t, err, fmt.Sprintf("%s should not return an error, but did", funcName))

	want := golden.LoadWithUpdateYAML(t, got)
	requireExportedEquals(t, want, got, fmt.Sprintf("%s should return the expected entry, but did not", funcName))
}

// requireExpectedEntriesResult asserts expected behaviour from any get* NSS request returning a list and can update them from golden content.
func requireExpectedEntriesResult[T authd.PasswdEntry | authd.GroupEntry | authd.ShadowEntry](t *testing.T, funcName string, got []*T, err error, wantErr bool) {
	t.Helper()

	if wantErr {
		require.Error(t, err, fmt.Sprintf("%s should return an error but did not", funcName))
		s, ok := status.FromError(err)
		require.True(t, ok, "The error is always a GRPC error")
		require.NotEqual(t, codes.NotFound, s.Code(), fmt.Sprintf("%s should never return NotFound error even with empty list", funcName))
		return
	}
	require.NoError(t, err, fmt.Sprintf("%s should not return an error, but did", funcName))

	want := golden.LoadWithUpdateYAML(t, got)
	if len(want) != len(got) {
		require.Equal(t, len(want), len(got), "Not the expected number of elements in the list. Wanted: %v\nGot: %v", want, got)
	}
	for i, e := range got {
		requireExportedEquals(t, want[i], e, fmt.Sprintf("%s should return the expected entry in the list, but did not", funcName))
	}
}

// requireExportedEquals compare *want to *got, only using the exported fields.
// It helps ensuring that we don’t end up in a lockcopy vetting warning when we directly
// compare the exported fields with require.EqualExportedValues.
func requireExportedEquals[T authd.PasswdEntry | authd.GroupEntry | authd.ShadowEntry](t *testing.T, want *T, got *T, msg string) {
	t.Helper()

	data, err := yaml.Marshal(got)
	require.NoError(t, err, "Setup: could not encode got")

	var exportedGotOnly *T
	err = yaml.Unmarshal(data, &exportedGotOnly)
	require.NoError(t, err, "Setup: could not decode got")

	require.Equal(t, want, exportedGotOnly, msg)
}

func TestMain(m *testing.M) {
	// Needed to skip the test setup when running the gpasswd mock.
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	cleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Println("Error starting system bus mock:", err)
		os.Exit(1)
	}
	defer cleanup()

	os.Exit(m.Run())
}
