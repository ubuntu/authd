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
	"github.com/ubuntu/authd/internal/fileutils"
)

var (
	bubbleWrapSupportsUnprivilegedNamespacesOnce sync.Once
	bubbleWrapSupportsUnprivilegedNamespaces     bool

	bubbleWrapNeedsSudoOnce sync.Once
	bubbleWrapNeedsSudo     bool
)

const bubbleWrapTestEnvVar = "BUBBLEWRAP_TEST"

// RunningInBubblewrap returns true if the test is being run in bubblewrap.
func RunningInBubblewrap() bool {
	return os.Getenv(bubbleWrapTestEnvVar) == "1"
}

// SkipIfCannotRunBubbleWrap checks whether we can run tests running in bubblewrap or
// skip the tests otherwise.
func SkipIfCannotRunBubbleWrap(t *testing.T) {
	t.Helper()

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

// RunTestInBubbleWrap runs the given test in bubblewrap.
func RunTestInBubbleWrap(t *testing.T, args ...string) {
	t.Helper()

	SkipIfCannotRunBubbleWrap(t)

	testBinary := compileTestBinary(t)

	testCommand := []string{testBinary, "-test.run", "^" + t.Name() + "$"}
	if testing.Verbose() {
		testCommand = append(testCommand, "-test.v")
	}
	if c := CoverDirForTests(); c != "" {
		testCommand = append(testCommand, fmt.Sprintf("-test.gocoverdir=%s", c))
	}
	args = append(args, testCommand...)

	err := runInBubbleWrap(t, bubbleWrapNeedsSudo, "", nil, args...)
	if err != nil {
		t.Fatalf("Running %s in bubblewrap failed: %v", t.Name(), err)
	}
}

func runInBubbleWrap(t *testing.T, withSudo bool, testDataPath string, env []string, args ...string) error {
	t.Helper()

	cmd := exec.Command("bwrap")
	cmd.Env = AppendCovEnv(os.Environ())
	cmd.Env = append(cmd.Env, env...)
	cmd.Env = append(cmd.Env, bubbleWrapTestEnvVar+"=1")

	if withSudo {
		cmd.Args = append([]string{"sudo"}, cmd.Args...)
	}

	if testDataPath == "" {
		testDataPath = t.TempDir()
	}

	etcDir := filepath.Join(testDataPath, "etc")
	err := os.MkdirAll(etcDir, 0700)
	require.NoError(t, err, "Impossible to create /etc")

	cmd.Args = append(cmd.Args,
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--bind", os.TempDir(), os.TempDir(),
		"--bind", testDataPath, testDataPath,
		"--bind", etcDir, "/etc",

		// Bind relevant etc files. We go manual here, since there's no
		// need to get much more than those, while we could in theory just
		// bind everything that is in host, and excluding the ones we want
		// to override.
		"--ro-bind", "/etc/environment", "/etc/environment",
		"--ro-bind", "/etc/localtime", "/etc/localtime",
		"--ro-bind", "/etc/login.defs", "/etc/login.defs",
		"--ro-bind", "/etc/nsswitch.conf", "/etc/nsswitch.conf",
		"--ro-bind", "/etc/passwd", "/etc/passwd",
		"--ro-bind", "/etc/shadow", "/etc/shadow",
		"--ro-bind", "/etc/subgid", "/etc/subgid",
		"--ro-bind", "/etc/sudo.conf", "/etc/sudo.conf",
		"--ro-bind", "/etc/sudoers", "/etc/sudoers",
		"--ro-bind-try", "/etc/timezone", "/etc/timezone",
		"--ro-bind", "/etc/pam.d", "/etc/pam.d",
		"--ro-bind", "/etc/security", "/etc/security",
	)

	replicateHostFile := func(file string) {
		require.NotContains(t, cmd.Args, file,
			"Setup: %q should not be managed by bwrap", file)
		dst := filepath.Join(testDataPath, file)
		err := fileutils.CopyFile(file, dst)
		require.NoError(t, err, "Setup: Copying %q to %q failed", file, dst)
	}

	// These are the files that we replicate in the bwrap environment and that
	// can be safely modified or mocked in the test.
	// Adapt this as needed, ensuring these files are not bound.
	for _, f := range []string{
		"/etc/group",
	} {
		replicateHostFile(f)
	}

	if coverDir := CoverDirForTests(); coverDir != "" {
		cmd.Args = append(cmd.Args, "--bind", coverDir, coverDir)
	}

	if os.Geteuid() != 0 && !withSudo {
		cmd.Args = append(cmd.Args, "--unshare-user", "--uid", "0")
	}

	cmd.Args = append(cmd.Args, args...)

	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	if testing.Verbose() {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
	}

	t.Log("Running command:", cmd.String())
	err = cmd.Run()
	output := strings.TrimSpace(b.String())

	if !testing.Verbose() {
		t.Logf("Command output\n%s", output)
	}
	return err
}

func canUseUnprivilegedUserNamespaces(t *testing.T) bool {
	t.Helper()

	if err := runInBubbleWrap(t, false, t.TempDir(), nil, "/bin/true"); err != nil {
		t.Logf("Can't use user namespaces: %v", err)
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

	if err := runInBubbleWrap(t, true, t.TempDir(), nil, "/bin/true"); err != nil {
		t.Logf("Can't use bubblewrap with sudo: %v", err)
		return false
	}

	return true
}

func compileTestBinary(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "test")
	// These are positional arguments.
	if CoverDirForTests() != "" {
		cmd.Args = append(cmd.Args, "-cover")
	}
	if IsAsan() {
		cmd.Args = append(cmd.Args, "-asan")
	}
	if IsRace() {
		cmd.Args = append(cmd.Args, "-race")
	}

	testBinary := filepath.Join(t.TempDir(), "test-binary")
	cmd.Args = append(cmd.Args, []string{
		"-tags", "bubblewrap_test", "-c", "-o", testBinary,
	}...)

	t.Logf("Compiling test binary: %s", strings.Join(cmd.Args, " "))
	compileOut, err := cmd.CombinedOutput()
	require.NoError(t, err, "Setup: Cannot compile test file: %s", compileOut)

	return testBinary
}
