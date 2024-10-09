package main_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestSSHAuthenticate(t *testing.T) {
	t.Parallel()

	// Due to external dependencies such as `vhs`, we can't run the tests in some environments (like LP builders), as we
	// can't install the dependencies there. So we need to be able to skip these tests on-demand.
	if os.Getenv("AUTHD_SKIP_EXTERNAL_DEPENDENT_TESTS") != "" {
		t.Skip("Skipping tests with external dependencies as requested")
	}

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	execModule := buildExecModuleWithCFlags(t, nil, true)
	execChild := buildPAMExecChild(t)
	sshdPreloadLibrary := buildCModule(t, []string{
		filepath.Join(currentDir, "/sshd_preloader/sshd_preloader.c"),
	}, nil, nil, nil, "sshd_preloader", true)

	sshdHostKey := filepath.Join(t.TempDir(), "ssh_host_ed25519_key")
	//#nosec:G204 - we control the command arguments in tests
	out, err := exec.Command("ssh-keygen", "-q", "-f", sshdHostKey, "-N", "", "-t", "ed25519").CombinedOutput()
	require.NoError(t, err, "Setup: Failed generating SSH host key: %s", out)
	saveArtifactsForDebugOnCleanup(t, []string{sshdHostKey})

	pubKey, err := os.ReadFile(sshdHostKey + ".pub")
	require.NoError(t, err, "Setup: Can't read sshd host public key")
	saveArtifactsForDebugOnCleanup(t, []string{sshdHostKey + ".pub"})

	defaultGPasswdOutput, groupsFile := prepareGPasswdFiles(t)
	defaultSocketPath := runAuthd(t, defaultGPasswdOutput, groupsFile, true)

	const tapeCommand = "ssh ${AUTHD_PAM_SSH_USER}@localhost ${AUTHD_PAM_SSH_ARGS}"
	defaultTapeSettings := []tapeSetting{{vhsHeight, 1000}, {vhsWidth, 800}}

	tests := map[string]struct {
		tape          string
		tapeSettings  []tapeSetting
		tapeVariables map[string]string

		user             string
		pamServiceName   string
		interactiveShell bool

		wantNotLoggedInUser bool
		wantLocalGroups     bool
	}{
		"Authenticate user successfully": {
			tape: "simple_auth",
		},
		"Authenticate user successfully and enters shell": {
			tape:             "simple_auth_with_shell",
			interactiveShell: true,
		},
		"Authenticate user with mfa": {
			tape:         "mfa_auth",
			tapeSettings: []tapeSetting{{vhsHeight, 1200}},
			user:         "user-mfa",
		},
		"Authenticate user with form mode with button": {
			tape: "form_with_button",
		},
		"Authenticate user with qr code": {
			tape:          "qr_code",
			tapeSettings:  []tapeSetting{{vhsHeight, 1500}},
			tapeVariables: map[string]string{"AUTHD_QRCODE_TAPE_ITEM": "2"},
		},
		"Authenticate user and reset password while enforcing policy": {
			tape: "mandatory_password_reset",
			user: "user-needs-reset",
		},
		"Authenticate user with mfa and reset password while enforcing policy": {
			tape:         "mfa_reset_pwquality_auth",
			user:         "user-mfa-with-reset",
			tapeSettings: []tapeSetting{{vhsHeight, 1500}},
		},
		"Authenticate user and offer password reset": {
			tape: "optional_password_reset_skip",
			user: "user-can-reset",
		},
		"Authenticate user and accept password reset": {
			tape: "optional_password_reset_accept",
			user: "user-can-reset2",
		},
		"Authenticate user switching auth mode": {
			tape:         "switch_auth_mode",
			tapeSettings: []tapeSetting{{vhsHeight, 3000}},
		},
		"Authenticate user switching to local broker": {
			tape:                "switch_local_broker",
			wantNotLoggedInUser: true,
		},
		"Authenticate user and add it to local group": {
			tape:            "local_group",
			user:            "user-local-groups",
			wantLocalGroups: true,
		},

		"Autoselect local broker for local user": {
			tape:                "local_user_preset",
			user:                "root",
			wantNotLoggedInUser: true,
			tapeSettings:        []tapeSetting{{vhsHeight, 200}},
		},

		"Deny authentication if max attempts reached": {
			tape:                "max_attempts",
			wantNotLoggedInUser: true,
		},
		"Deny authentication if user does not exist": {
			tape:                "unexistent_user",
			user:                "user-unexistent",
			wantNotLoggedInUser: true,
		},
		"Deny authentication if user does not exist and matches cancel key": {
			tape:                "cancel_key_user",
			user:                "r",
			wantNotLoggedInUser: true,
		},
		"Deny authentication if newpassword does not match required criteria": {
			tape: "bad_password",
			user: "user-needs-reset2",
		},

		"Prevent user from switching username": {
			tape: "switch_preset_username",
		},

		"Exit authd if local broker is selected": {
			tape:                "local_broker",
			wantNotLoggedInUser: true,
		},
		"Exit if user is not pre-checked on ssh service": {
			tape:                "local_ssh",
			user:                "user-integration-ssh-service",
			pamServiceName:      "sshd",
			wantNotLoggedInUser: true,
		},
		"Exit authd if user sigints": {
			tape:                "sigint",
			wantNotLoggedInUser: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socketPath := defaultSocketPath
			gpasswdOutput := defaultGPasswdOutput
			if tc.wantLocalGroups {
				// For the local groups tests we need to run authd again so that it has
				// special environment that generates a fake gpasswd output for us to test.
				// In the other cases this is not needed, so we can just use a shared authd.
				var groupsFile string
				gpasswdOutput, groupsFile = prepareGPasswdFiles(t)
				socketPath = runAuthd(t, gpasswdOutput, groupsFile, true)
			}

			user := tc.user
			if user == "" {
				user = "user-integration-pre-check-" + strings.ReplaceAll(
					strings.ToLower(filepath.Base(t.Name())), "_", "-")
			}

			sshdConnectCommand := fmt.Sprintf(
				"/usr/bin/echo ' SSHD: Connected to ssh via authd module! [%s]'",
				t.Name())
			if tc.interactiveShell {
				sshdConnectCommand = "/bin/sh"
			}

			serviceFile := createSshdServiceFile(t, execModule, execChild, socketPath)
			userHome := t.TempDir()
			sshdPort := startSSHd(t, sshdHostKey, sshdConnectCommand, []string{
				fmt.Sprintf("HOME=%s", userHome),
				fmt.Sprintf("LD_PRELOAD=%s", sshdPreloadLibrary),
				fmt.Sprintf("AUTHD_TEST_SSH_USER=%s", user),
				fmt.Sprintf("AUTHD_TEST_SSH_HOME=%s", userHome),
				fmt.Sprintf("AUTHD_TEST_SSH_PAM_SERVICE=%s", serviceFile),
			})

			knownHost := filepath.Join(t.TempDir(), "known_hosts")
			err := os.WriteFile(knownHost, []byte(
				fmt.Sprintf("[localhost]:%s %s", sshdPort, pubKey),
			), 0600)
			require.NoError(t, err, "Setup: can't create known hosts file")

			outDir := t.TempDir()
			td := newTapeData(tc.tape, append(defaultTapeSettings, tc.tapeSettings...)...)
			td.Command = tapeCommand
			td.CommandSleep = defaultSleepValues[authdSleepLong]
			td.Env[pam_test.RunnerEnvSupportsConversation] = "1"
			td.Env["HOME"] = userHome
			td.Env["AUTHD_PAM_SSH_USER"] = user
			td.Env["AUTHD_PAM_SSH_ARGS"] = strings.Join([]string{
				"-p", sshdPort,
				"-F", os.DevNull,
				"-i", os.DevNull,
				"-o", "ServerAliveInterval=300",
				"-o", "PasswordAuthentication=no",
				"-o", "PubkeyAuthentication=no",
				"-o", "UserKnownHostsFile=" + knownHost,
			}, " ")
			td.Variables = tc.tapeVariables
			td.RunVhs(t, "ssh", outDir, nil)
			got := sanitizeGoldenFile(t, td, outDir)
			want := testutils.LoadWithUpdateFromGolden(t, got)

			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)
			userEnv := fmt.Sprintf("USER=%s", user)
			if tc.wantNotLoggedInUser {
				require.NotContains(t, got, userEnv, "Should not have a logged in user")
			} else {
				require.Contains(t, got, userEnv, "Logged in user does not matches")
			}

			localgroupstestutils.RequireGPasswdOutput(t, gpasswdOutput, testutils.GoldenPath(t)+".gpasswd_out")
		})
	}
}

