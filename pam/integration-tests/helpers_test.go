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
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/fileutils"
	"github.com/ubuntu/authd/internal/grpcutils"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/testutils/golden"
	localgroupstestutils "github.com/ubuntu/authd/internal/users/localentries/testutils"
	"github.com/ubuntu/authd/pam/internal/pam_test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorbe.io/go/osrelease"
)

var (
	authdTestSessionTime     = time.Now()
	authdArtifactsDir        string
	authdArtifactsAlwaysSave bool
	authdArtifactsDirSync    sync.Once
)

type authdInstance struct {
	mu                sync.Mutex
	refCount          uint64
	socketPath        string
	gPasswdOutputPath string
	groupsFile        string
	cleanup           func()
}

var (
	sharedAuthdInstance = authdInstance{}
)

func runAuthdForTesting(t *testing.T, gpasswdOutput, groupsFile string, currentUserAsRoot bool, isSharedDaemon bool, args ...testutils.DaemonOption) (
	socketPath string, waitFunc func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	env := localgroupstestutils.AuthdIntegrationTestsEnvWithGpasswdMock(t, gpasswdOutput, groupsFile)
	if currentUserAsRoot {
		env = append(env, authdCurrentUserRootEnvVariableContent)
	}
	args = append(args, testutils.WithEnvironment(env...))

	outputFile := filepath.Join(t.TempDir(), "authd.log")
	args = append(args, testutils.WithOutputFile(outputFile))

	homeBaseDir := filepath.Join(t.TempDir(), "homes")
	err := os.MkdirAll(homeBaseDir, 0700)
	require.NoError(t, err, "Setup: Creating home base dir %q", homeBaseDir)
	args = append(args, testutils.WithHomeBaseDir(homeBaseDir))

	if !isSharedDaemon {
		database := filepath.Join(t.TempDir(), "db", consts.DefaultDatabaseFileName)
		args = append(args, testutils.WithDBPath(filepath.Dir(database)))
		saveArtifactsForDebugOnCleanup(t, []string{database})
	}
	if isSharedDaemon && authdArtifactsAlwaysSave {
		database := filepath.Join(authdArtifactsDir, "db", consts.DefaultDatabaseFileName)
		args = append(args, testutils.WithDBPath(filepath.Dir(database)))
	}

	socketPath, stopped := testutils.RunDaemon(ctx, t, daemonPath, args...)
	saveArtifactsForDebugOnCleanup(t, []string{outputFile})
	return socketPath, func() {
		cancel()
		<-stopped
	}
}

func runAuthd(t *testing.T, gpasswdOutput, groupsFile string, currentUserAsRoot bool, args ...testutils.DaemonOption) string {
	t.Helper()

	socketPath, waitFunc := runAuthdForTesting(t, gpasswdOutput, groupsFile, currentUserAsRoot, false, args...)
	t.Cleanup(waitFunc)
	return socketPath
}

func sharedAuthd(t *testing.T, args ...testutils.DaemonOption) (socketPath string, gpasswdFile string) {
	t.Helper()

	useSharedInstance := testutils.IsRace()
	if s, err := strconv.ParseBool(os.Getenv("AUTHD_TESTS_USE_SHARED_AUTHD_INSTANCES")); err == nil {
		useSharedInstance = s
	}

	if !useSharedInstance {
		gPasswd := filepath.Join(t.TempDir(), "gpasswd.output")
		groups := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
		socket, cleanup := runAuthdForTesting(t, gPasswd, groups, true, useSharedInstance, args...)
		t.Cleanup(cleanup)
		return socket, gPasswd
	}

	sa := &sharedAuthdInstance
	t.Cleanup(func() {
		sharedAuthdInstance.mu.Lock()
		defer sharedAuthdInstance.mu.Unlock()

		sa.refCount--
		if testutils.IsVerbose() {
			t.Logf("Authd shared instances decreased: %v", sa.refCount)
		}
		if sa.refCount != 0 {
			return
		}
		require.NotNil(t, sa.cleanup)
		cleanup := sa.cleanup
		sa.socketPath = ""
		sa.gPasswdOutputPath = ""
		sa.groupsFile = ""
		sa.cleanup = nil
		cleanup()
	})

	sharedAuthdInstance.mu.Lock()
	defer sharedAuthdInstance.mu.Unlock()

	sa.refCount++
	if testutils.IsVerbose() {
		t.Logf("Authd shared instances increased: %v", sa.refCount)
	}
	if sa.refCount != 1 {
		return sa.socketPath, sa.gPasswdOutputPath
	}

	args = append(slices.Clone(args), testutils.WithSharedDaemon(true))
	sa.gPasswdOutputPath = filepath.Join(t.TempDir(), "gpasswd.output")
	sa.groupsFile = filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")
	sa.socketPath, sa.cleanup = runAuthdForTesting(t, sa.gPasswdOutputPath,
		sa.groupsFile, true, useSharedInstance, args...)
	return sa.socketPath, sa.gPasswdOutputPath
}

