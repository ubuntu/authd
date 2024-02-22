package main_test

import (
	"context"
	"flag"
	"fmt"
	"log"
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

	outDir := filepath.Dir(daemonPath)

	err := os.MkdirAll(filepath.Join(outDir, "gpasswd"), 0700)
	require.NoError(t, err, "Setup: Could not create gpasswd output directory")
	gpasswdOutput := filepath.Join(outDir, "gpasswd", "authenticate.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	socketPath := "/tmp/pam-cli-authenticate-tests.sock"
	ctx, cancel := context.WithCancel(context.Background())
	_, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithSocketPath(socketPath),
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...),
	)
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := appendGoBinToPath(t)

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

	tests := map[string]struct {
		tape string
	}{
		"Authenticate user successfully":               {tape: "simple_auth"},
		"Authenticate user with mfa":                   {tape: "mfa_auth"},
		"Authenticate user with form mode with button": {tape: "form_with_button"},
		"Authenticate user with qr code":               {tape: "qr_code"},
		"Authenticate user and reset password":         {tape: "mandatory_password_reset"},
		"Authenticate user and offer password reset":   {tape: "optional_password_reset"},
		"Authenticate user switching auth mode":        {tape: "switch_auth_mode"},
		"Authenticate user switching username":         {tape: "switch_username"},
		"Authenticate user switching broker":           {tape: "switch_broker"},
		"Authenticate user and add it to local group":  {tape: "local_group"},

		"Remember last successful broker and mode": {tape: "remember_broker_and_mode"},

		"Deny authentication if max attempts reached": {tape: "max_attempts"},
		"Deny authentication if user does not exist":  {tape: "unexistent_user"},

		"Exit authd if local broker is selected": {tape: "local_broker"},
		"Exit authd if user sigints":             {tape: "sigint"},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer saveArtifactsForDebug(t, []string{filepath.Join(outDir, tc.tape+".gif"), filepath.Join(outDir, tc.tape+".txt")})

			// #nosec:G204 - we control the command arguments in tests
			cmd := exec.Command("vhs", filepath.Join(currentDir, "testdata", "tapes", tc.tape+".tape"))
			cmd.Env = testutils.AppendCovEnv(cmd.Env)
			cmd.Env = append(cmd.Env, pathEnv)
			cmd.Dir = outDir

			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to run tape %q: %v: %s", tc.tape, err, out)

			tmp, err := os.ReadFile(filepath.Join(outDir, tc.tape+".txt"))
			require.NoError(t, err, "Could not read output file of tape %q", tc.tape)

			// We need to format the output a little bit, since the txt file can have some noise at the beginning.
			var got string
			splitTmp := strings.Split(string(tmp), "\n")
			for i, str := range splitTmp {
				if strings.HasPrefix(str, fmt.Sprintf("> ./pam_authd login socket=%s", socketPath)) {
					got = strings.Join(splitTmp[i:], "\n")
					break
				}
			}
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)

			if tc.tape == "local_group" {
				got := grouptests.IdempotentGPasswdOutput(t, gpasswdOutput)
				want := testutils.LoadWithUpdateFromGolden(t, got, testutils.WithGoldenPath(testutils.GoldenPath(t)+".gpasswd_out"))
				require.Equal(t, want, got, "UpdateLocalGroups should do the expected gpasswd operation, but did not")
			}
		})
	}
}

