package main_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

var daemonPath string

func TestCLIAuthenticate(t *testing.T) {
	t.Parallel()

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := prependBinToPath(t)

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	tests := map[string]struct {
		tape string

		currentUserNotRoot bool
		termEnv            string
		sessionEnv         string
	}{
		"Authenticate user successfully":                              {tape: "simple_auth"},
		"Authenticate user successfully with preset user":             {tape: "simple_auth_with_preset_user"},
		"Authenticate user with mfa":                                  {tape: "mfa_auth"},
		"Authenticate user with form mode with button":                {tape: "form_with_button"},
		"Authenticate user with qr code":                              {tape: "qr_code"},
		"Authenticate user with qr code in a TTY":                     {tape: "qr_code", termEnv: "linux"},
		"Authenticate user with qr code in a TTY session":             {tape: "qr_code", termEnv: "xterm-256color", sessionEnv: "tty"},
		"Authenticate user with qr code in screen":                    {tape: "qr_code", termEnv: "screen"},
		"Authenticate user and reset password while enforcing policy": {tape: "mandatory_password_reset"},
		"Authenticate user and offer password reset":                  {tape: "optional_password_reset_skip"},
		"Authenticate user switching auth mode":                       {tape: "switch_auth_mode"},
		"Authenticate user switching username":                        {tape: "switch_username"},
		"Authenticate user switching broker":                          {tape: "switch_broker"},
		"Authenticate user and add it to local group":                 {tape: "local_group"},
		"Authenticate with warnings on unsupported arguments":         {tape: "simple_auth_with_unsupported_args"},

		"Remember last successful broker and mode":      {tape: "remember_broker_and_mode"},
		"Autoselect local broker for local user":        {tape: "local_user"},
		"Autoselect local broker for local user preset": {tape: "local_user_preset"},

		"Deny authentication if current user is not considered as root": {tape: "not_root", currentUserNotRoot: true},

		"Deny authentication if max attempts reached":                         {tape: "max_attempts"},
		"Deny authentication if user does not exist":                          {tape: "unexistent_user"},
		"Deny authentication if usernames dont match":                         {tape: "mismatch_username"},
		"Deny authentication if newpassword does not match required criteria": {tape: "bad_password"},

		"Exit authd if local broker is selected": {tape: "local_broker"},
		"Exit authd if user sigints":             {tape: "sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			outDir := t.TempDir()

			cliEnv := prepareCLITest(t, outDir)
			cliLog := prepareCLILogging(t)
			t.Cleanup(func() {
				saveArtifactsForDebug(t, []string{
					filepath.Join(outDir, tc.tape+".gif"),
					filepath.Join(outDir, tc.tape+".txt"),
					cliLog,
				})
			})

			gpasswdOutput := filepath.Join(outDir, "gpasswd.output")
			groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
			socketPath := runAuthd(t, gpasswdOutput, groupsFile, !tc.currentUserNotRoot)

			const socketPathEnv = "AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK"
			// #nosec:G204 - we control the command arguments in tests
			cmd := exec.Command("env", "vhs", filepath.Join(currentDir, "testdata", "tapes", tc.tape+".tape"))
			cmd.Env = append(testutils.AppendCovEnv(cmd.Env), cliEnv...)
			cmd.Env = append(cmd.Env,
				pathEnv,
				fmt.Sprintf("%s=%s", socketPathEnv, socketPath),
				fmt.Sprintf("AUTHD_PAM_CLI_LOG_DIR=%s", filepath.Dir(cliLog)),
				fmt.Sprintf("AUTHD_PAM_CLI_TEST_NAME=%s", t.Name()))
			if tc.termEnv != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_PAM_CLI_TERM=%s", tc.termEnv))
			}
			if tc.sessionEnv != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_SESSION_TYPE=%s", tc.sessionEnv))
			}
			cmd.Dir = outDir

			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to run tape %q: %v: %s", tc.tape, err, out)

			tmp, err := os.ReadFile(filepath.Join(outDir, tc.tape+".txt"))
			require.NoError(t, err, "Could not read output file of tape %q", tc.tape)

			// We need to format the output a little bit, since the txt file can have some noise at the beginning.
			got := string(tmp)
			splitTmp := strings.Split(got, "\n")
			for i, str := range splitTmp {
				if strings.Contains(str, " ./pam_authd login socket=$") {
					got = strings.Join(splitTmp[i:], "\n")
					break
				}
			}
			got = permissionstestutils.IdempotentPermissionError(got)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			localgroupstestutils.RequireGPasswdOutput(t, gpasswdOutput, testutils.GoldenPath(t)+".gpasswd_out")
		})
	}
}

