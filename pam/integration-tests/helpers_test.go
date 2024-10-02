package main_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/testutils"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localgroups/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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

func prepareClientTest(t *testing.T, clientPath string) []string {
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
		fmt.Sprintf("%s=%s", pam_test.ClientEnvExecModule, buildExecModule(t)),
		fmt.Sprintf("%s=%s", pam_test.ClientEnvPath, buildPAMClient(t)),
	}
}

// buildPAMTestClient builds the PAM module in a temporary directory and returns a cleanup function.
func buildPAMTestClient(execPath string) (cleanup func(), err error) {
	cmd := exec.Command("go", "build")
	if testutils.CoverDirForTests() != "" {
		// -cover is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-cover")
	}
	if testutils.IsAsan() {
		// -asan is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-asan")
	}
	if testutils.IsRace() {
		cmd.Args = append(cmd.Args, "-race")
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
	if testutils.IsAsan() {
		// -asan is a "positional flag", so it needs to come right after the "build" command.
		cmd.Args = append(cmd.Args, "-asan")
	}
	if testutils.IsRace() {
		cmd.Args = append(cmd.Args, "-race")
	}
	cmd.Args = append(cmd.Args, "-gcflags=-dwarflocationlists=true")
	cmd.Env = append(os.Environ(), `CGO_CFLAGS=-O0 -g3`)

	authdPam := filepath.Join(t.TempDir(), "authd-pam")
	t.Logf("Compiling Exec client at %s", authdPam)
	t.Log(strings.Join(cmd.Args, " "))

	cmd.Args = append(cmd.Args, "-o", authdPam)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not compile PAM client: %s", out)

	return authdPam
}

func prepareFileLogging(t *testing.T, fileName string) string {
	t.Helper()

	cliLog := filepath.Join(t.TempDir(), fileName)
	saveArtifactsForDebugOnCleanup(t, []string{cliLog})
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

func requirePreviousBrokerForUser(t *testing.T, socketPath string, brokerName string, user string) {
	t.Helper()

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(errmessages.FormatErrorMessage))
	require.NoError(t, err, "Can't connect to authd socket")

	t.Cleanup(func() { conn.Close() })
	pamClient := authd.NewPAMClient(conn)
	brokers, err := pamClient.AvailableBrokers(context.TODO(), nil)
	require.NoError(t, err, "Can't get available brokers")
	prevBroker, err := pamClient.GetPreviousBroker(context.TODO(), &authd.GPBRequest{Username: user})
	require.NoError(t, err, "Can't get previous broker")
	var prevBrokerID string
	for _, b := range brokers.BrokersInfos {
		if b.Name == brokerName {
			prevBrokerID = b.Id
		}
	}
	require.Equal(t, prevBroker.PreviousBroker, prevBrokerID)
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

func saveArtifactsForDebugOnCleanup(t *testing.T, artifacts []string) {
	t.Helper()
	t.Cleanup(func() { saveArtifactsForDebug(t, artifacts) })
}

func sleepDuration(in time.Duration) time.Duration {
	return time.Duration(math.Round(float64(in) * testutils.SleepMultiplier()))
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
