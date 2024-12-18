package daemon_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/cmd/authd/daemon"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/users/cache"
)

func TestHelp(t *testing.T) {
	a := daemon.NewForTests(t, nil, "--help")

	getStdout := captureStdout(t)

	err := a.Run()
	require.NoErrorf(t, err, "Run should not return an error with argument --help. Stdout: %v", getStdout())
}

func TestCompletion(t *testing.T) {
	a := daemon.NewForTests(t, nil, "completion", "bash")

	getStdout := captureStdout(t)

	err := a.Run()
	require.NoError(t, err, "Completion should not start the daemon. Stdout: %v", getStdout())
}

func TestVersion(t *testing.T) {
	a := daemon.NewForTests(t, nil, "version")

	getStdout := captureStdout(t)

	err := a.Run()
	require.NoError(t, err, "Run should not return an error")

	out := getStdout()

	fields := strings.Fields(out)
	require.Len(t, fields, 2, "wrong number of fields in version: %s", out)

	want := "authd"

	require.Equal(t, want, fields[0], "Wrong executable name")
	require.Equal(t, consts.Version, fields[1], "Wrong version")
}

func TestNoUsageError(t *testing.T) {
	a := daemon.NewForTests(t, nil, "completion", "bash")

	getStdout := captureStdout(t)
	err := a.Run()

	require.NoError(t, err, "Run should not return an error, stdout: %v", getStdout())
	isUsageError := a.UsageError()
	require.False(t, isUsageError, "No usage error is reported as such")
}

func TestUsageError(t *testing.T) {
	t.Parallel()

	a := daemon.NewForTests(t, nil, "doesnotexist")

	err := a.Run()
	require.Error(t, err, "Run should return an error, stdout: %v")
	isUsageError := a.UsageError()
	require.True(t, isUsageError, "Usage error is reported as such")
}

func TestCanQuitWhenExecute(t *testing.T) {
	t.Parallel()

	a, wait := startDaemon(t, nil)
	defer wait()

	a.Quit()
}

func TestCanQuitTwice(t *testing.T) {
	t.Parallel()

	a, wait := startDaemon(t, nil)

	a.Quit()
	wait()

	require.NotPanics(t, a.Quit)
}

func TestAppCanQuitWithoutExecute(t *testing.T) {
	t.Skipf("This test is skipped because it is flaky. There is no way to guarantee Quit has been called before run.")

	t.Parallel()

	a := daemon.NewForTests(t, nil)

	requireGoroutineStarted(t, a.Quit)
	err := a.Run()
	require.Error(t, err, "Should return an error")

	require.Containsf(t, err.Error(), "grpc: the server has been stopped", "Unexpected error message")
}

func TestAppRunFailsOnComponentsCreationAndQuit(t *testing.T) {
	t.Parallel()
	// Trigger the error with a cache directory that cannot be created over an
	// existing file

	const (
		ok = iota
		dirIsFile
		hasWrongPermission
		parentDirDoesNotExists
	)

	testCases := map[string]struct {
		cacheDBBehavior    int
		cachePathBehavior  int
		socketPathBehavior int
	}{
		"Error on existing cache path not being a directory":    {cachePathBehavior: dirIsFile},
		"Error on existing cache path with invalid permissions": {cachePathBehavior: hasWrongPermission},
		"Error on missing parent cache directory":               {cachePathBehavior: parentDirDoesNotExists},

		"Error on grpc daemon creation failure": {socketPathBehavior: dirIsFile},

		"Error on manager creationg failure": {cacheDBBehavior: hasWrongPermission},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// We are using our own temporary directory for the unix socket path being too long.
			shortTmp, err := os.MkdirTemp("", "authd-tests")
			require.NoError(t, err, "Setup: could not create temporary directory")
			t.Cleanup(func() { _ = os.RemoveAll(shortTmp) })

			filePath := filepath.Join(shortTmp, "file")
			err = os.WriteFile(filePath, []byte("I'm here to break the service"), 0600)
			require.NoError(t, err, "Setup: failed to write file")

			worldAccessDir := filepath.Join(shortTmp, "opened-to-world")
			//nolint: gosec // This is a directory with invalid permission for tests.
			err = os.MkdirAll(worldAccessDir, 0777)
			require.NoError(t, err, "Setup: failed to write file")

			var config daemon.DaemonConfig
			switch tc.cachePathBehavior {
			case dirIsFile:
				config.Paths.Cache = filePath
			case hasWrongPermission:
				config.Paths.Cache = worldAccessDir
			case parentDirDoesNotExists:
				config.Paths.Cache = filepath.Join(shortTmp, "not-exists", "cache")
			}
			switch tc.socketPathBehavior {
			case dirIsFile:
				config.Paths.Socket = filepath.Join(filePath, "mysocket")
			default:
				config.Paths.Socket = filepath.Join(shortTmp, "mysocket")
			}
			switch tc.cacheDBBehavior {
			case hasWrongPermission:
				config.Paths.Cache = filepath.Join(shortTmp, "cache")
				err := os.MkdirAll(config.Paths.Cache, 0700)
				require.NoError(t, err, "Setup: could not create cache directory")
				//nolint: gosec // This is a file with invalid permission for tests.
				err = os.WriteFile(filepath.Join(config.Paths.Cache, cache.Z_ForTests_DBName()), nil, 0644)
				require.NoError(t, err, "Setup: could not create database with invalid permissions")
			}

			a := daemon.NewForTests(t, &config)

			err = a.Run()
			require.Error(t, err, "Run should exit with an error")
			a.Quit()
		})
	}
}