func preparePamRunnerTest(t *testing.T, clientPath string) []string {
	t.Helper()

	// Due to external dependencies such as `vhs`, we can't run the tests in some environments (like LP builders), as we
	// can't install the dependencies there. So we need to be able to skip these tests on-demand.
	if os.Getenv("AUTHD_SKIP_EXTERNAL_DEPENDENT_TESTS") != "" {
		t.Skip("Skipping tests with external dependencies as requested")
	}

	pamCleanup, err := buildPAMRunner(clientPath)
	require.NoError(t, err, "Setup: Failed to build PAM executable")
	t.Cleanup(pamCleanup)

	return []string{
		fmt.Sprintf("%s=%s", pam_test.RunnerEnvExecModule, buildExecModule(t)),
		fmt.Sprintf("%s=%s", pam_test.RunnerEnvExecChildPath, buildPAMExecChild(t)),
	}
}

// buildPAMRunner builds the PAM module in a temporary directory and returns a cleanup function.
func buildPAMRunner(execPath string) (cleanup func(), err error) {
	cmd := exec.Command("go", "build")
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
	cmd.Args = append(cmd.Args, "-gcflags=all=-N -l")
	cmd.Args = append(cmd.Args, "-tags=withpamrunner", "-o", filepath.Join(execPath, "pam_authd"),
		"./pam/tools/pam-runner")
	if out, err := cmd.CombinedOutput(); err != nil {
		return func() {}, fmt.Errorf("%v: %s", err, out)
	}

	return func() { _ = os.Remove(filepath.Join(execPath, "pam_authd")) }, nil
}