func sanitizeGoldenFile(t *testing.T, td tapeData, outDir string) string {
	t.Helper()

	golden := td.ExpectedOutput(t, outDir)

	// When sshd is in debug mode, it shows the environment variables, so let's sanitize them
	golden = regexp.MustCompile(`(?m)  (PATH|HOME|SSH_[A-Z]+)=.*(\n*)($[^ ]{2}.*)?$`).ReplaceAllString(
		golden, "  $1=$${AUTHD_TEST_$1}")
	return golden
}

func createSshdServiceFile(t *testing.T, module, execChild, socketPath string) string {
	t.Helper()

	moduleArgs := []string{
		execChild,
		"socket=" + socketPath,
		"debug=true",
		"logfile=" + os.Stderr.Name(),
		"--exec-debug",
	}

	if env := testutils.CoverDirEnv(); env != "" {
		moduleArgs = append(moduleArgs, "--exec-env", env)
	}
	if testutils.IsAsan() {
		if o := os.Getenv("ASAN_OPTIONS"); o != "" {
			moduleArgs = append(moduleArgs, "--exec-env",
				fmt.Sprintf("ASAN_OPTIONS=%s", o))
		}
		if o := os.Getenv("LSAN_OPTIONS"); o != "" {
			moduleArgs = append(moduleArgs, "--exec-env",
				fmt.Sprintf("LSAN_OPTIONS=%s", o))
		}
	}

	outDir := t.TempDir()
	pamServiceName := "authd-sshd"
	serviceFile, err := pam_test.CreateService(outDir, pamServiceName, []pam_test.ServiceLine{
		{Action: pam_test.Auth, Control: pam_test.SufficientRequisite, Module: module, Args: moduleArgs},
		{Action: pam_test.Auth, Control: pam_test.Optional, Module: "pam_echo.so", Args: []string{"SSH PAM user '%u' using local broker"}},
		{Action: pam_test.Include, Module: "common-auth"},
		{Action: pam_test.Account, Control: pam_test.SufficientRequisite, Module: module, Args: moduleArgs},
		{Action: pam_test.Session, Control: pam_test.Requisite, Module: pam_test.Permit.String()},
	})
	require.NoError(t, err, "Setup: Creation of service file %s", pamServiceName)
	saveArtifactsForDebugOnCleanup(t, []string{serviceFile})

	return serviceFile
}

