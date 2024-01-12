package nss_test

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/newusers"
	"github.com/ubuntu/authd/internal/newusers/cache"
	cachetests "github.com/ubuntu/authd/internal/newusers/cache/tests"
	"github.com/ubuntu/authd/internal/services/nss"
	"github.com/ubuntu/authd/internal/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

func TestNewService(t *testing.T) {
	t.Parallel()

	m, err := newusers.NewManager(t.TempDir())
	require.NoError(t, err, "Setup: could not create user manager")
	t.Cleanup(func() { _ = m.Stop() })

	_ = nss.NewService(context.Background(), m)
}

func TestGetPasswdByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		sourceDB string

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return existing user": {username: "user1"},

		"Error in database fetched content":                      {username: "user1", sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {username: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error on missing name":                                  {wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetPasswdByName(context.Background(), &authd.GetByNameRequest{Name: tc.username})
			requireExpectedResult(t, "GetPasswdByName", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

//nolint:dupl // This is a dedicated test, not a duplicate.
func TestGetPasswdByUID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		uid int

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
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetPasswdByUID(context.Background(), &authd.GetByIDRequest{Id: uint32(tc.uid)})
			requireExpectedResult(t, "GetPasswdByUID", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetPasswdEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sourceDB string

		wantErr bool
	}{
		"Return all users": {},
		"Return no users":  {sourceDB: "empty.db.yaml"},

		"Error in database fetched content": {sourceDB: "invalid.db.yaml", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetPasswdEntries(context.Background(), &authd.Empty{})
			requireExpectedEntriesResult(t, "GetPasswdEntries", got.GetEntries(), err, tc.wantErr)
		})
	}
}

func TestGetGroupByName(t *testing.T) {
	t.Parallel()

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
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetGroupByName(context.Background(), &authd.GetByNameRequest{Name: tc.groupname})
			requireExpectedResult(t, "GetGroupByName", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

//nolint:dupl // This is a dedicated test, not a duplicate.
func TestGetGroupByGID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		gid int

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
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetGroupByGID(context.Background(), &authd.GetByIDRequest{Id: uint32(tc.gid)})
			requireExpectedResult(t, "GetGroupByGID", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetGroupEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sourceDB string

		wantErr bool
	}{
		"Return all groups": {},
		"Return no groups":  {sourceDB: "empty.db.yaml"},

		"Error in database fetched content": {sourceDB: "invalid.db.yaml", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetGroupEntries(context.Background(), &authd.Empty{})
			requireExpectedEntriesResult(t, "GetGroupEntries", got.GetEntries(), err, tc.wantErr)
		})
	}
}

func TestGetShadowByName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		username string

		sourceDB string

		wantErr          bool
		wantErrNotExists bool
	}{
		"Return existing user": {username: "user1"},

		"Error in database fetched content":                      {username: "user1", sourceDB: "invalid.db.yaml", wantErr: true},
		"Error with typed GRPC notfound code on unexisting user": {username: "does-not-exists", wantErr: true, wantErrNotExists: true},
		"Error on missing name":                                  {wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetShadowByName(context.Background(), &authd.GetByNameRequest{Name: tc.username})
			requireExpectedResult(t, "GetShadowByName", got, err, tc.wantErr, tc.wantErrNotExists)
		})
	}
}

func TestGetShadowEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sourceDB string

		wantErr bool
	}{
		"Return all users": {},
		"Return no users":  {sourceDB: "empty.db.yaml"},

		"Error in database fetched content": {sourceDB: "invalid.db.yaml", wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newManagerForTests(t, tc.sourceDB)
			client := newNSSClient(t, c)

			got, err := client.GetShadowEntries(context.Background(), &authd.Empty{})
			requireExpectedEntriesResult(t, "GetShadowEntries", got.GetEntries(), err, tc.wantErr)
		})
	}
}

// newNSSClient returns a new GRPC PAM client for tests connected to the global brokerManager with the given user manager.
func newNSSClient(t *testing.T, m *newusers.Manager) (client authd.NSSClient) {
	t.Helper()

	// socket path is limited in length.
	tmpDir, err := os.MkdirTemp("", "authd-socket-dir")
	require.NoError(t, err, "Setup: could not setup temporary socket dir path")
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "authd.sock")

	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Setup: could not create unix socket")

	service := nss.NewService(context.Background(), m)

	grpcServer := grpc.NewServer()
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

	conn, err := grpc.Dial("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "Setup: Could not connect to GRPC server")
	t.Cleanup(func() { _ = conn.Close() }) // We don't care about the error on cleanup

	return authd.NewNSSClient(conn)
}

// newManagerForTests returns a cache object cleaned up with the test ends.
func newManagerForTests(t *testing.T, sourceDB string) *newusers.Manager {
	t.Helper()

	cacheDir := t.TempDir()

	if sourceDB == "" {
		sourceDB = "cache.db.yaml"
	}

	f, err := os.Open(filepath.Join("testdata", sourceDB))
	require.NoError(t, err, "Setup: could open fixture cache")
	defer f.Close()
	err = cachetests.DbfromYAML(f, cacheDir)
	require.NoError(t, err, "Setup: could not create database from YAML fixture")

	expiration, err := time.Parse(time.DateOnly, "2004-01-01")
	require.NoError(t, err, "Setup: could not parse time for testing")

	c, err := cache.New(cacheDir, cache.WithExpirationDate(expiration))
	require.NoError(t, err, "Setup: could not create cache")

	m, err := newusers.NewManager(cacheDir, newusers.WithCache(c))
	require.NoError(t, err, "Setup: could not create user manager")

	t.Cleanup(func() { _ = m.Stop() })
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

	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
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

	want := testutils.LoadWithUpdateFromGoldenYAML(t, got)
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
	testutils.InstallUpdateFlag()
	flag.Parse()

	m.Run()
}
