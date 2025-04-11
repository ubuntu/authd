package userutils_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/userutils"
	"github.com/ubuntu/authd/log"
)

var useSudo bool
var gpasswdHelperPath string
var groupFile string

func TestLockAndUnlockGroupFile(t *testing.T) {
	// This test requires either root privileges, unprivileged user namespaces, or being
	// able to run sudo without user interaction.
	if os.Geteuid() == 0 {
		log.Info(context.Background(), "Running as EUID 0")
	} else if canUseUnprivilegedUserNamespaces() {
		log.Info(context.Background(), "Can use unprivileged user namespaces")
	} else if canUseSudoNonInteractively() {
		log.Info(context.Background(), "Can use sudo non-interactively")
		useSudo = true
	} else {
		t.Skip("Skipping test: requires root privileges or unprivileged user namespaces")
	}

	// Build the helper binary
	tempDir := t.TempDir()
	gpasswdHelperPath = tempDir + "/gpasswd"
	//nolint:gosec // G204 It's fine to pass a variable to exec.Command here
	cmd := exec.Command("go", "build",
		"-o", gpasswdHelperPath,
		"-tags", "userutils_testhelpers",
		"./testhelpers/gpasswd.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err)

	// Create a temporary group file
	groupFile = t.TempDir() + "/group"
	require.NoError(t, err)
	// Create the temporary group file
	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err = os.WriteFile(groupFile, []byte("testgroup:x:1001:testuser"), 0644)
	require.NoError(t, err)

	// Set the group file to the temporary file
	userutils.GroupFile = groupFile

	// Try using gpasswd to modify the group file. This should succeed, because
	// the group file is not locked.
	output, err := runGPasswd("--add", "root", "testgroup")
	require.NoError(t, err, string(output))

	// Lock the group file
	err = userutils.LockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file. This should fail, because
	// the group file is locked.
	output, err = runGPasswd("--delete", "root", "testgroup")
	require.Error(t, err, string(output))
	require.Contains(t, string(output), "gpasswd: cannot lock /etc/group")

	// Try locking the group file again. This should fail, because the group file is already locked.
	err = userutils.LockGroupFile()
	require.Error(t, err)

	// Unlock the group file
	err = userutils.UnlockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file again. This should succeed,
	// because the group file is unlocked.
	output, err = runGPasswd("--delete", "root", "testgroup")
	require.NoError(t, err, string(output))
}

func canUseUnprivilegedUserNamespaces() bool {
	cmd := exec.Command("bwrap", "--ro-bind", "/", "/", "--unshare-user", "--uid", "0", "/bin/true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Warningf(context.Background(), "Can't use unprivileged user namespaces: %v", err)
		return false
	}

	return true
}

func canUseSudoNonInteractively() bool {
	cmd := exec.Command("sudo", "-Nnv")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Warningf(context.Background(), "Can't use sudo non-interactively: %v", err)
		return false
	}

	return true
}

func runGPasswd(args ...string) ([]byte, error) {
	args = append([]string{
		"env", "GROUP_FILE=" + groupFile,
		gpasswdHelperPath,
	}, args...)

	if useSudo {
		args = append([]string{"sudo"}, args...)
	}

	log.Infof(context.Background(), "Running command: %s", strings.Join(args, " "))
	//nolint:gosec // G204 It's fine to pass variables to exec.Command here
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.CombinedOutput()
}
