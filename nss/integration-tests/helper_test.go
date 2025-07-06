package nss_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

// getentOutputForLib returns the specific part for the nss command for the authd service.
// It uses the locally build authd nss module for the integration tests.
func getentOutputForLib(t *testing.T, socketPath string, env []string, shouldPreCheck bool, cmds ...string) (got string, exitCode int) {
	t.Helper()

	// #nosec:G204 - we control the command arguments in tests
	cmds = append(cmds, "--service", "authd")
	cmd := exec.Command("getent", cmds...)
	cmd.Env = env

	if socketPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_NSS_SOCKET=%s", socketPath))
	}

	if shouldPreCheck {
		cmd.Env = append(cmd.Env, "AUTHD_NSS_SHOULD_PRE_CHECK=1")
	}

	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = os.Stderr

	// We are only interested in the output and the exit code of the command, so we can ignore the error.
	_ = cmd.Run()

	return out.String(), cmd.ProcessState.ExitCode()
}
