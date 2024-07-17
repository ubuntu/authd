package daemon_test

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/daemon"
	"github.com/ubuntu/authd/internal/daemon/testdata/grpctestservice"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNew(t *testing.T) {
	t.Parallel()

	type socketType int
	const (
		systemdActivationListener socketType = iota
		manualSocket
		systemdActivationListenerAndManualSocket
		systemdActivationListenerMultipleSockets
		systemdActivationListenerFails
		systemdActivationListenerSocketDoesNotExists
		manualSocketParentDirectoryDoesNotExists
	)

	testCases := map[string]struct {
		socketType socketType

		wantSelectedSocket string
		wantErr            bool
	}{
		"With socket activation":                               {wantSelectedSocket: "systemd.sock1"},
		"Socket provided manually is created":                  {socketType: manualSocket, wantSelectedSocket: "manual.sock"},
		"Socket provided manually wins over socket activation": {socketType: systemdActivationListenerAndManualSocket, wantSelectedSocket: "manual.sock"},

		"Error when systemd provides multiple sockets":             {socketType: systemdActivationListenerMultipleSockets, wantErr: true},
		"Error when systemd activation fails":                      {socketType: systemdActivationListenerFails, wantErr: true},
		"Error when systemd activated socket does not exists":      {socketType: systemdActivationListenerSocketDoesNotExists, wantErr: true},
		"Error when manually provided socket path does not exists": {socketType: manualSocketParentDirectoryDoesNotExists, wantErr: true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var registered bool
			registering := func(context.Context) *grpc.Server {
				registered = true
				return nil
			}

			// Prepare and create socket setup.
			var sockets []net.Listener
			socketDir := t.TempDir()
			for _, socket := range []string{"systemd.sock1", "systemd.sock2"} {
				l, err := net.Listen("unix", filepath.Join(socketDir, socket))
				require.NoErrorf(t, err, "setup failed: couldn't create unix socket: %v", err)
				defer l.Close()
				sockets = append(sockets, l)
			}
			manualSocketPath := filepath.Join(t.TempDir(), "manual.sock")

			// Setup socket environment based
			var args []daemon.Option
			switch tc.socketType {
			case systemdActivationListener:
				args = append(args, daemon.WithSystemdActivationListener(
					func() ([]net.Listener, error) {
						return []net.Listener{sockets[0]}, nil
					}))
			case manualSocket:
				args = append(args, daemon.WithSocketPath(manualSocketPath))
			case systemdActivationListenerAndManualSocket:
				args = append(args, daemon.WithSystemdActivationListener(
					func() ([]net.Listener, error) {
						return []net.Listener{sockets[0]}, nil
					}),
					daemon.WithSocketPath(manualSocketPath),
				)
			case systemdActivationListenerMultipleSockets:
				args = append(args, daemon.WithSystemdActivationListener(
					func() ([]net.Listener, error) {
						return []net.Listener{sockets[0], sockets[1]}, nil
					}))
			case systemdActivationListenerFails:
				args = append(args, daemon.WithSystemdActivationListener(
					func() ([]net.Listener, error) {
						return nil, errors.New("systemd activation error")
					}))
			case systemdActivationListenerSocketDoesNotExists:
				sockets[0].Close()
				args = append(args, daemon.WithSystemdActivationListener(
					func() ([]net.Listener, error) {
						return []net.Listener{sockets[0]}, nil
					}))
			case manualSocketParentDirectoryDoesNotExists:
				err := os.Remove(filepath.Dir(manualSocketPath))
				require.NoError(t, err, "Setup: removing manual socket dir fails")
				args = append(args, daemon.WithSocketPath(manualSocketPath))
			}

			// Test itself
			d, err := daemon.New(context.Background(), registering, args...)
			if tc.wantErr {
				require.Error(t, err, "New() should return an error")
				return
			}
			require.NoError(t, err, "New() should not return an error")

			require.True(t, registered, "daemon should register GRPC services")
			require.Equal(t, tc.wantSelectedSocket, filepath.Base(d.SelectedSocketAddr()), "Desired socket is selected")
		})
	}
}