func TestCLIChangeAuthTok(t *testing.T) {
	t.Parallel()

	outDir := filepath.Dir(daemonPath)

	// we don't care about the output of gpasswd for this test, but we still need to mock it.
	err := os.MkdirAll(filepath.Join(outDir, "gpasswd"), 0700)
	require.NoError(t, err, "Setup: Could not create gpasswd output directory")
	gpasswdOutput := filepath.Join(outDir, "gpasswd", "chauthtok.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	socketPath := "/tmp/pam-cli-chauthtok-tests.sock"
	ctx, cancel := context.WithCancel(context.Background())
	_, stopped := testutils.RunDaemon(ctx, t, daemonPath,
		testutils.WithSocketPath(socketPath),
		testutils.WithEnvironment(grouptests.GPasswdMockEnv(t, gpasswdOutput, groupsFile)...),
	)
	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	// If vhs is installed with "go install", we need to add GOPATH to PATH.
	pathEnv := appendGoBinToPath(t)

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Setup: Could not get current directory for the tests")

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
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer saveArtifactsForDebug(t, []string{filepath.Join(outDir, tc.tape+".gif"), filepath.Join(outDir, tc.tape+".txt")})

			// #nosec:G204 - we control the command arguments in tests
			cmd := exec.Command("vhs", filepath.Join(currentDir, "testdata", "tapes", tc.tape+".tape"))
			cmd.Env = testutils.AppendCovEnv(cmd.Env)
			cmd.Env = append(cmd.Env, pathEnv)
			cmd.Dir = outDir

			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "Failed to run tape %q: %v: %s", tc.tape, err, out)

			tmp, err := os.ReadFile(filepath.Join(outDir, tc.tape+".txt"))
			require.NoError(t, err, "Could not read output file of tape %q", tc.tape)

			// We need to format the output a little bit, since the txt file can have some noise at the beginning.
			var got string
			splitTmp := strings.Split(string(tmp), "\n")
			for i, str := range splitTmp {
				if strings.HasPrefix(str, fmt.Sprintf("> ./pam_authd passwd socket=%s", socketPath)) {
					got = strings.Join(splitTmp[i:], "\n")
					break
				}
			}
			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Output of tape %q does not match golden file", tc.tape)
		})
	}
}

// buildPAM builds the PAM module in a temporary directory and returns a cleanup function.
func buildPAM(execPath string) (cleanup func(), err error) {
	cmd := exec.Command("go", "build")
	if testutils.CoverDir() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	cmd.Args = append(cmd.Args, "-tags=pam_binary_cli", "-o", filepath.Join(execPath, "pam_authd"), "../.")
	if out, err := cmd.CombinedOutput(); err != nil {
		return func() {}, fmt.Errorf("%v: %s", err, out)
	}

	return func() { _ = os.Remove(filepath.Join(execPath, "pam_authd")) }, nil
}

func TestMockgpasswd(t *testing.T) {
	grouptests.Mockgpasswd(t)
}

// appendGoBinToPath returns the value of the GOPATH defined in go env appended to PATH.
func appendGoBinToPath(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "env", "GOPATH")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Could not get GOPATH: %v: %s", err, out)

	env := os.Getenv("PATH")
	return fmt.Sprintf("PATH=%s:%s", strings.TrimSpace(string(out)+"/bin"), env)
}

// saveArtifactsForDebug saves the specified artifacts to a temporary directory if the test failed.
func saveArtifactsForDebug(t *testing.T, artifacts []string) {
	t.Helper()
	if !t.Failed() {
		return
	}

	// We need to copy the artifacts to a temporary directory, since the test directory will be cleaned up.
	tmpDir := filepath.Join("/tmp/authd-test-artifacts", testutils.GoldenPath(t))
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

func TestMain(m *testing.M) {
	// Due to external dependecies such as `vhs`, we can't run the tests in some environments (like LP builders), as we
	// can't install the dependencies there. So we need to be able to skip these tests on-demand.
	if os.Getenv("AUTHD_SKIP_EXTERNAL_DEPENDENT_TESTS") != "" {
		fmt.Println("Skipping tests with external dependencies as requested")
		return
	}

	// Needed to skip the test setup when running the gpasswd mock.
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	testutils.InstallUpdateFlag()
	flag.Parse()

	execPath, daemonCleanup, err := testutils.BuildDaemon("-tags=withexamplebroker,integrationtests")
	if err != nil {
		log.Printf("Setup: Failed to build authd daemon: %v", err)
		os.Exit(1)
	}
	defer daemonCleanup()
	daemonPath = execPath

	pamCleanup, err := buildPAM(filepath.Dir(execPath))
	if err != nil {
		log.Printf("Setup: Failed to build PAM executable: %v", err)
		daemonCleanup()
		os.Exit(1)
	}
	defer pamCleanup()

	m.Run()
}
