package nss_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	cachetests "github.com/ubuntu/authd/internal/cache/tests"
	"github.com/ubuntu/authd/internal/testutils"
)

var daemonPath string

// buildRustNSSLib builds the NSS library and links the compiled file to libPath.
func buildRustNSSLib(t *testing.T) {
	t.Helper()

	projectRoot := getProjectRoot()

	cargo := os.Getenv("CARGO_PATH")
	if cargo == "" {
		cargo = "cargo"
	}

	var target string
	rustDir := filepath.Join(projectRoot, "nss")
	rustCovEnv, target = testutils.TrackRustCoverage(t, rustDir)

	// Builds the nss library.
	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command(cargo, "build", "--verbose", "--all-features", "--target-dir", target)
	cmd.Env = append(os.Environ(), rustCovEnv...)
	cmd.Dir = projectRoot

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not build Rust NSS library: %s", out)

	// When building the crate with dh-cargo, this env is set to indicate which arquitecture the code
	// is being compiled to. When it's set, the compiled is stored under target/$(DEB_HOST_RUST_TYPE)/debug,
	// rather than under target/debug, so we need to append at the end of target to ensure we use
	// the right path.
	// If the env is not set, the target stays the same.
	target = filepath.Join(target, os.Getenv("DEB_HOST_RUST_TYPE"))

	// Creates a symlink for the compiled library with the expected versioned name.
	libPath = filepath.Join(target, "libnss_authd.so.2")
	if err = os.Symlink(filepath.Join(target, "debug", "libnss_authd.so"), libPath); err != nil {
		require.ErrorIs(t, err, os.ErrExist, "Setup: failed to create versioned link to the library")
	}
}

// outNSSCommandForLib returns the specific part for the nss command, filtering originOut.
// It uses the locally build authd nss module for the integration tests.
func outNSSCommandForLib(t *testing.T, socketPath, originOut string, cmds ...string) (got string, err error) {
	t.Helper()

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command(cmds[0], cmds[1:]...)
	cmd.Env = append(cmd.Env, rustCovEnv...)
	cmd.Env = append(cmd.Env,
		"AUTHD_NSS_DEBUG=stderr",
		// NSS needs both LD_PRELOAD and LD_LIBRARY_PATH to load the module library
		fmt.Sprintf("LD_PRELOAD=%s:%s", libPath, os.Getenv("LD_PRELOAD")),
		fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", filepath.Dir(libPath), os.Getenv("LD_LIBRARY_PATH")),
	)

	if socketPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_NSS_SOCKET=%s", socketPath))
	}

	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	return strings.Replace(out.String(), originOut, "", 1), err
}

func runDaemon(ctx context.Context, t *testing.T, cacheDB string) (socketPath string, stopped chan struct{}) {
	t.Helper()

	// Socket name has a maximum size, so we can't use t.TempDir() directly.
	tempDir, err := os.MkdirTemp("", "authd-nss-tests")
	require.NoError(t, err, "Setup: failed to create socket dir for tests")
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	cacheDir := filepath.Join(tempDir, "cache")
	if cacheDB != "" {
		require.NoError(t, os.MkdirAll(cacheDir, 0700), "Setup: failed to create cache dir")
		createDBFile(t, filepath.Join("testdata", "db", cacheDB+".db.yaml"), cacheDir)
	}
	socketPath = filepath.Join(tempDir, "authd.socket")

	config := fmt.Sprintf(`
verbosity: 2
paths:
  cache: %s
  socket: %s
`, cacheDir, socketPath)

	configPath := filepath.Join(tempDir, "testconfig.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600), "Setup: failed to create config file for tests")

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command(daemonPath, "-c", configPath)
	cmd.Stderr = os.Stderr
	cmd.Env = testutils.AppendCovEnv(cmd.Env)

	stopped = make(chan struct{})
	go func() {
		require.NoError(t, cmd.Start(), "Setup: daemon should start with no error")
		<-ctx.Done()
		// The daemon can trigger some background tasks so, in order to stop it gracefully, we need to send either
		// SIGTERM or SIGINT to tell it that it's time to cleanup and stop.
		require.NoError(t, cmd.Process.Signal(os.Signal(syscall.SIGTERM)), "Teardown: Failed to send signal to stop daemon")
		require.NoError(t, cmd.Wait(), "Teardown: daemon should stop with no error")
		close(stopped)
	}()

	// Give some time for the daemon to start.
	time.Sleep(500 * time.Millisecond)

	return socketPath, stopped
}

// buildDaemon builds the daemon executable and returns the binary path.
func buildDaemon() (execPath string, cleanup func(), err error) {
	projectRoot := getProjectRoot()

	tempDir, err := os.MkdirTemp("", "authd-tests-daemon")
	if err != nil {
		return "", nil, fmt.Errorf("Setup: failed to create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	execPath = filepath.Join(tempDir, "authd")
	cmd := exec.Command("go", "build")
	cmd.Dir = projectRoot
	if testutils.CoverDir() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	cmd.Args = append(cmd.Args, "-o", execPath, "./cmd/authd")

	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("Setup: failed to build daemon: %w", err)
	}

	return execPath, cleanup, err
}

func getProjectRoot() string {
	// Gets the path to the integration-tests.
	_, p, _, _ := runtime.Caller(0)
	l := strings.Split(filepath.Dir(p), "/")

	// Walk up the tree to get the path of the project root
	return "/" + filepath.Join(l[:len(l)-2]...)
}

// createDBFile creates the database inside destDir and loads the src file content into it.
func createDBFile(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = cachetests.DbfromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}
