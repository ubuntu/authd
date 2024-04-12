package nss_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
)

// buildRustNSSLib builds the NSS library and links the compiled file to libPath.
func buildRustNSSLib(t *testing.T) (libPath string, rustCovEnv []string) {
	t.Helper()

	projectRoot := testutils.ProjectRoot()

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

	return libPath, rustCovEnv
}

// outNSSCommandForLib returns the specific part for the nss command, filtering originOut.
// It uses the locally build authd nss module for the integration tests.
func outNSSCommandForLib(t *testing.T, libPath, socketPath string, rustCovEnv []string, originOut string, shouldPreCheck bool, cmds ...string) (got string, err error) {
	t.Helper()

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command(cmds[0], cmds[1:]...)
	cmd.Env = append(cmd.Env, rustCovEnv...)
	cmd.Env = append(cmd.Env,
		"AUTHD_NSS_INFO=stderr",
		// NSS needs both LD_PRELOAD and LD_LIBRARY_PATH to load the module library
		fmt.Sprintf("LD_PRELOAD=%s:%s", libPath, os.Getenv("LD_PRELOAD")),
		fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", filepath.Dir(libPath), os.Getenv("LD_LIBRARY_PATH")),
	)

	if socketPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_NSS_SOCKET=%s", socketPath))
	}

	if shouldPreCheck {
		cmd.Env = append(cmd.Env, "AUTHD_NSS_SHOULD_PRE_CHECK=1")
	}

	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	return strings.Replace(out.String(), originOut, "", 1), err
}
