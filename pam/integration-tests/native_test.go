package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestNativeAuthenticate(t *testing.T) {
	t.Parallel()

	clientPath := t.TempDir()
	cliEnv := preparePamRunnerTest(t, clientPath)
	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK"
	tapeCommand := fmt.Sprintf("./pam_authd login socket=${%s} force_native_client=true",
		socketPathEnv)

	defaultGPasswdOutput, groupsFile := prepareGPasswdFiles(t)
	defaultSocketPath := runAuthd(t, defaultGPasswdOutput, groupsFile, true)

	tests := map[string]struct {
		tape          string
		tapeSettings  []tapeSetting
		tapeVariables map[string]string
		tapeCommand   string

		clientOptions      clientOptions
		currentUserNotRoot bool
		wantLocalGroups    bool
		skipRunnerCheck    bool
	}{
		"Authenticate user successfully": {
			tape:          "simple_auth",
			clientOptions: clientOptions{PamUser: "user-integration-simple-auth"},
		},
		"Authenticate user successfully with user selection": {
			tape: "simple_auth_with_user_selection",
		},
		"Authenticate user with mfa": {
			tape:          "mfa_auth",
			tapeSettings:  []tapeSetting{{vhsHeight, 1000}},
			clientOptions: clientOptions{PamUser: "user-mfa-integration-auth"},
		},
		"Authenticate user with form mode with button": {
			tape:          "form_with_button",
			tapeSettings:  []tapeSetting{{vhsHeight, 700}},
			clientOptions: clientOptions{PamUser: "user-integration-form-w-button"},
		},
		"Authenticate user with qr code": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 3000}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "7"},
			clientOptions: clientOptions{PamUser: "user-integration-qr-code"},
		},
		"Authenticate user with qr code in a TTY": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 4000}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "7"},
			clientOptions: clientOptions{
				PamUser: "user-integration-qr-code-tty",
				Term:    "linux",
			},
		},
		"Authenticate user with qr code in a TTY session": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 4000}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "7"},
			clientOptions: clientOptions{
				PamUser: "user-integration-qr-code-tty-session",
				Term:    "xterm-256color", SessionType: "tty",
			},
		},
		"Authenticate user with qr code in screen": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 4000}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "7"},
			clientOptions: clientOptions{
				PamUser: "user-integration-qr-code-screen",
				Term:    "screen",
			},
		},
		"Authenticate user with qr code in polkit": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 3500}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "2"},
			clientOptions: clientOptions{
				PamUser:        "user-integration-qr-code-polkit",
				PamServiceName: "polkit-1",
			},
		},
		"Authenticate user with qr code in ssh": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 3500}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "2"},
			clientOptions: clientOptions{
				PamUser:        "user-integration-pre-check-ssh-service-qr-code",
				PamServiceName: "sshd",
			},
		},
		"Authenticate user and reset password while enforcing policy": {
			tape:          "mandatory_password_reset",
			clientOptions: clientOptions{PamUser: "user-needs-reset-integration-mandatory"},
		},
		"Authenticate user with mfa and reset password while enforcing policy": {
			tape:          "mfa_reset_pwquality_auth",
			tapeSettings:  []tapeSetting{{vhsHeight, 1000}},
			clientOptions: clientOptions{PamUser: "user-mfa-with-reset-integration-pwquality"},
		},
		"Authenticate user and offer password reset": {
			tape:          "optional_password_reset_skip",
			clientOptions: clientOptions{PamUser: "user-can-reset-integration-skip"},
		},
		"Authenticate user and accept password reset": {
			tape:          "optional_password_reset_accept",
			clientOptions: clientOptions{PamUser: "user-can-reset-integration-accept"},
		},
		"Authenticate user switching auth mode": {
			tape:          "switch_auth_mode",
			tapeSettings:  []tapeSetting{{vhsHeight, 3000}},
			clientOptions: clientOptions{PamUser: "user-integration-switch-mode"},
			tapeVariables: map[string]string{"AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM": "6"},
		},
		"Authenticate user switching username": {
			tape: "switch_username",
		},
		"Authenticate user switching to local broker": {
			tape:          "switch_local_broker",
			tapeSettings:  []tapeSetting{{vhsHeight, 700}},
			clientOptions: clientOptions{PamUser: "user-integration-switch-broker"},
		},
		"Authenticate user and add it to local group": {
			tape:            "local_group",
			wantLocalGroups: true,
			clientOptions:   clientOptions{PamUser: "user-local-groups"},
		},
		"Authenticate user on ssh service": {
			tape: "simple_ssh_auth",
			clientOptions: clientOptions{
				PamUser:        "user-integration-pre-check-ssh-service",
				PamServiceName: "sshd",
			},
		},
		"Authenticate user on ssh service with custom name and connection env": {
			tape: "simple_ssh_auth",
			clientOptions: clientOptions{
				PamUser: "user-integration-pre-check-ssh-connection",
				PamEnv:  []string{"SSH_CONNECTION=foo-connection"},
			},
		},
		"Authenticate user on ssh service with custom name and auth info env": {
			tape: "simple_ssh_auth",
			clientOptions: clientOptions{
				PamUser: "user-integration-pre-check-ssh-auth-info",
				PamEnv:  []string{"SSH_AUTH_INFO_0=foo-authinfo"},
			},
		},
		"Authenticate with warnings on unsupported arguments": {
			tape: "simple_auth_with_unsupported_args",
			tapeCommand: strings.ReplaceAll(tapeCommand, "force_native_client=true",
				"invalid_flag=foo force_native_client=true bar"),
			clientOptions: clientOptions{PamUser: "user-integration-with-unsupported-args"},
		},

		"Remember last successful broker and mode": {
			tape:          "remember_broker_and_mode",
			tapeSettings:  []tapeSetting{{vhsHeight, 800}},
			clientOptions: clientOptions{PamUser: "user-integration-remember-mode"},
		},
		"Autoselect local broker for local user": {
			tape: "local_user",
		},
		"Autoselect local broker for local user preset": {
			tape:          "local_user_preset",
			clientOptions: clientOptions{PamUser: "root"},
		},

		"Deny authentication if current user is not considered as root": {
			tape: "not_root", currentUserNotRoot: true,
			clientOptions: clientOptions{PamUser: "user-integration-not-root"},
		},

		"Deny authentication if max attempts reached": {
			tape:          "max_attempts",
			tapeSettings:  []tapeSetting{{vhsHeight, 700}},
			clientOptions: clientOptions{PamUser: "user-integration-max-attempts"},
		},
		"Deny authentication if user does not exist": {
			tape:          "unexistent_user",
			clientOptions: clientOptions{PamUser: "user-unexistent"},
		},
		"Deny authentication if user does not exist and matches cancel key": {
			tape: "cancel_key_user",
		},
		"Deny authentication if newpassword does not match required criteria": {
			tape:          "bad_password",
			tapeSettings:  []tapeSetting{{vhsHeight, 800}},
			clientOptions: clientOptions{PamUser: "user-needs-reset-integration-bad-password"},
		},

		"Prevent preset user from switching username": {
			tape:          "switch_preset_username",
			tapeSettings:  []tapeSetting{{vhsHeight, 800}},
			clientOptions: clientOptions{PamUser: "user-integration-pam-preset"},
		},

		"Exit authd if local broker is selected": {
			tape:          "local_broker",
			clientOptions: clientOptions{PamUser: "user-local-broker"},
		},
		"Exit if user is not pre-checked on ssh service": {
			tape: "local_ssh",
			clientOptions: clientOptions{
				PamUser:        "user-integration-ssh-service",
				PamServiceName: "sshd",
			},
		},
		"Exit if user is not pre-checked on custom ssh service with connection env": {
			tape: "local_ssh",
			clientOptions: clientOptions{
				PamUser: "user-integration-ssh-connection",
				PamEnv:  []string{"SSH_CONNECTION=foo-connection"},
			},
		},
		"Exit if user is not pre-checked on custom ssh service with auth info env": {
			tape: "local_ssh",
			clientOptions: clientOptions{
				PamUser: "user-integration-ssh-auth-info",
				PamEnv:  []string{"SSH_AUTH_INFO_0=foo-authinfo"},
			},
		},
		// FIXME: While this works now, it requires proper handling via signal_fd
		"Exit authd if user sigints": {
			tape:            "sigint",
			clientOptions:   clientOptions{PamUser: "user-integration-sigint"},
			skipRunnerCheck: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			outDir := t.TempDir()
			err := os.Symlink(filepath.Join(clientPath, "pam_authd"),
				filepath.Join(outDir, "pam_authd"))
			require.NoError(t, err, "Setup: symlinking the pam client")

			socketPath := defaultSocketPath
			gpasswdOutput := defaultGPasswdOutput
			if tc.wantLocalGroups || tc.currentUserNotRoot {
				// For the local groups tests we need to run authd again so that it has
				// special environment that generates a fake gpasswd output for us to test.
				// Similarly for the not-root tests authd has to run in a more restricted way.
				// In the other cases this is not needed, so we can just use a shared authd.
				var groupsFile string
				gpasswdOutput, groupsFile = prepareGPasswdFiles(t)
				socketPath = runAuthd(t, gpasswdOutput, groupsFile, !tc.currentUserNotRoot)
			}

			if tc.tapeCommand == "" {
				tc.tapeCommand = tapeCommand
			}
			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Command = tc.tapeCommand
			td.CommandSleep = defaultSleepValues[authdSleepCommand] * 2
			td.Env[socketPathEnv] = socketPath
			td.Env[pam_test.RunnerEnvSupportsConversation] = "1"
			td.Variables = tc.tapeVariables
			td.AddClientOptions(t, tc.clientOptions)
			td.RunVhs(t, "native", outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			localgroupstestutils.RequireGPasswdOutput(t, gpasswdOutput, testutils.GoldenPath(t)+".gpasswd_out")

			if !tc.skipRunnerCheck {
				requireRunnerResultForUser(t, authd.SessionMode_AUTH, tc.clientOptions.PamUser, got)
			}
		})
	}
}