func buildPAMExecChild(t *testing.T) string {
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
	cmd.Args = append(cmd.Args, "-gcflags=all=-N -l")
	cmd.Args = append(cmd.Args, "-tags=pam_debug")
	cmd.Env = append(os.Environ(), `CGO_CFLAGS=-O0 -g3`)

	authdPam := filepath.Join(t.TempDir(), "authd-pam")
	t.Logf("Compiling Exec child at %s", authdPam)
	t.Log(strings.Join(cmd.Args, " "))

	cmd.Args = append(cmd.Args, "-o", authdPam)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: could not compile PAM exec child: %s", out)

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
	require.NoError(t, grpcutils.WaitForConnection(context.TODO(), conn,
		sleepDuration(30*time.Second)))
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

func artifactsPath(t *testing.T) string {
	t.Helper()

	authdArtifactsDirSync.Do(func() {
		defer func() { t.Logf("Saving test artifacts at %s", authdArtifactsDir) }()

		authdArtifactsAlwaysSave = os.Getenv("AUTHD_TESTS_ARTIFACTS_ALWAYS_SAVE") != ""

		// We need to copy the artifacts to another directory, since the test directory will be cleaned up.
		authdArtifactsDir = os.Getenv("AUTHD_TESTS_ARTIFACTS_PATH")
		if authdArtifactsDir != "" {
			if err := os.MkdirAll(authdArtifactsDir, 0750); err != nil && !os.IsExist(err) {
				require.NoError(t, err, "TearDown: could not create artifacts directory %q", authdArtifactsDir)
			}
			return
		}

		st := authdTestSessionTime
		folderName := fmt.Sprintf("authd-test-artifacts-%d-%02d-%02dT%02d:%02d:%02d.%d-",
			st.Year(), st.Month(), st.Day(), st.Hour(), st.Minute(), st.Second(),
			st.UnixMilli())

		var err error
		authdArtifactsDir, err = os.MkdirTemp(os.TempDir(), folderName)
		require.NoError(t, err, "TearDown: could not create artifacts directory %q", authdArtifactsDir)
	})

	return authdArtifactsDir
}

// saveArtifactsForDebug saves the specified artifacts to a temporary directory if the test failed.
func saveArtifactsForDebug(t *testing.T, artifacts []string) {
	t.Helper()
	if !t.Failed() && !authdArtifactsAlwaysSave {
		return
	}

	tmpDir := filepath.Join(artifactsPath(t), golden.Path(t))
	err := os.MkdirAll(tmpDir, 0750)
	require.NoError(t, err, "TearDown: could not create temporary directory %q for artifacts", tmpDir)

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

func prepareGPasswdFiles(t *testing.T) (string, string) {
	t.Helper()

	gpasswdOutput := filepath.Join(t.TempDir(), "gpasswd.output")
	groupsFile := filepath.Join(testutils.TestFamilyPath(t), "gpasswd.group")

	// Do a copy of the original group file, since it may be migrated by authd.
	tmpCopy := filepath.Join(t.TempDir(), filepath.Base(groupsFile))
	err = fileutils.CopyFile(groupsFile, tmpCopy)
	require.NoError(t, err, "Cannot copy the group file %q", groupsFile)
	groupsFile = tmpCopy

	saveArtifactsForDebugOnCleanup(t, []string{gpasswdOutput, groupsFile})

	return gpasswdOutput, groupsFile
}

func getUbuntuVersion(t *testing.T) int {
	t.Helper()

	err := osrelease.Parse()
	require.NoError(t, err, "Can't parse os-release file %q: %v", osrelease.Path, err)

	var versionID string
	switch osrelease.Release.ID {
	case "ubuntu":
		versionID = strings.ReplaceAll(osrelease.Release.VersionID, ".", "")
	case "ubuntu-core":
		versionID = osrelease.Release.VersionID + "04"
	default:
		t.Logf("Not an ubuntu version: %q", osrelease.Release.ID)
		return 0
	}

	v, err := strconv.Atoi(versionID)
	require.NoError(t, err, "Can't parse version ID: %q", osrelease.Release.ID)
	return v
}

func nssTestEnvBase(t *testing.T, nssLibrary string) []string {
	t.Helper()

	require.NotEmpty(t, nssLibrary, "Setup: NSS Library is unset")
	return []string{
		"AUTHD_NSS_INFO=stderr",
		fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", filepath.Dir(nssLibrary),
			os.Getenv("LD_LIBRARY_PATH")),
	}
}

func nssTestEnv(t *testing.T, nssLibrary, authdSocket string) []string {
	t.Helper()

	baseEnv := nssTestEnvBase(t, nssLibrary)

	var preloadLibraries []string
	if testutils.IsAsan() {
		asanPath, err := exec.Command("gcc", "-print-file-name=libasan.so").Output()
		require.NoError(t, err, "Setup: Looking for ASAN lib path")
		preloadLibraries = []string{strings.TrimSpace(string(asanPath))}
	}
	preloadLibraries = append(preloadLibraries, nssLibrary)
	baseEnv = append(baseEnv, fmt.Sprintf("LD_PRELOAD=%s",
		strings.Join(preloadLibraries, ":")))

	if authdSocket == "" {
		return baseEnv
	}

	return append(baseEnv, fmt.Sprintf("AUTHD_NSS_SOCKET=%s", authdSocket))
}

func requireAuthdUser(t *testing.T, client authd.UserServiceClient, user string) *authd.User {
	t.Helper()

	authdUser, err := client.GetUserByName(context.Background(),
		&authd.GetUserByNameRequest{Name: user, ShouldPreCheck: false})
	require.NoError(t, err, "User %q is expected to exist", user)
	require.NotNil(t, authdUser, "User %q is expected to be not nil", user)

	require.Equal(t, strings.ToLower(user), authdUser.Name, "Passwd user does not match")
	require.Equal(t, "/bin/sh", authdUser.Shell, "Unexpected Shell for user %q", user)
	require.NotZero(t, authdUser.Uid, "Unexpected UID for user %q", user)
	require.NotZero(t, authdUser.Gid, "Unexpected GID for user %q", user)
	require.NotEmpty(t, authdUser.Homedir, "Unexpected HOME for user %q", user)
	require.NotEmpty(t, authdUser.Gecos, "Unexpected Gecos for user %q", user)

	p, err := client.GetUserByID(context.Background(),
		&authd.GetUserByIDRequest{Id: authdUser.Uid})
	require.NoError(t, err, "User %q is expected to exist as id %v", authdUser.Uid)
	require.Equal(t, authdUser, p, "User by ID does not match")

	return authdUser
}

func requireNoAuthdUser(t *testing.T, client authd.UserServiceClient, user string) {
	t.Helper()

	_, err := client.GetUserByName(context.Background(),
		&authd.GetUserByNameRequest{Name: user, ShouldPreCheck: false})
	require.Error(t, err, "User %q is not expected to exist")
}

func requireAuthdGroup(t *testing.T, client authd.UserServiceClient, gid uint32) *authd.Group {
	t.Helper()

	group, err := client.GetGroupByID(context.Background(),
		&authd.GetGroupByIDRequest{Id: gid})
	require.NoError(t, err, "Group %v is expected to exist", gid)
	require.NotNil(t, group, "Group %v is expected to be not nil", gid)

	require.Equal(t, gid, group.Gid, group.Name, "Group ID does not match")
	require.NotEmpty(t, group.Name, "Group does not match")
	require.NotEmpty(t, group.Gid, "Unexpected GID for group %q", group.Name)
	require.NotEmpty(t, group.Members, "Unexpected Members for user %q", group.Name)

	g, err := client.GetGroupByName(context.Background(),
		&authd.GetGroupByNameRequest{Name: group.Name})
	require.NoError(t, err, "Group %q is expected to exist", group.Name)
	require.Equal(t, group, g, "Group by name %q does not match", group.Name)

	return group
}

func getEntOutput(t *testing.T, nssLibrary, authdSocket, db, key string) string {
	t.Helper()

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command("getent", db, key)
	cmd.Env = nssTestEnv(t, nssLibrary, authdSocket)

	out, err := cmd.Output()
	require.NoError(t, err, "getent %s should not fail for key %q\n%s", db, key)

	o := strings.TrimSpace(string(out))
	t.Log(strings.Join(cmd.Args, " "), "returned:", o)

	return o
}

func requireGetEntOutputEqualsUser(t *testing.T, getentOutput string, user *authd.User) {
	t.Helper()

	require.Equal(t, []string{
		user.Name,
		"x",
		fmt.Sprint(user.Uid),
		fmt.Sprint(user.Gid),
		user.Gecos,
		user.Homedir,
		user.Shell,
	}, strings.Split(getentOutput, ":"), "Unexpected getent output: %s", getentOutput)
}

func requireGetEntEqualsUser(t *testing.T, nssLibrary, authdSocket, userName string, user *authd.User) {
	t.Helper()

	requireGetEntOutputEqualsUser(t,
		getEntOutput(t, nssLibrary, authdSocket, "passwd", userName), user)

	requireGetEntOutputEqualsUser(t,
		getEntOutput(t, nssLibrary, authdSocket, "passwd", fmt.Sprint(user.Uid)),
		user)
}

func requireGetEntOutputEqualsGroup(t *testing.T, getentOutput string, group *authd.Group) {
	t.Helper()

	require.Equal(t, []string{
		group.Name,
		group.Passwd,
		fmt.Sprint(group.Gid),
		strings.Join(group.Members, ","),
	}, strings.Split(getentOutput, ":"), "Unexpected getent output: %s", getentOutput)
}

func requireGetEntEqualsGroup(t *testing.T, nssLibrary, authdSocket, name string, group *authd.Group) {
	t.Helper()

	requireGetEntOutputEqualsGroup(t,
		getEntOutput(t, nssLibrary, authdSocket, "group", name), group)

	requireGetEntOutputEqualsGroup(t,
		getEntOutput(t, nssLibrary, authdSocket, "group", fmt.Sprint(group.Gid)), group)
}

func requireGetEntExists(t *testing.T, nssLibrary, authdSocket, user string, exists bool) {
	t.Helper()

	// #nosec:G204 - we control the command arguments in tests
	cmd := exec.Command("getent", "passwd", user)
	cmd.Env = nssTestEnv(t, nssLibrary, authdSocket)
	out, err := cmd.CombinedOutput()

	if !exists {
		require.Error(t, err, "getent should fail for user %q\n%s", user, out)
		return
	}
	require.NoError(t, err, "getent should not fail for user %q\n%s", user, out)
}

func useOldDatabaseEnv(t *testing.T, oldDB string) []string {
	t.Helper()

	if oldDB == "" {
		return nil
	}

	cwd, err := os.Getwd()
	require.NoError(t, err, "Cannot get current working directory")

	tempDir := t.TempDir()
	oldDBDir, err := os.MkdirTemp(tempDir, "old-db-path")
	require.NoError(t, err, "Cannot create db directory in %q", tempDir)

	testDataDB := filepath.Join(cwd, "testdata", "databases", oldDB)
	t.Logf("Using database at %q", testDataDB)
	err = os.CopyFS(oldDBDir, os.DirFS(testDataDB))
	require.NoError(t, err, "Cannot copy contents of %q to %q", testDataDB, oldDBDir)

	return []string{fmt.Sprintf("AUTHD_INTEGRATIONTESTS_OLD_DB_DIR=%s", oldDBDir)}
}
