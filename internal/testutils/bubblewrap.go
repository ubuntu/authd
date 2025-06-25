package testutils

import (
	"bytes"
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

// RunInBubbleWrapWithEnv runs the passed commands in bubble wrap sandbox with env variables.
func RunInBubbleWrapWithEnv(t *testing.T, testDataPath string, env []string, args ...string) (string, error) {
	t.Helper()

	SkipIfCannotRunBubbleWrap(t)
	return runInBubbleWrap(t, bubbleWrapNeedsSudo, testDataPath, env, args...)
}

// RunInBubbleWrap runs the passed commands in bubble wrap sandbox.
func RunInBubbleWrap(t *testing.T, testDataPath string, args ...string) (string, error) {
	t.Helper()

	return RunInBubbleWrapWithEnv(t, testDataPath, nil, args...)
}

func runInBubbleWrap(t *testing.T, withSudo bool, testDataPath string, env []string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("bwrap")
	cmd.Env = AppendCovEnv(os.Environ())
	cmd.Env = append(cmd.Env, env...)

	if withSudo {
		cmd.Args = append([]string{"sudo"}, cmd.Args...)
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
		"--ro-bind", "/etc/timezone", "/etc/timezone",
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
		t.Log(output)
	}
	return output, err
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
