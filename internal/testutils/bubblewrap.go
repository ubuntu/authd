package testutils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	buildBubbleWrapRunnerMu sync.Mutex
	buildBubbleWrapRunner   string

	bubbleWrapSupportsUnprivilegedNamespacesOnce sync.Once
	bubbleWrapSupportsUnprivilegedNamespaces     bool

	bubbleWrapNeedsSudoOnce sync.Once
	bubbleWrapNeedsSudo     bool
)

// CanRunBubbleWrapTest checks whether we can run tests running in bubblewrap or
// sip the tests otherwise.
func CanRunBubbleWrapTest(t *testing.T) {
	t.Helper()

	_, err := exec.LookPath("bwrap")
	require.NoError(t, err, "Setup: bwrap cannot be found")

	if os.Geteuid() == 0 {
		t.Log("Running as EUID 0")
		return
	}

	bubbleWrapSupportsUnprivilegedNamespacesOnce.Do(func() {
		bubbleWrapSupportsUnprivilegedNamespaces = canUseUnprivilegedUserNamespaces(t)
	})
	if bubbleWrapSupportsUnprivilegedNamespaces {
		t.Log("Can use unprivileged user namespaces")
		return
	}

	bubbleWrapNeedsSudoOnce.Do(func() {
		bubbleWrapNeedsSudo = canUseSudoNonInteractively(t)
	})
	if bubbleWrapNeedsSudo {
		t.Log("Can use sudo non-interactively")
	}

	t.Skip("Skipping test: requires root privileges or unprivileged user namespaces")
}

// RunInBubbleWrapWithEnv runs the passed commands in bubble wrap sandbox with env variables.
func RunInBubbleWrapWithEnv(t *testing.T, testDataPath string, env []string, args ...string) (string, error) {
	t.Helper()

	CanRunBubbleWrapTest(t)
	return runInBubbleWrap(t, bubbleWrapNeedsSudo, testDataPath, env, args...)
}

// RunInBubbleWrap runs the passed commands in bubble wrap sandbox.
func RunInBubbleWrap(t *testing.T, testDataPath string, args ...string) (string, error) {
	t.Helper()

	return RunInBubbleWrapWithEnv(t, testDataPath, nil, args...)
}

func runInBubbleWrap(t *testing.T, withSudo bool, testDataPath string, env []string, args ...string) (string, error) {
	t.Helper()

	var envArgs []string
	if withSudo {
		envArgs = append(envArgs, "sudo")
	}
	envArgs = append(envArgs,
		"env",
		fmt.Sprintf("BUBBLEWRAP_TEST_DATA=%s", testDataPath),
	)
	if e := CoverDirEnv(); e != "" {
		envArgs = append(envArgs, e)
	}
	envArgs = append(envArgs, env...)
	envArgs = append(envArgs, buildBubbleWrapRunnerOnce(t))

	args = append(envArgs, args...)

	//nolint:gosec // G204 It's fine to pass variables to exec.Command here
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = AppendCovEnv(os.Environ())

	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	if IsVerbose() {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
	}

	t.Log("Running command", strings.Join(args, " "))
	err := cmd.Run()
	output := strings.TrimSpace(b.String())

	if !IsVerbose() {
		t.Log(output)
	}
	return output, err
}

func buildBubbleWrapRunnerOnce(t *testing.T) string {
	t.Helper()

	buildBubbleWrapRunnerMu.Lock()
	defer buildBubbleWrapRunnerMu.Unlock()

	if buildBubbleWrapRunner != "" {
		return buildBubbleWrapRunner
	}

	t.Cleanup(func() {
		buildBubbleWrapRunnerMu.Lock()
		buildBubbleWrapRunner = ""
		buildBubbleWrapRunnerMu.Unlock()
	})

	helperPath := filepath.Join(t.TempDir(), "run-in-bubblewrap")
	cmd := exec.Command("go", "build")

	// All these are "positional flag", so it needs to come right after the "build" command.
	if CoverDirForTests() != "" {
		cmd.Args = append(cmd.Args, "-cover")
	}
	if IsAsan() {
		cmd.Args = append(cmd.Args, "-asan")
	}
	if IsRace() {
		cmd.Args = append(cmd.Args, "-race")
	}

	cmd.Args = append(cmd.Args, []string{
		"-o", helperPath,
		"-tags", "testutils_testhelpers",
		filepath.Join(CurrentDir(), "testhelpers",
			"bubblewrap-runner", "bubblewrap-runner.go"),
	}...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: cannot compile helper tool: %s", out)

	buildBubbleWrapRunner = helperPath
	return buildBubbleWrapRunner
}

func canUseUnprivilegedUserNamespaces(t *testing.T) bool {
	t.Helper()

	if out, err := runInBubbleWrap(t, false, t.TempDir(), nil, "/bin/true"); err != nil {
		t.Logf("Can't use unprivileged user namespaces: %v\n%s", err, out)
		return false
	}

	return true
}

func canUseSudoNonInteractively(t *testing.T) bool {
	t.Helper()

	cmd := exec.Command("sudo", "-Nnv")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("Can't use sudo non-interactively: %v\n%s", err, out)
		return false
	}

	if out, err := runInBubbleWrap(t, true, t.TempDir(), nil, "/bin/true"); err != nil {
		t.Logf("Can't use user namespaces: %v\n%s", err, out)
		return false
	}

	return true
}
