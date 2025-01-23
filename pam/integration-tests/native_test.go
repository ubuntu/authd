package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/testutils/golden"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
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
		stopDaemonAfter    time.Duration
		skipRunnerCheck    bool
		socketPath         string
	}{
		"Authenticate_user_successfully": {
			tape:          "simple_auth",
			clientOptions: clientOptions{PamUser: "user-integration-simple-auth"},
		},
		"Authenticate_user_successfully_with_user_selection": {
			tape: "simple_auth_with_user_selection",
		},
		"Authenticate_user_successfully_with_invalid_connection_timeout": {
			tape: "simple_auth",
			clientOptions: clientOptions{
				PamUser:    "user-integration-simple-auth-invalid-timeout",
				PamTimeout: "invalid",
			},
		},
		"Authenticate_user_with_mfa": {
			tape:          "mfa_auth",
			tapeSettings:  []tapeSetting{{vhsHeight, 1200}},
			clientOptions: clientOptions{PamUser: "user-mfa-integration-auth"},
		},
		"Authenticate_user_with_form_mode_with_button": {
			tape:          "form_with_button",
			tapeSettings:  []tapeSetting{{vhsHeight, 700}},
			clientOptions: clientOptions{PamUser: "user-integration-form-w-button"},
		},
		"Authenticate_user_with_qr_code": {
			tape:         "qr_code",
			tapeSettings: []tapeSetting{{vhsHeight, 3000}},
			tapeVariables: map[string]string{
				"AUTHD_QRCODE_TAPE_ITEM":      "7",
				"AUTHD_QRCODE_TAPE_ITEM_NAME": "QR code",
			},
			clientOptions: clientOptions{PamUser: "user-integration-qr-code"},
		},
		"Authenticate_user_with_qr_code_in_a_TTY": {
			tape:         "qr_code",
			tapeSettings: []tapeSetting{{vhsHeight, 4000}},
			tapeVariables: map[string]string{
				"AUTHD_QRCODE_TAPE_ITEM":      "7",
				"AUTHD_QRCODE_TAPE_ITEM_NAME": "QR code",
			},
			clientOptions: clientOptions{
				PamUser: "user-integration-qr-code-tty",
				Term:    "linux",
			},
		},
		"Authenticate_user_with_qr_code_in_a_TTY_session": {
			tape:         "qr_code",
			tapeSettings: []tapeSetting{{vhsHeight, 4000}},
			tapeVariables: map[string]string{
				"AUTHD_QRCODE_TAPE_ITEM":      "7",
				"AUTHD_QRCODE_TAPE_ITEM_NAME": "QR code",
			},
			clientOptions: clientOptions{
				PamUser: "user-integration-qr-code-tty-session",
				Term:    "xterm-256color", SessionType: "tty",
			},
		},
		"Authenticate_user_with_qr_code_in_screen": {
			tape:         "qr_code",
			tapeSettings: []tapeSetting{{vhsHeight, 4000}},
			tapeVariables: map[string]string{
				"AUTHD_QRCODE_TAPE_ITEM":      "7",
				"AUTHD_QRCODE_TAPE_ITEM_NAME": "QR code",
			},
			clientOptions: clientOptions{
				PamUser: "user-integration-qr-code-screen",
				Term:    "screen",
			},
		},
		"Authenticate_user_with_qr_code_in_polkit": {
			tape:         "qr_code",
			tapeSettings: []tapeSetting{{vhsHeight, 3500}},
			tapeVariables: map[string]string{
				"AUTHD_QRCODE_TAPE_ITEM":      "2",
				"AUTHD_QRCODE_TAPE_ITEM_NAME": "Login code",
			},
			clientOptions: clientOptions{
				PamUser:        "user-integration-qr-code-polkit",
				PamServiceName: "polkit-1",
			},
		},
		"Authenticate_user_with_qr_code_in_ssh": {
			tape:         "qr_code",
			tapeSettings: []tapeSetting{{vhsHeight, 3500}},
			tapeVariables: map[string]string{
				"AUTHD_QRCODE_TAPE_ITEM":      "2",
				"AUTHD_QRCODE_TAPE_ITEM_NAME": "Login code",
			},
			clientOptions: clientOptions{
				PamUser:        "user-integration-pre-check-ssh-service-qr-code",
				PamServiceName: "sshd",
			},
		},
		"Authenticate_user_and_reset_password_while_enforcing_policy": {
			tape:          "mandatory_password_reset",
			clientOptions: clientOptions{PamUser: "user-needs-reset-integration-mandatory"},
		},
		"Authenticate_user_with_mfa_and_reset_password_while_enforcing_policy": {
			tape:          "mfa_reset_pwquality_auth",
			tapeSettings:  []tapeSetting{{vhsHeight, 3000}},
			clientOptions: clientOptions{PamUser: "user-mfa-with-reset-integration-pwquality"},
		},
		"Authenticate_user_and_offer_password_reset": {
			tape:          "optional_password_reset_skip",
			clientOptions: clientOptions{PamUser: "user-can-reset-integration-skip"},
		},
		"Authenticate_user_and_accept_password_reset": {
			tape:          "optional_password_reset_accept",
			clientOptions: clientOptions{PamUser: "user-can-reset-integration-accept"},
		},
		"Authenticate_user_switching_auth_mode": {
			tape:          "switch_auth_mode",
			tapeSettings:  []tapeSetting{{vhsHeight, 3000}},
			clientOptions: clientOptions{PamUser: "user-integration-switch-mode"},
			tapeVariables: map[string]string{
				"AUTHD_SWITCH_AUTH_MODE_TAPE_SEND_URL_TO_EMAIL_ITEM":   "2",
				"AUTHD_SWITCH_AUTH_MODE_TAPE_FIDO_DEVICE_FOO_ITEM":     "3",
				"AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_33_ITEM":            "4",
				"AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_1_ITEM":             "5",
				"AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM":            "6",
				"AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM":    "7",
				"AUTHD_SWITCH_AUTH_MODE_TAPE_AUTHENTICATION_CODE_ITEM": "8",

				"AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM_NAME": "QR code",
			},
		},
		"Authenticate_user_switching_username": {
			tape: "switch_username",
		},
		"Authenticate_user_switching_to_local_broker": {
			tape:          "switch_local_broker",
			tapeSettings:  []tapeSetting{{vhsHeight, 700}},
			clientOptions: clientOptions{PamUser: "user-integration-switch-broker"},
		},
		"Authenticate_user_and_add_it_to_local_group": {
			tape:            "local_group",
			tapeSettings:    []tapeSetting{{vhsHeight, 700}},
			wantLocalGroups: true,
			clientOptions:   clientOptions{PamUser: "user-local-groups"},
		},
		"Authenticate_user_on_ssh_service": {
			tape: "simple_ssh_auth",
			clientOptions: clientOptions{
				PamUser:        "user-integration-pre-check-ssh-service",
				PamServiceName: "sshd",
			},
		},
		"Authenticate_user_on_ssh_service_with_custom_name_and_connection_env": {
			tape: "simple_ssh_auth",
			clientOptions: clientOptions{
				PamUser: "user-integration-pre-check-ssh-connection",
				PamEnv:  []string{"SSH_CONNECTION=foo-connection"},
			},
		},
		"Authenticate_user_on_ssh_service_with_custom_name_and_auth_info_env": {
			tape: "simple_ssh_auth",
			clientOptions: clientOptions{
				PamUser: "user-integration-pre-check-ssh-auth-info",
				PamEnv:  []string{"SSH_AUTH_INFO_0=foo-authinfo"},
			},
		},
		"Authenticate_with_warnings_on_unsupported_arguments": {
			tape: "simple_auth_with_unsupported_args",
			tapeCommand: strings.ReplaceAll(tapeCommand, "force_native_client=true",
				"invalid_flag=foo force_native_client=true bar"),
			clientOptions: clientOptions{PamUser: "user-integration-with-unsupported-args"},
		},

		"Remember_last_successful_broker_and_mode": {
			tape:          "remember_broker_and_mode",
			tapeSettings:  []tapeSetting{{vhsHeight, 800}},
			clientOptions: clientOptions{PamUser: "user-integration-remember-mode"},
		},
		"Autoselect_local_broker_for_local_user": {
			tape: "local_user",
		},
		"Autoselect_local_broker_for_local_user_preset": {
			tape:          "local_user_preset",
			clientOptions: clientOptions{PamUser: "root"},
		},

		"Deny_authentication_if_current_user_is_not_considered_as_root": {
			tape: "not_root", currentUserNotRoot: true,
			clientOptions: clientOptions{PamUser: "user-integration-not-root"},
		},

		"Deny_authentication_if_max_attempts_reached": {
			tape:          "max_attempts",
			tapeSettings:  []tapeSetting{{vhsHeight, 700}},
			clientOptions: clientOptions{PamUser: "user-integration-max-attempts"},
		},
		"Deny_authentication_if_user_does_not_exist": {
			tape:          "unexistent_user",
			clientOptions: clientOptions{PamUser: "user-unexistent"},
		},
		"Deny_authentication_if_user_does_not_exist_and_matches_cancel_key": {
			tape: "cancel_key_user",
		},
		"Deny_authentication_if_newpassword_does_not_match_required_criteria": {
			tape:          "bad_password",
			tapeSettings:  []tapeSetting{{vhsHeight, 800}},
			clientOptions: clientOptions{PamUser: "user-needs-reset-integration-bad-password"},
		},

		"Prevent_preset_user_from_switching_username": {
			tape:          "switch_preset_username",
			tapeSettings:  []tapeSetting{{vhsHeight, 800}},
			clientOptions: clientOptions{PamUser: "user-integration-pam-preset"},
		},

		"Exit_authd_if_local_broker_is_selected": {
			tape:          "local_broker",
			clientOptions: clientOptions{PamUser: "user-local-broker"},
		},
		"Exit_if_user_is_not_pre-checked_on_ssh_service": {
			tape: "local_ssh",
			clientOptions: clientOptions{
				PamUser:        "user-integration-ssh-service",
				PamServiceName: "sshd",
			},
		},
		"Exit_if_user_is_not_pre-checked_on_custom_ssh_service_with_connection_env": {
			tape: "local_ssh",
			clientOptions: clientOptions{
				PamUser: "user-integration-ssh-connection",
				PamEnv:  []string{"SSH_CONNECTION=foo-connection"},
			},
		},
		"Exit_if_user_is_not_pre-checked_on_custom_ssh_service_with_auth_info_env": {
			tape: "local_ssh",
			clientOptions: clientOptions{
				PamUser: "user-integration-ssh-auth-info",
				PamEnv:  []string{"SSH_AUTH_INFO_0=foo-authinfo"},
			},
		},
		// FIXME: While this works now, it requires proper handling via signal_fd
		"Exit_authd_if_user_sigints": {
			tape:            "sigint",
			clientOptions:   clientOptions{PamUser: "user-integration-sigint"},
			skipRunnerCheck: true,
		},
		"Exit_if_authd_is_stopped": {
			tape:            "authd_stopped",
			clientOptions:   clientOptions{PamUser: "user-integration-authd-stopped"},
			stopDaemonAfter: sleepDuration(defaultSleepValues[authdSleepLong] * 5),
		},

		"Error_if_cannot_connect_to_authd": {
			tape:       "connection_error",
			socketPath: "/some-path/not-existent-socket",
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
			if tc.wantLocalGroups || tc.currentUserNotRoot || tc.stopDaemonAfter > 0 {
				// For the local groups tests we need to run authd again so that it has
				// special environment that generates a fake gpasswd output for us to test.
				// Similarly for the not-root tests authd has to run in a more restricted way.
				// In the other cases this is not needed, so we can just use a shared authd.
				var groupsFile string
				var cancel func()
				gpasswdOutput, groupsFile = prepareGPasswdFiles(t)
				socketPath, cancel = runAuthdWithCancel(t, gpasswdOutput, groupsFile, !tc.currentUserNotRoot)

				if tc.stopDaemonAfter > 0 {
					go func() {
						<-time.After(tc.stopDaemonAfter)
						t.Log("Stopping daemon!")
						cancel()
					}()
				}
			}
			if tc.socketPath != "" {
				socketPath = tc.socketPath
			}

			if tc.tapeCommand == "" {
				tc.tapeCommand = tapeCommand
			}
			td := newTapeData(tc.tape, tc.tapeSettings...)
			td.Command = tc.tapeCommand
			td.Env[socketPathEnv] = socketPath
			td.Env[pam_test.RunnerEnvSupportsConversation] = "1"
			td.Variables = tc.tapeVariables
			td.AddClientOptions(t, tc.clientOptions)
			td.RunVhs(t, vhsTestTypeNative, outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			golden.CheckOrUpdate(t, got)

			localgroupstestutils.RequireGPasswdOutput(t, gpasswdOutput, golden.Path(t)+".gpasswd_out")

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
	tapeCommand := fmt.Sprintf(tapeBaseCommand, pam_test.RunnerActionPasswd, socketPathEnv)
	defaultSocketPath := runAuthd(t, os.DevNull, os.DevNull, true)

	tests := map[string]struct {
		tape          string
		tapeSettings  []tapeSetting
		tapeVariables map[string]string

		currentUserNotRoot bool
		skipRunnerCheck    bool
	}{
		"Change_password_successfully_and_authenticate_with_new_one": {
			tape: "passwd_simple",
			tapeVariables: map[string]string{
				"AUTHD_TEST_TAPE_LOGIN_COMMAND": fmt.Sprintf(
					tapeBaseCommand, pam_test.RunnerActionLogin, socketPathEnv),
			},
		},
		"Change_passwd_after_MFA_auth": {
			tape:         "passwd_mfa",
			tapeSettings: []tapeSetting{{vhsHeight, 1300}},
		},

		"Retry_if_new_password_is_rejected_by_broker": {
			tape:         "passwd_rejected",
			tapeSettings: []tapeSetting{{vhsHeight, 1000}},
		},
		"Retry_if_new_password_is_same_of_previous": {
			tape: "passwd_not_changed",
		},
		"Retry_if_password_confirmation_is_not_the_same": {
			tape: "passwd_not_confirmed",
		},
		"Retry_if_new_password_does_not_match_quality_criteria": {
			tape:         "passwd_bad_password",
			tapeSettings: []tapeSetting{{vhsHeight, 800}},
		},

		"Prevent_change_password_if_auth_fails": {
			tape:         "passwd_auth_fail",
			tapeSettings: []tapeSetting{{vhsHeight, 700}},
		},
		"Prevent_change_password_if_user_does_not_exist": {
			tape: "passwd_unexistent_user",
		},
		"Prevent_change_password_if_current_user_is_not_root_as_can_not_authenticate": {
			tape: "passwd_not_root", currentUserNotRoot: true,
		},

		"Exit_authd_if_local_broker_is_selected": {
			tape: "passwd_local_broker",
		},
		// FIXME: While this works now, it requires proper handling via signal_fd
		"Exit_authd_if_user_sigints": {
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
			td.RunVhs(t, vhsTestTypeNative, outDir, cliEnv)
			got := td.ExpectedOutput(t, outDir)
			golden.CheckOrUpdate(t, got)

			if !tc.skipRunnerCheck {
				requireRunnerResult(t, authd.SessionMode_PASSWD, got)
			}
		})
	}
}