func sshdCommand(t *testing.T, port, hostKey, forcedCommand string, env []string) *exec.Cmd {
	t.Helper()

	// #nosec:G204 - we control the command arguments in tests
	sshd := exec.Command("/usr/sbin/sshd",
		"-ddd",
		"-f", os.DevNull,
		"-p", port,
		"-h", hostKey,
		"-o", "UsePAM=yes",
		"-o", "KbdInteractiveAuthentication=yes",
		"-o", "AuthenticationMethods=keyboard-interactive",
		"-o", "IgnoreUserKnownHosts=yes",
		"-o", "AuthorizedKeysFile=none",
		"-o", "PermitUserEnvironment=no",
		"-o", "PermitUserRC=no",
		"-o", "ClientAliveInterval=300",
		"-o", "ClientAliveCountMax=3",
		"-o", "ForceCommand="+forcedCommand,
	)
	sshd.Env = append(sshd.Env, env...)
	sshd.Env = testutils.AppendCovEnv(sshd.Env)

	return sshd
}

// safeBuffer is used to buffer the sshd output, since we may read this from
// cleanup sub-functions (that run as different goroutines than the command's exec)
// we need to use locked read/writes on bytes.Buffer to avoid read/write races when
// running the tests in race mode.
// We only override the methods we require in the tests.
type safeBuffer struct {
	bytes.Buffer
	mu sync.RWMutex
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	return sb.Buffer.Write(p)
}

func (sb *safeBuffer) ReadFrom(r io.Reader) (n int64, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	return sb.Buffer.ReadFrom(r)
}

func (sb *safeBuffer) String() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	return sb.Buffer.String()
}

func startSSHd(t *testing.T, hostKey, forcedCommand string, env []string) string {
	t.Helper()

	// We use this to easily find a free port we can use, without going random
	server := httptest.NewServer(http.HandlerFunc(nil))
	url, err := url.Parse(server.URL)
	require.NoError(t, err, "Setup: Impossible to find a valid port to use")
	sshdPort := url.Port()
	server.Close()

	sshd := sshdCommand(t, sshdPort, hostKey, forcedCommand, env)
	sshdStderr := safeBuffer{}
	sshd.Stderr = &sshdStderr
	if testing.Verbose() {
		sshd.Stdout = os.Stdout
		sshd.Stderr = os.Stderr
	}

	t.Log("Launching sshd with", sshd.Env, sshd.Args)
	err = sshd.Start()
	require.NoError(t, err, "Setup: Impossible to start sshd")

	t.Cleanup(func() {
		if testing.Verbose() || !t.Failed() {
			return
		}
		sshdLog := filepath.Join(t.TempDir(), "sshd.log")
		require.NoError(t, os.WriteFile(sshdLog, []byte(sshdStderr.String()), 0600),
			"TearDown: Saving sshd log")
		saveArtifactsForDebug(t, []string{sshdLog})
	})

	t.Cleanup(func() {
		if sshd.Process == nil {
			return
		}

		sshdExited := make(chan *os.ProcessState)
		go func() {
			processState, err := sshd.Process.Wait()
			require.NoError(t, err, "TearDown: Waiting SSHd failed")
			sshdExited <- processState
		}()

		t.Log("Waiting for sshd to be terminated")
		select {
		case <-time.After(sleepDuration(5 * time.Second)):
			require.NoError(t, sshd.Process.Kill(), "TearDown: Killing SSHd failed")
			if !testing.Verbose() {
				t.Logf("SSHd stopped (killed)\n ##### STDERR #####\n %s \n ##### END #####",
					sshdStderr.String())
			}
			t.Fatal("SSHd didn't finish in time!")
		case state := <-sshdExited:
			if !testing.Verbose() {
				t.Logf("SSHd stopped (%s)\n ##### STDERR #####\n %s \n ##### END #####",
					state, sshdStderr.String())
			}
			require.Equal(t, 255, state.ExitCode(), "TearDown: SSHd exited with %s", state)
		}
	})

	// Sadly we can't wait for SSHd to be ready using net.Dial, since that will make sshd
	// (when in debug mode) not to accept further connections from the actual test, but we
	// can assume we're good.
	t.Logf("SSHd started with pid %d and listening on port %s", sshd.Process.Pid, sshdPort)
	return sshdPort
}