func TestServe(t *testing.T) {
	t.Parallel()

	type systemdNotifierType int

	const (
		systemdNotifierOk systemdNotifierType = iota
		systemdNotifierFails
		noSystemdNotifier
	)

	testCases := map[string]struct {
		systemdNotifier systemdNotifierType
		quitBeforeServe bool

		wantErr bool
	}{
		"Success with systemd notifier":    {},
		"Success without systemd notifier": {systemdNotifier: noSystemdNotifier},

		"Error on call to Quit before serve": {quitBeforeServe: true, wantErr: true},
		"Error on systemd notifier failing":  {systemdNotifier: systemdNotifierFails, wantErr: true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			registerGRPC := func(context.Context) *grpc.Server {
				return grpc.NewServer(grpc.UnaryInterceptor(errmessages.RedactErrorInterceptor))
			}
			socketPath := filepath.Join(t.TempDir(), "manual.socket")

			var systemdNotifier func(unsetEnvironment bool, state string) (bool, error)
			switch tc.systemdNotifier {
			case systemdNotifierOk:
				systemdNotifier = func(unsetEnvironment bool, state string) (bool, error) {
					return true, nil
				}
			case noSystemdNotifier:
				systemdNotifier = func(unsetEnvironment bool, state string) (bool, error) {
					return false, nil
				}
			case systemdNotifierFails:
				systemdNotifier = func(unsetEnvironment bool, state string) (bool, error) {
					return false, errors.New("systemd notifier failure")
				}
			}

			d, err := daemon.New(context.Background(), registerGRPC,
				daemon.WithSystemdSdNotifier(systemdNotifier),
				daemon.WithSocketPath(filepath.Join(t.TempDir(), "manual.socket")))
			require.NoError(t, err, "Setup: New() should not return an error")

			if tc.quitBeforeServe {
				d.Quit(context.Background(), false)
			}

			go func() {
				// make sure Serve() is called. Even std golang grpc has this timeout in tests
				time.Sleep(time.Millisecond * 10)
				d.Quit(context.Background(), false)
			}()

			err = d.Serve(context.Background())
			if tc.wantErr {
				require.Error(t, err, "Serve() should return an error")
				return
			}
			require.NoError(t, err, "Serve() should not return an error")

			_, err = os.Stat(socketPath)
			require.ErrorIs(t, err, fs.ErrNotExist, "socket should be cleaned up")
		})
	}
}
func TestQuit(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		force bool

		activeConnection bool

		wantErr bool
	}{
		"Graceful stop": {},
		"Graceful stop is blocked on active connection": {activeConnection: true},
		"Force stop drops active connection":            {force: true, activeConnection: true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			grpcServer := grpc.NewServer(grpc.UnaryInterceptor(errmessages.RedactErrorInterceptor))
			defer grpcServer.Stop()
			registerGRPC := func(context.Context) *grpc.Server {
				var service testGRPCService
				grpctestservice.RegisterTestServiceServer(grpcServer, service)
				return grpcServer
			}
			systemdNotifier := func(unsetEnvironment bool, state string) (bool, error) {
				return true, nil
			}

			socketPath := filepath.Join(t.TempDir(), "manual.socket")
			d, err := daemon.New(context.Background(), registerGRPC,
				daemon.WithSystemdSdNotifier(systemdNotifier),
				daemon.WithSocketPath(socketPath))
			require.NoError(t, err, "Setup: New() should not return an error")

			go func() {
				err = d.Serve(context.Background())
				require.NoError(t, err, "Serve() should not return an error")
			}()

			// make sure Serve() is called. Even std golang grpc has this timeout in tests
			time.Sleep(100 * time.Millisecond)

			var disconnectClient func()
			if tc.activeConnection {
				var connected bool
				connected, disconnectClient = createClientConnection(t, socketPath)
				require.True(t, connected, "new connection should be made allowed")
				defer disconnectClient()
			}

			// Request quitting.
			quiteDone := make(chan struct{})
			go func() {
				defer close(quiteDone)
				d.Quit(context.Background(), tc.force)
			}()

			time.Sleep(100 * time.Millisecond)

			// Any new connection is disallowed
			connected, _ := createClientConnection(t, socketPath)
			require.False(t, connected, "new connection should be disallowed")

			serverHasQuit := func() bool {
				select {
				case _, running := <-quiteDone:
					return !running
				default:
					return false
				}
			}

			if !tc.activeConnection || tc.force {
				require.Eventually(t, serverHasQuit, 100*time.Millisecond, 10*time.Millisecond, "Server should quit with no active connection or force")
				return
			}

			time.Sleep(100 * time.Millisecond)
			require.False(t, serverHasQuit(), "Server should still be running because of active connection and not forced")

			// drop connection
			disconnectClient()

			require.Eventually(t, serverHasQuit, 100*time.Millisecond, 10*time.Millisecond, "Server should quit with no more active connection")
		})
	}
}

func createClientConnection(t *testing.T, socketPath string) (success bool, disconnect func()) {
	t.Helper()

	ctx, disconnect := context.WithCancel(context.Background())

	var conn *grpc.ClientConn
	connected := make(chan struct{})
	go func() {
		defer close(connected)
		var err error
		conn, err = grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(errmessages.FormatErrorMessage))
		require.NoError(t, err, "Could not connect to grpc server")

		// The daemon tests require an active connection, so we need to block here until the connection is ready.
		conn.Connect()
		for conn.GetState() != connectivity.Ready {
			conn.WaitForStateChange(context.Background(), conn.GetState())
		}
	}()
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		disconnect()
		return false, func() {}
	}

	client := grpctestservice.NewTestServiceClient(conn)
	go func() { _, _ = client.Blocking(ctx, &grpctestservice.Empty{}) }()
	time.Sleep(10 * time.Millisecond)

	return true, disconnect
}

// Our mock GRPC service.
type testGRPCService struct {
	grpctestservice.UnimplementedTestServiceServer
}

func (testGRPCService) Blocking(ctx context.Context, e *grpctestservice.Empty) (*grpctestservice.Empty, error) {
	<-ctx.Done()
	return &grpctestservice.Empty{}, nil
}
