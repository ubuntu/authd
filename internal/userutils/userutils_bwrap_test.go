//go:build bubblewrap_test

package userutils_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/userutils"
)

func TestLockAndUnlockGroupFile(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	groupFile := filepath.Join("/etc", "group")
	newGroupContents := "testgroup:x:1001:testuser"
	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err := os.WriteFile(groupFile, []byte(newGroupContents), 0644)
	require.NoError(t, err)

	// Try using gpasswd to modify the group file. This should succeed, because
	// the group file is not locked.
	output, err := runGPasswd(t, "--add", "root", "testgroup")
	require.NoError(t, err, "Output: %s", output)

	// Lock the group file
	err = userutils.LockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file. This should fail, because
	// the group file is locked.
	output, err = runGPasswd(t, "--delete", "root", "testgroup")
	require.Error(t, err, output)
	require.Contains(t, output, "gpasswd: cannot lock /etc/group")

	// Try locking the group file again. This should fail, because the group
	// file is already locked.
	err = userutils.LockGroupFile()
	require.Error(t, err)

	// Unlock the group file
	err = userutils.UnlockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file again. This should succeed,
	// because the group file is unlocked.
	output, err = runGPasswd(t, "--delete", "root", "testgroup")
	require.NoError(t, err, "Output: %s", output)
}

func runCmd(t *testing.T, command string, args ...string) (string, error) {
	t.Helper()

	//nolint:gosec // G204 It's fine to pass variables to exec.Command here
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	t.Logf("Running command: %s", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runGPasswd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	return runCmd(t, "gpasswd", args...)
}