func TestCLIChangeAuthTok(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	cliEnv := prepareCLITest(t, outDir)

	// we don't care about the output of gpasswd for this test, but we still need to mock it.
	err := os.MkdirAll(filepath.Join(outDir, "gpasswd"), 0700)
	require.NoError(t, err, "Setup: Could not create gpasswd output directory")
	gpasswdOutput := filepath.Join(outDir, "gpasswd", "chauthtok.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK"
	defaultSocketPath := runAuthd(t, gpasswdOutput, groupsFile, true)

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := prependBinToPath(t)

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	tests := map[string]struct {
		tape string

		currentUserNotRoot bool
	}{
		"Change password successfully and authenticate with new one": {tape: "passwd_simple"},
		"Change passwd after MFA auth":                               {tape: "passwd_mfa"},

		"Retry if new password is rejected by broker":           {tape: "passwd_rejected"},
		"Retry if new password is same of previous":             {tape: "passwd_not_changed"},
		"Retry if password confirmation is not the same":        {tape: "passwd_not_confirmed"},
		"Retry if new password does not match quality criteria": {tape: "passwd_bad_password"},

		"Prevent change password if auth fails":                                     {tape: "passwd_auth_fail"},
		"Prevent change password if current user is not root as can't authenticate": {tape: "passwd_not_root", currentUserNotRoot: true},

		"Exit authd if local broker is selected": {tape: "passwd_local_broker"},
		"Exit authd if user sigints":             {tape: "passwd_sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socketPath := defaultSocketPath
			if tc.currentUserNotRoot {
				socketPath = runAuthd(t, gpasswdOutput, groupsFile, false)
			}

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
			cmd.Env = append(testutils.AppendCovEnv(cmd.Env), cliEnv...)
			cmd.Env = append(cmd.Env, pathEnv,
				fmt.Sprintf("%s=%s", socketPathEnv, socketPath),
				fmt.Sprintf("AUTHD_PAM_CLI_LOG_DIR=%s", filepath.Dir(cliLog)),
				fmt.Sprintf("AUTHD_PAM_CLI_TEST_NAME=%s", t.Name()))
			cmd.Dir = outDir

			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to run tape %q: %v: %s", tc.tape, err, out)

			tmp, err := os.ReadFile(filepath.Join(outDir, tc.tape+".txt"))
			require.NoError(t, err, "Could not read output file of tape %q", tc.tape)

			// We need to format the output a little bit, since the txt file can have some noise at the beginning.
			got := string(tmp)
			splitTmp := strings.Split(got, "\n")
			for i, str := range splitTmp {
				if strings.Contains(str, " ./pam_authd passwd socket=$") {
					got = strings.Join(splitTmp[i:], "\n")
					break
				}
			}
			got = permissionstestutils.IdempotentPermissionError(got)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)
		})
	}
}

func TestPamCLIRunStandalone(t *testing.T) {
	t.Parallel()

	clientPath := t.TempDir()
	pamCleanup, err := buildPAMTestClient(clientPath)
	require.NoError(t, err, "Setup: Failed to build PAM executable")
	t.Cleanup(pamCleanup)

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command("go", "run")
	if testutils.CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
		cmd.Env = testutils.AppendCovEnv(os.Environ())
	}

	cmd.Dir = testutils.ProjectRoot()
	cmd.Args = append(cmd.Args, "-tags", "pam_binary_cli", "./pam", "login", "--exec-debug")
	cmd.Args = append(cmd.Args, "logfile="+os.Stdout.Name())

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Could not run PAM client: %s", out)
	outStr := string(out)
	t.Log(outStr)

	require.Contains(t, outStr, pam.ErrSystem.Error())
	require.Contains(t, outStr, pam.ErrIgnore.Error())
}

func runAuthd(t *testing.T, gpasswdOutput, groupsFile string, currentUserAsRoot bool) string {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	env := localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, gpasswdOutput, groupsFile)
	if currentUserAsRoot {
		env = append(env, authdCurrentUserRootEnvVariableContent)
	}
	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath, testutils.WithEnvironment(env...))
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	return socketPath
}

func prepareCLITest(t *testing.T, clientPath string) []string {
	t.Helper()

	// Due to external dependencies such as `vhs`, we can't run the tests in some environments (like LP builders), as we
	// can't install the dependencies there. So we need to be able to skip these tests on-demand.
	if os.Getenv("AUTHD_SKIP_EXTERNAL_DEPENDENT_TESTS") != "" {
		t.Skip("Skipping tests with external dependencies as requested")
	}

	pamCleanup, err := buildPAMTestClient(clientPath)
	require.NoError(t, err, "Setup: Failed to build PAM executable")
	t.Cleanup(pamCleanup)

	return []string{
		fmt.Sprintf("AUTHD_PAM_EXEC_MODULE=%s", buildExecModule(t)),
		fmt.Sprintf("AUTHD_PAM_CLI_PATH=%s", buildPAMClient(t)),
	}
}

func prepareCLILogging(t *testing.T) string {
	t.Helper()

	return prepareFileLogging(t, "authd-pam-cli.log")
}

// buildPAMTestClient builds the PAM module in a temporary directory and returns a cleanup function.
func buildPAMTestClient(execPath string) (cleanup func(), err error) {
	cmd := exec.Command("go", "build")
	if testutils.CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	if pam_test.IsAddressSanitizerActive() {
		// -asan is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-asan")
	}
	cmd.Args = append(cmd.Args, "-tags=pam_binary_cli", "-o", filepath.Join(execPath, "pam_authd"), "../.")
	if out, err := cmd.CombinedOutput(); err != nil {
		return func() {}, fmt.Errorf("%v: %s", err, out)
	}

	return func() { _ = os.Remove(filepath.Join(execPath, "pam_authd")) }, nil
}

func buildPAMClient(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "build", "-C", "pam")
	cmd.Dir = testutils.ProjectRoot()
	if testutils.CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	if pam_test.IsAddressSanitizerActive() {
		// -asan is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-asan")
	}
	cmd.Args = append(cmd.Args, "-gcflags=-dwarflocationlists=true")
	cmd.Env = append(os.Environ(), `CGO_CFLAGS=-O0 -g3`)

	authdPam := filepath.Join(t.TempDir(), "authd-pam")
	t.Logf("Compiling Exec client at %s", authdPam)
	t.Logf(strings.Join(cmd.Args, " "))

	cmd.Args = append(cmd.Args, "-o", authdPam)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not compile PAM client: %s", out)

	return authdPam
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
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
