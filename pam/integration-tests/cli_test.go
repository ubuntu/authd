package main_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	grouptests "github.com/ubuntu/authd/internal/users/localgroups/tests"
)

var daemonPath string

func TestCLIAuthenticate(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	prepareCLITest(t, outDir)

	currentDir := testutils.CurrentDir()

	err := os.MkdirAll(filepath.Join(outDir, "gpasswd"), 0700)
	require.NoError(t, err, "Setup: Could not create gpasswd output directory")
	gpasswdOutput := filepath.Join(outDir, "gpasswd", "authenticate.output")
	groupsFile := filepath.Join(currentDir, testutils.TestFamilyPath(t), "gpasswd.group")

	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK"
	ctx, cancel := context.WithCancel(context.Background())
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...),
	)
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := prependBinToPath(t)

	tests := map[string]struct {
		tape string
	}{
		"Authenticate user successfully":                      {tape: "simple_auth"},
		"Authenticate user with mfa":                          {tape: "mfa_auth"},
		"Authenticate user with form mode with button":        {tape: "form_with_button"},
		"Authenticate user with qr code":                      {tape: "qr_code"},
		"Authenticate user and reset password":                {tape: "mandatory_password_reset"},
		"Authenticate user and offer password reset":          {tape: "optional_password_reset"},
		"Authenticate user switching auth mode":               {tape: "switch_auth_mode"},
		"Authenticate user switching username":                {tape: "switch_username"},
		"Authenticate user switching broker":                  {tape: "switch_broker"},
		"Authenticate user and add it to local group":         {tape: "local_group"},
		"Authenticate with warnings on unsupported arguments": {tape: "simple_auth_with_unsupported_args"},

		"Remember last successful broker and mode": {tape: "remember_broker_and_mode"},

		"Deny authentication if max attempts reached": {tape: "max_attempts"},
		"Deny authentication if user does not exist":  {tape: "unexistent_user"},

		"Exit authd if local broker is selected": {tape: "local_broker"},
		"Exit authd if user sigints":             {tape: "sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cliLog := prepareCLILogging(t)
			t.Cleanup(func() {
				saveArtifactsForDebug(t, []string{
					filepath.Join(outDir, tc.tape+".gif"),
					filepath.Join(outDir, tc.tape+".txt"),
					cliLog,
				})
			})

			// #nosec:G204 - we control the command arguments in tests
			cmd := exec.Command("env", "vhs", filepath.Join(currentDir, "testdata", "tapes", tc.tape+".tape"))
			cmd.Env = testutils.AppendCovEnv(cmd.Env)
			cmd.Env = append(cmd.Env, pathEnv)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", socketPathEnv, socketPath))
			cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_PAM_CLI_LOG_DIR=%s", filepath.Dir(cliLog)))
			cmd.Dir = outDir

			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to run tape %q: %v: %s", tc.tape, err, out)

			tmp, err := os.ReadFile(filepath.Join(outDir, tc.tape+".txt"))
			require.NoError(t, err, "Could not read output file of tape %q", tc.tape)

			// We need to format the output a little bit, since the txt file can have some noise at the beginning.
			var got string
			splitTmp := strings.Split(string(tmp), "\n")
			for i, str := range splitTmp {
				if strings.HasPrefix(str, fmt.Sprintf("> ./pam_authd login socket=${%s}", socketPathEnv)) {
					got = strings.Join(splitTmp[i:], "\n")
					break
				}
			}
			goldenDir := testutils.WithGoldenDir(currentDir)
			want := testutils.LoadWithUpdateFromGolden(t, got, goldenDir)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			if tc.tape == "local_group" {
				got := grouptests.IdempotentGPasswdOutput(t, gpasswdOutput)
				want := testutils.LoadWithUpdateFromGolden(t, got, goldenDir, testutils.WithGoldenPath(testutils.GoldenPath(t)+".gpasswd_out"))
				require.Equal(t, want, got, "UpdateLocalGroups should do the expected gpasswd operation, but did not")
			}
		})
	}
}

