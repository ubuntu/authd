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

//go:generate go build -o testhelpers/gpasswd ./testhelpers/gpasswd.go

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

	// Ensure the helper binary is built before running the test
	cmd := exec.Command("go", "generate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err)

	// Create a temporary group file
	tempGroupFile := t.TempDir() + "/group"
	require.NoError(t, err)
	// Create the temporary group file
	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err = os.WriteFile(tempGroupFile, []byte("testgroup:x:1001:testuser"), 0644)
	require.NoError(t, err)

	// Set the group file to the temporary file
	origGroupFile := userutils.GroupFile
	userutils.GroupFile = tempGroupFile
	defer func() { userutils.GroupFile = origGroupFile }()

	// Try using gpasswd to modify the group file. This should succeed, because
	// the group file is not locked.
	output, err := runGPasswd(tempGroupFile, "--add", "root", "testgroup")
	require.NoError(t, err, string(output))

	// Lock the group file
	err = userutils.LockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file. This should fail, because
	// the group file is locked.
	output, err = runGPasswd(tempGroupFile, "--delete", "root", "testgroup")
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
	output, err = runGPasswd(tempGroupFile, "--delete", "root", "testgroup")
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

func runGPasswd(groupFile string, args ...string) ([]byte, error) {
	args = append([]string{
		"env", "GROUP_FILE=" + groupFile,
		"testhelpers/gpasswd",
	}, args...)

	if useSudo {
		args = append([]string{"sudo"}, args...)
	}

	log.Infof(context.Background(), "Running command: %s", strings.Join(args, " "))
	//nolint:gosec // G204 It's fine to pass variables to exec.Command here
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.CombinedOutput()
}
