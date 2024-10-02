package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestNativeAuthenticate(t *testing.T) {
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
		"Authenticate user with mfa":                                           {tape: "mfa_auth", tapeSettings: []tapeSetting{{vhsHeight, 700}}},
		"Authenticate user with form mode with button":                         {tape: "form_with_button"},
		"Authenticate user with qr code":                                       {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 2300}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code"}},
		"Authenticate user with qr code in a TTY":                              {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 3500}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-tty", Term: "linux"}},
		"Authenticate user with qr code in a TTY session":                      {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 3500}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-tty-session", Term: "xterm-256color", SessionType: "tty"}},
		"Authenticate user with qr code in screen":                             {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 3500}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-screen", Term: "screen"}},
		"Authenticate user with qr code in polkit":                             {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 3500}}, clientOptions: clientOptions{PamUser: "user-integration-qr-code-polkit", PamServiceName: "polkit-1"}},
		"Authenticate user with qr code in ssh":                                {tape: "qr_code", tapeSettings: []tapeSetting{{vhsHeight, 3500}}, clientOptions: clientOptions{PamUser: "user-integration-pre-check-ssh-service-qr-code", PamServiceName: "sshd"}},
		"Authenticate user and reset password while enforcing policy":          {tape: "mandatory_password_reset"},
		"Authenticate user with mfa and reset password while enforcing policy": {tape: "mfa_reset_pwquality_auth"},
		"Authenticate user and offer password reset":                           {tape: "optional_password_reset_skip"},
		"Authenticate user and accept password reset":                          {tape: "optional_password_reset_accept"},
		"Authenticate user switching auth mode":                                {tape: "switch_auth_mode", tapeSettings: []tapeSetting{{vhsHeight, 2350}}},
		"Authenticate user switching username":                                 {tape: "switch_username"},
		"Authenticate user switching to local broker":                          {tape: "switch_local_broker", tapeSettings: []tapeSetting{{vhsHeight, 600}}},
		"Authenticate user and add it to local group":                          {tape: "local_group"},
		"Authenticate user on ssh service":                                     {tape: "simple_ssh_auth", clientOptions: clientOptions{PamUser: "user-integration-pre-check-ssh-service", PamServiceName: "sshd"}},
		"Authenticate user on ssh service with custom name and connection env": {tape: "simple_ssh_auth", clientOptions: clientOptions{PamUser: "user-integration-pre-check-ssh-connection", PamEnv: []string{"SSH_CONNECTION=foo-connection"}}},
		"Authenticate user on ssh service with custom name and auth info env":  {tape: "simple_ssh_auth", clientOptions: clientOptions{PamUser: "user-integration-pre-check-ssh-auth-info", PamEnv: []string{"SSH_AUTH_INFO_0=foo-authinfo"}}},
		"Authenticate with warnings on unsupported arguments":                  {tape: "simple_auth_with_unsupported_args"},

		"Remember last successful broker and mode":      {tape: "remember_broker_and_mode", tapeSettings: []tapeSetting{{vhsHeight, 800}}},
		"Autoselect local broker for local user":        {tape: "local_user"},
		"Autoselect local broker for local user preset": {tape: "local_user_preset", clientOptions: clientOptions{PamUser: "root"}},

		"Deny authentication if current user is not considered as root": {tape: "not_root", currentUserNotRoot: true},

		"Deny authentication if max attempts reached":                         {tape: "max_attempts"},
		"Deny authentication if user does not exist":                          {tape: "unexistent_user"},
		"Deny authentication if user does not exist and matches cancel key":   {tape: "cancel_key_user"},
		"Deny authentication if newpassword does not match required criteria": {tape: "bad_password", tapeSettings: []tapeSetting{{vhsHeight, 550}}},

		"Prevent preset user from switching username": {tape: "switch_preset_username", tapeSettings: []tapeSetting{{vhsHeight, 700}}, clientOptions: clientOptions{PamUser: "user-integration-pam-preset"}},

		"Exit authd if local broker is selected":                                    {tape: "local_broker"},
		"Exit if user is not pre-checked on ssh service":                            {tape: "local_ssh", clientOptions: clientOptions{PamUser: "user-integration-ssh-service", PamServiceName: "sshd"}},
		"Exit if user is not pre-checked on custom ssh service with connection env": {tape: "local_ssh", clientOptions: clientOptions{PamUser: "user-integration-ssh-connection", PamEnv: []string{"SSH_CONNECTION=foo-connection"}}},
		"Exit if user is not pre-checked on custom ssh service with auth info env":  {tape: "local_ssh", clientOptions: clientOptions{PamUser: "user-integration-ssh-auth-info", PamEnv: []string{"SSH_AUTH_INFO_0=foo-authinfo"}}},
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

			gpasswdOutput := filepath.Join(outDir, "gpasswd.output")
			groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
			socketPath := runAuthd(t, gpasswdOutput, groupsFile, !tc.currentUserNotRoot)

			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Env[socketPathEnv] = socketPath
			td.Env[pam_test.RunnerEnvSupportsConversation] = "1"
			td.AddClientOptions(t, tc.clientOptions)
			td.RunVhs(t, "native", outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			localgroupstestutils.RequireGPasswdOutput(t, gpasswdOutput, testutils.GoldenPath(t)+".gpasswd_out")
		})
	}
}

func TestNativeChangeAuthTok(t *testing.T) {
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
		"Change passwd after MFA auth":                               {tape: "passwd_mfa", tapeSettings: []tapeSetting{{vhsHeight, 900}}},

		"Retry if new password is rejected by broker":           {tape: "passwd_rejected", tapeSettings: []tapeSetting{{vhsHeight, 700}}},
		"Retry if new password is same of previous":             {tape: "passwd_not_changed"},
		"Retry if password confirmation is not the same":        {tape: "passwd_not_confirmed"},
		"Retry if new password does not match quality criteria": {tape: "passwd_bad_password", tapeSettings: []tapeSetting{{vhsHeight, 550}}},

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

			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Env[socketPathEnv] = socketPath
			td.Env[pam_test.RunnerEnvSupportsConversation] = "1"
			td.AddClientOptions(t, clientOptions{})
			td.RunVhs(t, "native", outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)
		})
	}
}
