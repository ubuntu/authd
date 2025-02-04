package testutils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/grpcutils"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/users/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type daemonOptions struct {
	dbPath     string
	existentDB string
	socketPath string
	pidFile    string
	env        []string
}

// DaemonOption represents an optional function that can be used to override some of the daemon default values.
type DaemonOption func(*daemonOptions)

// WithDBPath overrides the default database path of the daemon.
func WithDBPath(path string) DaemonOption {
	return func(o *daemonOptions) {
		o.dbPath = path
	}
}

// WithPreviousDBState initializes the database of the daemon with a preexistent database.
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

// WithPidFile sets the path where the process pid will be saved while running.
// The pidFile is also special because when it gets removed, authd is stopped.
func WithPidFile(pidFile string) DaemonOption {
	return func(o *daemonOptions) {
		o.pidFile = pidFile
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

	if opts.dbPath == "" {
		opts.dbPath = filepath.Join(tempDir, "db")
		require.NoError(t, os.MkdirAll(opts.dbPath, 0700), "Setup: failed to create database dir")
	}

	if opts.existentDB != "" {
		db.Z_ForTests_CreateDBFromYAML(t, filepath.Join("testdata", "db", opts.existentDB+".db.yaml"), opts.dbPath)
	}

	if opts.socketPath == "" {
		opts.socketPath = filepath.Join(tempDir, "authd.socket")
	}

	config := fmt.Sprintf(`
verbosity: 2
paths:
  database: %s
  socket: %s
`, opts.dbPath, opts.socketPath)

	configPath := filepath.Join(tempDir, "testconfig.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600), "Setup: failed to create config file for tests")

	var cancel context.CancelCauseFunc
	if opts.pidFile != "" {
		ctx, cancel = context.WithCancelCause(ctx)
	}

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.CommandContext(ctx, execPath, "-c", configPath)
	opts.env = append(opts.env, os.Environ()...)
	opts.env = append(opts.env, fmt.Sprintf("AUTHD_EXAMPLE_BROKER_SLEEP_MULTIPLIER=%f", SleepMultiplier()))
	cmd.Env = AppendCovEnv(opts.env)

	// This is the function that is called by CommandContext when the context is cancelled.
	cmd.Cancel = func() error {
		defer os.RemoveAll(tempDir)
		return cmd.Process.Signal(os.Signal(syscall.SIGTERM))
	}

	// Start the daemon
	stopped = make(chan struct{})
	processPid := make(chan int)
	go func() {
		defer close(stopped)
		var b bytes.Buffer
		cmd.Stdout = &b
		cmd.Stderr = &b
		err := cmd.Start()
		require.NoError(t, err, "Setup: daemon cannot start %v", cmd.Args)
		if opts.pidFile != "" {
			processPid <- cmd.Process.Pid
		}
		err = cmd.Wait()
		out := b.Bytes()
		require.ErrorIs(t, err, context.Canceled, "Setup: daemon stopped unexpectedly: %s", out)
		if opts.pidFile != "" {
			defer cancel(nil)
			if err := os.Remove(opts.pidFile); err != nil {
				t.Logf("TearDown: failed to remove pid file %q: %v", opts.pidFile, err)
			}
		}
		t.Logf("Daemon stopped (%v)\n ##### Output #####\n %s \n ##### END #####", err, out)
	}()

	conn, err := grpc.NewClient("unix://"+opts.socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(errmessages.FormatErrorMessage))
	require.NoError(t, err, "Setup: could not connect to the daemon on %s", opts.socketPath)
	defer conn.Close()

	// Block until the daemon is started and ready to accept connections.
	err = grpcutils.WaitForConnection(ctx, conn, time.Second*30)
	require.NoError(t, err, "Setup: wait for daemon to be ready timed out")

	if opts.pidFile != "" {
		err := os.WriteFile(opts.pidFile, []byte(fmt.Sprint(<-processPid)), 0600)
		require.NoError(t, err, "Setup: cannot create PID file")

		// In case the pid file gets removed externally, close authd!
		// fsnotify watcher doesn't seem to work here, so let's go manual.
		go func() {
			for {
				f, err := os.Open(opts.pidFile)
				if err != nil {
					cancel(err)
					return
				}
				defer f.Close()
				<-time.After(time.Millisecond * 200)
			}
		}()
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
	if IsAsan() {
		cmd.Args = append(cmd.Args, "-asan")
	}
	if IsRace() {
		cmd.Args = append(cmd.Args, "-race")
	}
	cmd.Args = append(cmd.Args, "-gcflags=all=-N -l")
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Args = append(cmd.Args, "-o", execPath, "./cmd/authd")

	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to build daemon(%v): %s", err, out)
	}

	return execPath, cleanup, err
}
