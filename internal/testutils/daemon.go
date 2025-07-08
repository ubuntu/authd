package testutils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/grpcutils"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/localentries"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type daemonOptions struct {
	dbPath     string
	existentDB string
	socketPath string
	pidFile    string
	outputFile string
	shared     bool
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
		o.env = append(o.env, env...)
	}
}

// WithPidFile sets the path where the process pid will be saved while running.
// The pidFile is also special because when it gets removed, authd is stopped.
func WithPidFile(pidFile string) DaemonOption {
	return func(o *daemonOptions) {
		o.pidFile = pidFile
	}
}

// WithOutputFile sets the path where the process log will be saved.
func WithOutputFile(outputFile string) DaemonOption {
	return func(o *daemonOptions) {
		o.outputFile = outputFile
	}
}

// WithSharedDaemon sets whether the daemon is shared between tests.
func WithSharedDaemon(shared bool) DaemonOption {
	return func(o *daemonOptions) {
		o.shared = shared
	}
}

// WithHomeBaseDir sets the base path for the user home directories.
func WithHomeBaseDir(baseDir string) DaemonOption {
	return func(o *daemonOptions) {
		o.env = append(o.env, fmt.Sprintf("AUTHD_EXAMPLE_BROKER_HOME_BASE_DIR=%s", baseDir))
	}
}

// WithGroupFile sets the group file.
func WithGroupFile(groupFile string) DaemonOption {
	return func(o *daemonOptions) {
		o.env = slices.DeleteFunc(o.env, func(e string) bool {
			return strings.HasPrefix(e, localentries.Z_ForTests_GroupFilePathEnv+"=")
		})
		o.env = append(o.env, fmt.Sprintf("%s=%s", localentries.Z_ForTests_GroupFilePathEnv, groupFile))
	}
}

// WithGroupFileOutput sets the group output file.
func WithGroupFileOutput(groupFile string) DaemonOption {
	return func(o *daemonOptions) {
		o.env = slices.DeleteFunc(o.env, func(e string) bool {
			return strings.HasPrefix(e, localentries.Z_ForTests_GroupFileOutputPathEnv+"=")
		})
		o.env = append(o.env, fmt.Sprintf("%s=%s", localentries.Z_ForTests_GroupFileOutputPathEnv, groupFile))
	}
}

// WithCurrentUserAsRoot configures the daemon to accept the current user as root when checking permissions.
// This is useful for integration tests where the current user is not root, but we want to
// test the behavior as if it were root.
var WithCurrentUserAsRoot DaemonOption = func(o *daemonOptions) {
	o.env = append(o.env, "AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT=1")
}

// StartDaemon starts the daemon in a separate process and returns the socket path.
func StartDaemon(t *testing.T, execPath string, args ...DaemonOption) (socketPath string) {
	t.Helper()

	socketPath, cancelFunc := StartDaemonWithCancel(t, execPath, args...)
	t.Cleanup(cancelFunc)
	return socketPath
}

// StartDaemonWithCancel starts the daemon in a separate process and returns the socket path and a cancel function.
func StartDaemonWithCancel(t *testing.T, execPath string, args ...DaemonOption) (socketPath string, cancelFunc func()) {
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
	}

	if opts.existentDB != "" {
		require.NoError(t, os.MkdirAll(opts.dbPath, 0700), "Setup: failed to create database dir")
		err := db.Z_ForTests_CreateDBFromYAML(filepath.Join("testdata", "db", opts.existentDB+".db.yaml"), opts.dbPath)
		require.NoError(t, err, "Setup: could not create database from testdata")
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

	stopped := make(chan struct{})
	ctx, cancel := context.WithCancelCause(context.Background())
	cancelFunc = func() {
		t.Log("Stopping daemon...")
		cancel(nil)
		<-stopped
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
	processPid := make(chan int)
	go func() {
		defer close(stopped)
		var b bytes.Buffer
		cmd.Stdout = &b
		cmd.Stderr = &b

		t.Logf("Setup: starting daemon with command: %s", cmd.String())
		err := cmd.Start()
		require.NoError(t, err, "Setup: daemon cannot start %v", cmd.Args)
		if opts.pidFile != "" {
			processPid <- cmd.Process.Pid
		}

		// When using a shared daemon we should not use the test parameter from now on
		// since the test is referring to may not be the one actually running.
		t := t
		logger := t.Logf
		errorIs := func(err, target error, format string, args ...any) {
			require.ErrorIs(t, err, target, fmt.Sprintf(format, args...))
		}
		if opts.shared {
			// Unset the testing value, since it's wrong to use it from!
			t = nil
			logger = func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }
			errorIs = func(err, target error, format string, args ...any) {
				if errors.Is(err, target) {
					return
				}
				panic(fmt.Sprintf("Error %v is not matching %v: %s", err, target, fmt.Sprintf(format, args...)))
			}
		}

		err = cmd.Wait()
		out := b.Bytes()
		if opts.outputFile != "" {
			logger("writing authd log files to %v", opts.outputFile)
			if err := os.WriteFile(opts.outputFile, out, 0600); err != nil {
				logger("TearDown: failed to save output file %q: %v", opts.outputFile, err)
			}
		}
		errorIs(err, context.Canceled, "Setup: daemon stopped unexpectedly: %s", out)
		if opts.pidFile != "" {
			defer cancel(nil)
			if err := os.Remove(opts.pidFile); err != nil {
				logger("TearDown: failed to remove pid file %q: %v", opts.pidFile, err)
			}
		}
		logger("Daemon stopped (%v)\n ##### Output #####\n %s \n ##### END #####", err, out)
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

	return opts.socketPath, cancelFunc
}

// BuildDaemonWithExampleBroker builds the daemon executable and returns the binary path.
func BuildDaemonWithExampleBroker() (execPath string, cleanup func(), err error) {
	projectRoot := ProjectRoot()

	tempDir, err := os.MkdirTemp("", "authd-tests-daemon")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	execPath = filepath.Join(tempDir, "authd")
	cmd := exec.Command("go", "build")
	cmd.Dir = projectRoot
	cmd.Args = append(cmd.Args, GoBuildFlags()...)
	cmd.Args = append(cmd.Args, "-gcflags=all=-N -l")
	cmd.Args = append(cmd.Args, "-tags=withexamplebroker,integrationtests")
	cmd.Args = append(cmd.Args, "-o", execPath, "./cmd/authd")

	fmt.Fprintln(os.Stderr, "Running command:", cmd.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to build daemon(%v): %s", err, out)
	}

	return execPath, cleanup, err
}
