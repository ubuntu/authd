package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
)

var daemonPath string

func TestCLIAuthenticate(t *testing.T) {
	t.Parallel()

	clientPath := t.TempDir()
	cliEnv := preparePamRunnerTest(t, clientPath)
	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK"

	tests := map[string]struct {
		tape         string
		tapeSettings []tapeSetting

		clientOptions      clientOptions
		currentUserNotRoot bool
	}{
		"Authenticate user successfully":                                       {tape: "simple_auth"},
		"Authenticate user successfully with preset user":                      {tape: "simple_auth_with_preset_user", clientOptions: clientOptions{PamUser: "user-integration-simple-preset"}},
		"Authenticate user with mfa":                                           {tape: "mfa_auth"},
		"Authenticate user with form mode with button":                         {tape: "form_with_button"},
		"Authenticate user with qr code":                                       {tape: "qr_code", clientOptions: clientOptions{PamUser: "user-integration-qr-code"}},
		"Authenticate user with qr code in a TTY":                              {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 650}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-tty", Term: "linux"}},
		"Authenticate user with qr code in a TTY session":                      {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 650}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-tty-session", Term: "xterm-256color", SessionType: "tty"}},
		"Authenticate user with qr code in screen":                             {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 650}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-screen", Term: "screen"}},
		"Authenticate user with qr code after many regenerations":              {tape: "qr_code_quick_regenerate", tapeSettings: []tapeSetting{{vhsHeight, 650}}},
		"Authenticate user and reset password while enforcing policy":          {tape: "mandatory_password_reset"},
		"Authenticate user with mfa and reset password while enforcing policy": {tape: "mfa_reset_pwquality_auth"},
		"Authenticate user and offer password reset":                           {tape: "optional_password_reset_skip"},
		"Authenticate user switching auth mode":                                {tape: "switch_auth_mode"},
		"Authenticate user switching username":                                 {tape: "switch_username"},
		"Authenticate user switching to local broker":                          {tape: "switch_local_broker"},
		"Authenticate user and add it to local group":                          {tape: "local_group"},
		"Authenticate with warnings on unsupported arguments":                  {tape: "simple_auth_with_unsupported_args"},

		"Remember last successful broker and mode":      {tape: "remember_broker_and_mode"},
		"Autoselect local broker for local user":        {tape: "local_user"},
		"Autoselect local broker for local user preset": {tape: "local_user_preset", clientOptions: clientOptions{PamUser: "root"}},

		"Prevent user from switching username": {tape: "switch_preset_username", clientOptions: clientOptions{PamUser: "user-integration-pam-preset"}},

		"Deny authentication if current user is not considered as root": {tape: "not_root", currentUserNotRoot: true},

		"Deny authentication if max attempts reached":                         {tape: "max_attempts"},
		"Deny authentication if user does not exist":                          {tape: "unexistent_user"},
		"Deny authentication if newpassword does not match required criteria": {tape: "bad_password"},

		"Exit authd if local broker is selected": {tape: "local_broker"},
		"Exit authd if user sigints":             {tape: "sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			outDir := t.TempDir()
			err := os.Symlink(filepath.Join(clientPath, "pam_authd"),
				filepath.Join(outDir, "pam_authd"))
			require.NoError(t, err, "Setup: symlinking the pam client")

			gpasswdOutput := filepath.Join(outDir, "gpasswd.output")
			groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
			socketPath := runAuthd(t, gpasswdOutput, groupsFile, !tc.currentUserNotRoot)

			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Env[socketPathEnv] = socketPath
			td.AddClientOptions(t, tc.clientOptions)
			td.RunVhs(t, "cli", outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			localgroupstestutils.RequireGPasswdOutput(t, gpasswdOutput, testutils.GoldenPath(t)+".gpasswd_out")
		})
	}
}

func TestCLIChangeAuthTok(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	cliEnv := preparePamRunnerTest(t, outDir)

	// we don't care about the output of gpasswd for this test, but we still need to mock it.
	err := os.MkdirAll(filepath.Join(outDir, "gpasswd"), 0700)
	require.NoError(t, err, "Setup: Could not create gpasswd output directory")
	gpasswdOutput := filepath.Join(outDir, "gpasswd", "chauthtok.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK"
	defaultSocketPath := runAuthd(t, gpasswdOutput, groupsFile, true)

	tests := map[string]struct {
		tape         string
		tapeSettings []tapeSetting

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
		"Exit authd if user sigints":             {tape: "passwd_sigint"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socketPath := defaultSocketPath
			if tc.currentUserNotRoot {
				socketPath = runAuthd(t, gpasswdOutput, groupsFile, false)
			}

			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Env[socketPathEnv] = socketPath
			td.AddClientOptions(t, clientOptions{})
			td.RunVhs(t, "cli", outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)
		})
	}
}

func TestPamCLIRunStandalone(t *testing.T) {
	t.Parallel()

	clientPath := t.TempDir()
	pamCleanup, err := buildPAMRunner(clientPath)
	require.NoError(t, err, "Setup: Failed to build PAM executable")
	t.Cleanup(pamCleanup)

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command("go", "run")
	if testutils.CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
		cmd.Env = testutils.AppendCovEnv(os.Environ())
	}
	if testutils.IsRace() {
		cmd.Args = append(cmd.Args, "-race")
	}

	cmd.Dir = testutils.ProjectRoot()
	cmd.Args = append(cmd.Args, "-tags", "withpamrunner",
		"./pam/tools/pam-runner",
		"login", "--exec-debug")
	cmd.Args = append(cmd.Args, "logfile="+os.Stdout.Name())

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Could not run PAM client: %s", out)
	outStr := string(out)
	t.Log(outStr)

	require.Contains(t, outStr, pam.ErrSystem.Error())
	require.Contains(t, outStr, pam.ErrIgnore.Error())
}

func TestMockgpasswd(t *testing.T) {
	localgroupstestutils.Mockgpasswd(t)
}