func TestAppCanSigHupWhenExecute(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err, "Setup: pipe shouldn't fail")

	a, wait := startDaemon(t, nil)

	defer wait()
	defer a.Quit()

	orig := os.Stdout
	os.Stdout = w

	a.Hup()

	os.Stdout = orig
	w.Close()

	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	require.NoError(t, err, "Couldn't copy stdout to buffer")
	require.NotEmpty(t, out.String(), "Stacktrace is printed")
}

func TestAppCanSigHupAfterExecute(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err, "Setup: pipe shouldn't fail")

	a, wait := startDaemon(t, nil)
	a.Quit()
	wait()

	orig := os.Stdout
	os.Stdout = w

	a.Hup()

	os.Stdout = orig
	w.Close()

	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	require.NoError(t, err, "Couldn't copy stdout to buffer")
	require.NotEmpty(t, out.String(), "Stacktrace is printed")
}

func TestAppCanSigHupWithoutExecute(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err, "Setup: pipe shouldn't fail")

	a := daemon.NewForTests(t, nil)

	orig := os.Stdout
	os.Stdout = w

	a.Hup()

	os.Stdout = orig
	w.Close()

	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	require.NoError(t, err, "Couldn't copy stdout to buffer")
	require.NotEmpty(t, out.String(), "Stacktrace is printed")
}

func TestAppGetRootCmd(t *testing.T) {
	t.Parallel()

	a := daemon.NewForTests(t, nil)
	require.NotNil(t, a.RootCmd(), "Returns root command")
}

func TestConfigLoad(t *testing.T) {
	customizedSocketPath := filepath.Join(t.TempDir(), "mysocket")
	var config daemon.DaemonConfig
	config.Verbosity = 1
	config.Paths.Socket = customizedSocketPath

	a, wait := startDaemon(t, &config)
	defer wait()
	defer a.Quit()

	_, err := os.Stat(customizedSocketPath)
	require.NoError(t, err, "Socket should exist")
	require.Equal(t, 1, a.Config().Verbosity, "Verbosity is set from config")
}

func TestAutoDetectConfig(t *testing.T) {
	customizedSocketPath := filepath.Join(t.TempDir(), "mysocket")
	var config daemon.DaemonConfig
	config.Verbosity = 1
	config.Paths.Socket = customizedSocketPath

	configPath := daemon.GenerateTestConfig(t, &config)
	configNextToBinaryPath := filepath.Join(filepath.Dir(os.Args[0]), "authd.yaml")
	err := os.Rename(configPath, configNextToBinaryPath)
	require.NoError(t, err, "Could not relocate authd configuration file in the binary directory")
	// Remove configuration next binary for other tests to not pick it up.
	defer os.Remove(configNextToBinaryPath)

	a := daemon.New()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := a.Run()
		require.NoError(t, err, "Run should exits without any error")
	}()
	a.WaitReady()
	time.Sleep(50 * time.Millisecond)

	defer wg.Wait()
	defer a.Quit()

	_, err = os.Stat(customizedSocketPath)
	require.NoError(t, err, "Socket should exist")
	require.Equal(t, 1, a.Config().Verbosity, "Verbosity is set from config")
}

func TestNoConfigSetDefaults(t *testing.T) {
	a := daemon.New()
	// Use version to still run preExec to load no config but without running server
	a.SetArgs("version")

	err := a.Run()
	require.NoError(t, err, "Run should not return an error")

	require.Equal(t, 0, a.Config().Verbosity, "Default Verbosity")
	require.Equal(t, consts.DefaultBrokersConfPath, a.Config().Paths.BrokersConf, "Default brokers configuration path")
	require.Equal(t, consts.DefaultCacheDir, a.Config().Paths.Cache, "Default cache directory")
	require.Equal(t, "", a.Config().Paths.Socket, "No socket address as default")
}

func TestBadConfigReturnsError(t *testing.T) {
	a := daemon.New()
	// Use version to still run preExec to load no config but without running server
	a.SetArgs("version", "--config", "/does/not/exist.yaml")

	err := a.Run()
	require.Error(t, err, "Run should return an error on config file")
}

// requireGoroutineStarted starts a goroutine and blocks until it has been launched.
func requireGoroutineStarted(t *testing.T, f func()) {
	t.Helper()

	launched := make(chan struct{})

	go func() {
		close(launched)
		f()
	}()

	<-launched
}

// startDaemon prepares and starts the daemon in the background. The done function should be called
// to wait for the daemon to stop.
func startDaemon(t *testing.T, conf *daemon.DaemonConfig) (app *daemon.App, done func()) {
	t.Helper()

	a := daemon.NewForTests(t, conf)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := a.Run()
		require.NoError(t, err, "Run should exits without any error")
	}()
	a.WaitReady()
	time.Sleep(50 * time.Millisecond)

	return a, func() {
		wg.Wait()
	}
}

// captureStdout capture current process stdout and returns a function to get the captured buffer.
func captureStdout(t *testing.T) func() string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err, "Setup: pipe shouldn't fail")

	orig := os.Stdout
	os.Stdout = w

	t.Cleanup(func() {
		os.Stdout = orig
		w.Close()
	})

	var out bytes.Buffer
	errch := make(chan error)
	go func() {
		_, err = io.Copy(&out, r)
		errch <- err
		close(errch)
	}()

	return func() string {
		w.Close()
		w = nil
		require.NoError(t, <-errch, "Couldn't copy stdout to buffer")

		return out.String()
	}
}

func TestMain(m *testing.M) {
	// Start system bus mock.
	cleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	m.Run()
}