func TestNativeChangeAuthTok(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	cliEnv := preparePamRunnerTest(t, outDir)

	const socketPathEnv = "AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK"
	const tapeBaseCommand = "./pam_authd %s socket=${%s} force_native_client=true"
	tapeCommand := fmt.Sprintf(tapeBaseCommand, "passwd", socketPathEnv)
	defaultSocketPath := runAuthd(t, os.DevNull, os.DevNull, true)

	tests := map[string]struct {
		tape          string
		tapeSettings  []tapeSetting
		tapeVariables map[string]string

		currentUserNotRoot bool
		skipRunnerCheck    bool
	}{
		"Change password successfully and authenticate with new one": {
			tape: "passwd_simple",
			tapeVariables: map[string]string{
				"AUTHD_TEST_TAPE_LOGIN_COMMAND": fmt.Sprintf(tapeBaseCommand, "login", socketPathEnv),
			},
		},
		"Change passwd after MFA auth": {
			tape:         "passwd_mfa",
			tapeSettings: []tapeSetting{{vhsHeight, 1100}},
		},

		"Retry if new password is rejected by broker": {
			tape:         "passwd_rejected",
			tapeSettings: []tapeSetting{{vhsHeight, 700}},
		},
		"Retry if new password is same of previous": {
			tape: "passwd_not_changed",
		},
		"Retry if password confirmation is not the same": {
			tape: "passwd_not_confirmed",
		},
		"Retry if new password does not match quality criteria": {
			tape:         "passwd_bad_password",
			tapeSettings: []tapeSetting{{vhsHeight, 800}},
		},

		"Prevent change password if auth fails": {
			tape:         "passwd_auth_fail",
			tapeSettings: []tapeSetting{{vhsHeight, 700}},
		},
		"Prevent change password if user does not exist": {
			tape: "passwd_unexistent_user",
		},
		"Prevent change password if current user is not root as can't authenticate": {
			tape: "passwd_not_root", currentUserNotRoot: true,
		},

		"Exit authd if local broker is selected": {
			tape: "passwd_local_broker",
		},
		// FIXME: While this works now, it requires proper handling via signal_fd
		"Exit authd if user sigints": {
			tape:            "passwd_sigint",
			skipRunnerCheck: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socketPath := defaultSocketPath
			if tc.currentUserNotRoot {
				// For the not-root tests authd has to run in a more restricted way.
				// In the other cases this is not needed, so we can just use a shared authd.
				socketPath = runAuthd(t, os.DevNull, os.DevNull, false)
			}

			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Command = tapeCommand
			td.Variables = tc.tapeVariables
			td.Env[socketPathEnv] = socketPath
			td.Env[pam_test.RunnerEnvSupportsConversation] = "1"
			td.AddClientOptions(t, clientOptions{})
			td.RunVhs(t, "native", outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			if !tc.skipRunnerCheck {
				requireRunnerResult(t, authd.SessionMode_PASSWD, got)
			}
		})
	}
}