func TestCLIChangeAuthTok(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	prepareCLITest(t, outDir)

	currentDir := testutils.CurrentDir()

	// we don't care about the output of gpasswd for this test, but we still need to mock it.
	err := os.MkdirAll(filepath.Join(outDir, "gpasswd"), 0700)
	require.NoError(t, err, "Setup: Could not create gpasswd output directory")
	gpasswdOutput := filepath.Join(outDir, "gpasswd", "chauthtok.output")
	groupsFile := filepath.Join(currentDir, testutils.TestFamilyPath(t), "gpasswd.group")

	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK"
	ctx, cancel := context.WithCancel(context.Background())
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...),
	)
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := prependBinToPath(t)

	tests := map[string]struct {
		tape string
	}{
		"Change password successfully and authenticate with new one": {tape: "passwd_simple"},
		"Change passwd after MFA auth":                               {tape: "passwd_mfa"},

		"Retry if new password is rejected by broker":    {tape: "passwd_rejected"},
		"Retry if password confirmation is not the same": {tape: "passwd_not_confirmed"},

		"Prevent change password if auth fails": {"passwd_auth_fail"},

		"Exit authd if local broker is selected": {tape: "passwd_local_broker"},
		"Exit authd if user sigints":             {tape: "passwd_sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cliLog := prepareCLILogging(t)
			t.Cleanup(func() {
				saveArtifactsForDebug(t, []string{
					filepath.Join(outDir, tc.tape+".gif"),
					filepath.Join(outDir, tc.tape+".txt"),
					cliLog,
				})
			})

			// #nosec:G204 - we control the command arguments in tests
			cmd := exec.Command("env", "vhs", filepath.Join(currentDir, "testdata", "tapes", tc.tape+".tape"))
			cmd.Env = testutils.AppendCovEnv(cmd.Env)
			cmd.Env = append(cmd.Env, pathEnv)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", socketPathEnv, socketPath))
			cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_PAM_CLI_LOG_DIR=%s", filepath.Dir(cliLog)))
			cmd.Dir = outDir

			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to run tape %q: %v: %s", tc.tape, err, out)

			tmp, err := os.ReadFile(filepath.Join(outDir, tc.tape+".txt"))
			require.NoError(t, err, "Could not read output file of tape %q", tc.tape)

			// We need to format the output a little bit, since the txt file can have some noise at the beginning.
			var got string
			splitTmp := strings.Split(string(tmp), "\n")
			for i, str := range splitTmp {
				if strings.HasPrefix(str, fmt.Sprintf("> ./pam_authd passwd socket=${%s}", socketPathEnv)) {
					got = strings.Join(splitTmp[i:], "\n")
					break
				}
			}
			goldenDir := testutils.WithGoldenDir(currentDir)
			want := testutils.LoadWithUpdateFromGolden(t, got, goldenDir)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)
		})
	}
}

func prepareCLITest(t *testing.T, clientPath string) {
	t.Helper()

	// Due to external dependencies such as `vhs`, we can't run the tests in some environments (like LP builders), as we
	// can't install the dependencies there. So we need to be able to skip these tests on-demand.
	if os.Getenv("AUTHD_SKIP_EXTERNAL_DEPENDENT_TESTS") != "" {
		t.Skip("Skipping tests with external dependencies as requested")
	}

	pamCleanup, err := buildPAM(clientPath)
	require.NoError(t, err, "Setup: Failed to build PAM executable")
	t.Cleanup(pamCleanup)
}

func prepareCLILogging(t *testing.T) string {
	t.Helper()

	cliLog := filepath.Join(t.TempDir(), "authd-pam-cli.log")
	t.Cleanup(func() {
		out, err := os.ReadFile(cliLog)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		require.NoError(t, err, "Teardown: Impossible to read PAM client logs")
		t.Log(string(out))
	})

	return cliLog
}

// buildPAM builds the PAM module in a temporary directory and returns a cleanup function.
func buildPAM(execPath string) (cleanup func(), err error) {
	cmd := exec.Command("go", "build")
	cmd.Args = append(cmd.Args, "-C", "pam")
	cmd.Dir = testutils.ProjectRoot()
	if testutils.CoverDir() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	cmd.Args = append(cmd.Args, "-tags=pam_binary_cli", "-o", filepath.Join(execPath, "pam_authd"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return func() {}, fmt.Errorf("%v: %s", err, out)
	}

	return func() { _ = os.Remove(filepath.Join(execPath, "pam_authd")) }, nil
}

func TestMockgpasswd(t *testing.T) {
	grouptests.Mockgpasswd(t)
}

// prependBinToPath returns the value of the GOPATH defined in go env prepended to PATH.
func prependBinToPath(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "env", "GOPATH")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Could not get GOPATH: %v: %s", err, out)

	env := os.Getenv("PATH")
	return "PATH=" + strings.Join([]string{filepath.Join(strings.TrimSpace(string(out)), "bin"), env}, ":")
}

// saveArtifactsForDebug saves the specified artifacts to a temporary directory if the test failed.
func saveArtifactsForDebug(t *testing.T, artifacts []string) {
	t.Helper()
	if !t.Failed() {
		return
	}

	// We need to copy the artifacts to another directory, since the test directory will be cleaned up.
	artifactPath := os.Getenv("AUTHD_TEST_ARTIFACTS_PATH")
	if artifactPath == "" {
		artifactPath = filepath.Join(os.TempDir(), "authd-test-artifacts")
	}
	tmpDir := filepath.Join(artifactPath, testutils.GoldenPath(t))
	if err := os.MkdirAll(tmpDir, 0750); err != nil && !os.IsExist(err) {
		require.NoError(t, err, "Could not create temporary directory for artifacts")
		return
	}

	// Copy the artifacts to the temporary directory.
	for _, artifact := range artifacts {
		content, err := os.ReadFile(artifact)
		if err != nil {
			t.Logf("Could not read artifact %q: %v", artifact, err)
			continue
		}
		if err := os.WriteFile(filepath.Join(tmpDir, filepath.Base(artifact)), content, 0600); err != nil {
			t.Logf("Could not write artifact %q: %v", artifact, err)
		}
	}
}
