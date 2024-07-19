package testutils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/services/errmessages"
	cachetestutils "github.com/ubuntu/authd/internal/users/cache/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

type daemonOptions struct {
	cachePath  string
	existentDB string
	socketPath string
	env        []string
}

// DaemonOption represents an optional function that can be used to override some of the daemon default values.
type DaemonOption func(*daemonOptions)

// WithCachePath overrides the default cache path of the daemon.
func WithCachePath(path string) DaemonOption {
	return func(o *daemonOptions) {
		o.cachePath = path
	}
}

// WithPreviousDBState initializes the cache of the daemon with a preexistent database.
func WithPreviousDBState(db string) DaemonOption {
	return func(o *daemonOptions) {
		o.existentDB = db
	}
}

// WithSocketPath overrides the default socket path of the daemon.
func WithSocketPath(path string) DaemonOption {
	return func(o *daemonOptions) {
		o.socketPath = path
	}
}

// WithEnvironment overrides the default environment of the daemon.
func WithEnvironment(env ...string) DaemonOption {
	return func(o *daemonOptions) {
		o.env = env
	}
}

// RunDaemon runs the daemon in a separate process and returns the socket path and a channel that will be closed when
// the daemon stops.
func RunDaemon(ctx context.Context, t *testing.T, execPath string, args ...DaemonOption) (socketPath string, stopped chan struct{}) {
	t.Helper()

	opts := &daemonOptions{}
	for _, opt := range args {
		opt(opts)
	}

	// Socket name has a maximum size, so we can't use t.TempDir() directly.
	tempDir, err := os.MkdirTemp("", "authd-daemon4tests")
	require.NoError(t, err, "Setup: failed to create temp dir for tests")
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	if opts.cachePath == "" {
		opts.cachePath = filepath.Join(tempDir, "cache")
		require.NoError(t, os.MkdirAll(opts.cachePath, 0700), "Setup: failed to create cache dir")
	}

	if opts.existentDB != "" {
		cachetestutils.CreateDBFromYAML(t, filepath.Join("testdata", "db", opts.existentDB+".db.yaml"), opts.cachePath)
	}

	if opts.socketPath == "" {
		opts.socketPath = filepath.Join(tempDir, "authd.socket")
	}

	config := fmt.Sprintf(`
verbosity: 2
paths:
  cache: %s
  socket: %s
`, opts.cachePath, opts.socketPath)

	configPath := filepath.Join(tempDir, "testconfig.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600), "Setup: failed to create config file for tests")

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.CommandContext(ctx, execPath, "-c", configPath)
	opts.env = append(opts.env, os.Environ()...)
	cmd.Env = AppendCovEnv(opts.env)

	// This is the function that is called by CommandContext when the context is cancelled.
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Signal(syscall.SIGTERM))
	}

	// Start the daemon
	stopped = make(chan struct{})
	go func() {
		defer close(stopped)
		out, err := cmd.CombinedOutput()
		require.ErrorIs(t, err, context.Canceled, "Setup: daemon stopped unexpectedly: %s", out)
		t.Logf("Daemon stopped (%v)\n ##### STDOUT #####\n %s \n ##### END #####", err, out)
	}()

	conn, err := grpc.NewClient("unix://"+opts.socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(errmessages.FormatErrorMessage))
	require.NoError(t, err, "Setup: could not connect to the daemon on %s", opts.socketPath)
	defer conn.Close()

	waitCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	// Block until the daemon is started and ready to accept connections.
	conn.Connect()
	for conn.GetState() != connectivity.Ready {
		conn.WaitForStateChange(waitCtx, conn.GetState())
		require.NoError(t, waitCtx.Err(), "Setup: wait for daemon to be ready timed out")
	}

	return opts.socketPath, stopped
}

// BuildDaemon builds the daemon executable and returns the binary path.
func BuildDaemon(extraArgs ...string) (execPath string, cleanup func(), err error) {
	projectRoot := ProjectRoot()

	tempDir, err := os.MkdirTemp("", "authd-tests-daemon")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	execPath = filepath.Join(tempDir, "authd")
	cmd := exec.Command("go", "build")
	cmd.Dir = projectRoot
	if CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Args = append(cmd.Args, "-o", execPath, "./cmd/authd")

	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to build daemon(%v): %s", err, out)
	}

	return execPath, cleanup, err
}
