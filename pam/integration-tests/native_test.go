package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
)

func TestNativeAuthenticate(t *testing.T) {
	t.Parallel()

	clientPath := t.TempDir()
	cliEnv := prepareClientTest(t, clientPath)

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := prependBinToPath(t)

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	tests := map[string]struct {
		tape string

		currentUserNotRoot bool
		termEnv            string
		sessionEnv         string
		pamUser            string
		pamServiceName     string
	}{
		"Authenticate user successfully":                                       {tape: "simple_auth"},
		"Authenticate user successfully with preset user":                      {tape: "simple_auth_with_preset_user"},
		"Authenticate user with mfa":                                           {tape: "mfa_auth"},
		"Authenticate user with form mode with button":                         {tape: "form_with_button"},
		"Authenticate user with qr code":                                       {tape: "qr_code", pamUser: "user-integration-qr-code"},
		"Authenticate user with qr code in a TTY":                              {tape: "qr_code", pamUser: "user-integration-qr-code-tty", termEnv: "linux"},
		"Authenticate user with qr code in a TTY session":                      {tape: "qr_code", pamUser: "user-integration-qr-code-tty-session", termEnv: "xterm-256color", sessionEnv: "tty"},
		"Authenticate user with qr code in screen":                             {tape: "qr_code", pamUser: "user-integration-qr-code-screen", termEnv: "screen"},
		"Authenticate user with qr code in polkit":                             {tape: "qr_code", pamUser: "user-integration-qr-code-screen", pamServiceName: "polkit-1"},
		"Authenticate user and reset password while enforcing policy":          {tape: "mandatory_password_reset"},
		"Authenticate user with mfa and reset password while enforcing policy": {tape: "mfa_reset_pwquality_auth"},
		"Authenticate user and offer password reset":                           {tape: "optional_password_reset_skip"},
		"Authenticate user and accept password reset":                          {tape: "optional_password_reset_accept"},
		"Authenticate user switching auth mode":                                {tape: "switch_auth_mode"},
		"Authenticate user switching username":                                 {tape: "switch_username"},
		"Authenticate user switching to local broker":                          {tape: "switch_local_broker"},
		"Authenticate user and add it to local group":                          {tape: "local_group"},
		"Authenticate with warnings on unsupported arguments":                  {tape: "simple_auth_with_unsupported_args"},

		"Remember last successful broker and mode":      {tape: "remember_broker_and_mode"},
		"Autoselect local broker for local user":        {tape: "local_user"},
		"Autoselect local broker for local user preset": {tape: "local_user_preset"},

		"Deny authentication if current user is not considered as root": {tape: "not_root", currentUserNotRoot: true},

		"Deny authentication if max attempts reached":                         {tape: "max_attempts"},
		"Deny authentication if user does not exist":                          {tape: "unexistent_user"},
		"Deny authentication if usernames dont match":                         {tape: "mismatch_username"},
		"Deny authentication if newpassword does not match required criteria": {tape: "bad_password"},

		"Exit authd if local broker is selected": {tape: "local_broker"},
		// FIXME: While this works now, it requires proper handling via signal_fd
		"Exit authd if user sigints": {tape: "sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			outDir := t.TempDir()
			err := os.Symlink(filepath.Join(clientPath, "pam_authd"),
				filepath.Join(outDir, "pam_authd"))
			require.NoError(t, err, "Setup: symlinking the pam client")

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
			cmd := exec.Command("env", "vhs", filepath.Join(currentDir, "testdata", "tapes", "native", tc.tape+".tape"))
			cmd.Env = append(testutils.AppendCovEnv(cmd.Env), cliEnv...)
			cmd.Env = append(cmd.Env,
				pathEnv,
				fmt.Sprintf("%s=%s", socketPathEnv, socketPath),
				fmt.Sprintf("AUTHD_PAM_CLI_LOG_DIR=%s", filepath.Dir(cliLog)),
				fmt.Sprintf("AUTHD_PAM_CLI_TEST_NAME=%s", t.Name()),
				"AUTHD_PAM_CLI_SUPPORTS_CONVERSATION=1",
			)
			if tc.pamUser != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_PAM_CLI_USER=%s", tc.pamUser))
			}
			if tc.pamServiceName != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("AUTHD_PAM_CLI_SERVICE=%s", tc.pamServiceName))
			}
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

func TestNativeChangeAuthTok(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	cliEnv := prepareClientTest(t, outDir)

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
		"Prevent change password if user does not exist":                            {tape: "passwd_unexistent_user"},
		"Prevent change password if current user is not root as can't authenticate": {tape: "passwd_not_root", currentUserNotRoot: true},

		"Exit authd if local broker is selected": {tape: "passwd_local_broker"},
		// FIXME: While this works now, it requires proper handling via signal_fd
		"Exit authd if user sigints": {tape: "passwd_sigint"},
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
			cmd := exec.Command("env", "vhs", filepath.Join(currentDir, "testdata", "tapes", "native", tc.tape+".tape"))
			cmd.Env = append(testutils.AppendCovEnv(cmd.Env), cliEnv...)
			cmd.Env = append(cmd.Env, pathEnv,
				fmt.Sprintf("%s=%s", socketPathEnv, socketPath),
				fmt.Sprintf("AUTHD_PAM_CLI_LOG_DIR=%s", filepath.Dir(cliLog)),
				fmt.Sprintf("AUTHD_PAM_CLI_TEST_NAME=%s", t.Name()),
				"AUTHD_PAM_CLI_SUPPORTS_CONVERSATION=1",
			)
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
