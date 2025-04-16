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
	err := os.WriteFile(groupFile, []byte("root:x:0:\n"+newGroupContents), 0644)
	require.NoError(t, err)

	// Try using gpasswd to modify the group file. This should succeed, because
	// the group file is not locked.
	output, err := runGPasswd(t, "--add", "root", "testgroup")
	require.NoError(t, err, "Output: %s", output)

	// Lock the group file
	err = userutils.LockGroupFile()
	require.NoError(t, err)

	output, err = runCmd(t, "getent", "group", "testgroup")
	require.NoError(t, err, "Output: %s", output)
	require.Equal(t, output, newGroupContents+",root", "Group not found")

	// Try using gpasswd to modify the group file. This should fail, because
	// the group file is locked.
	output, err = runGPasswd(t, "--delete", "root", "testgroup")
	require.Error(t, err, output)
	require.Contains(t, output, "gpasswd: cannot lock /etc/group")

	// Reading is allowed when locked.
	output, err = runCmd(t, "getent", "group", "testgroup")
	require.NoError(t, err, "Output: %s", output)
	require.Equal(t, output, newGroupContents+",root", "Group not found")

	// Unlock the group file
	err = userutils.UnlockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file again. This should succeed,
	// because the group file is unlocked.
	output, err = runGPasswd(t, "--delete", "root", "testgroup")
	require.NoError(t, err, "Output: %s", output)

	output, err = runCmd(t, "getent", "group", "testgroup")
	require.NoError(t, err, "Output: %s", output)
	require.Equal(t, output, newGroupContents, "Group not found")
}

func TestReadWhileLocked(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	groupFile := filepath.Join("/etc", "group")
	groupContents := `root:x:0:
testgroup:x:1001:testuser`

	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err := os.WriteFile(groupFile, []byte(groupContents), 0644)
	require.NoError(t, err)

	err = userutils.LockGroupFile()
	require.NoError(t, err, "Locking once it is allowed")
	t.Cleanup(func() { userutils.UnlockGroupFile() })

	output, err := runCmd(t, "getent", "group")
	require.NoError(t, err, "Reading should be allowed")
	require.Equal(t, groupContents, output)
}

func TestLockAndLockAgainGroupFile(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	err := userutils.LockGroupFile()
	require.NoError(t, err, "Locking once it is allowed")

	err = userutils.LockGroupFile()
	require.Error(t, err, "Locking again should not be allowed")

	err = userutils.UnlockGroupFile()
	require.NoError(t, err, "Unlocking should be allowed")
}

func TestUnlockUnlocked(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	err := userutils.UnlockGroupFile()
	require.Error(t, err, "Unlocking unlocked should not be allowed")
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
