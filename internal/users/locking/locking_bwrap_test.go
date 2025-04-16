//go:build bubblewrap_test

package userslocking_test

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testutils"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
)

func TestLockAndWriteUnlock(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	groupFile := filepath.Join("/etc", "group")
	newGroupContents := "testgroup:x:1001:testuser"
	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err := os.WriteFile(groupFile, []byte("root:x:0:\n"+newGroupContents), 0644)
	require.NoError(t, err, "Writing group file")

	// Try using gpasswd to modify the group file. This should succeed, because
	// the group file is not locked.
	output, err := runGPasswd(t, "--add", "root", "testgroup")
	require.NoError(t, err, "Output: %s", output)

	// Lock the group file
	err = userslocking.WriteLock()
	require.NoError(t, err, "Locking database")

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
	err = userslocking.WriteUnlock()
	require.NoError(t, err, "Unlocking database")

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
	require.NoError(t, err, "Writing group file")

	err = userslocking.WriteLock()
	require.NoError(t, err, "Locking once it is allowed")
	t.Cleanup(func() { userslocking.WriteUnlock() })

	output, err := runCmd(t, "getent", "group")
	require.NoError(t, err, "Reading should be allowed")
	require.Equal(t, groupContents, output)
}

func TestLockAndLockAgainGroupFileOverridden(t *testing.T) {
	userslocking.Z_ForTests_OverrideLocking()
	restoreFunc := userslocking.Z_ForTests_RestoreLocking
	t.Cleanup(func() { restoreFunc() })

	err := userslocking.WriteLock()
	require.NoError(t, err, "Locking once it is allowed")

	err = userslocking.WriteLock()
	require.ErrorIs(t, err, userslocking.ErrLock, "Locking again should not be allowed")

	err = userslocking.WriteUnlock()
	require.NoError(t, err, "Unlocking should be allowed")

	// Ensure restoring works as expected.
	restoreFunc = func() {}
	userslocking.Z_ForTests_RestoreLocking()

	groupFile := filepath.Join("/etc", "group")
	groupContents := "testgroup:x:1001:testuser"

	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err = os.WriteFile(groupFile, []byte(groupContents), 0644)
	require.NoError(t, err, "Writing group file")

	err = userslocking.WriteLock()
	require.NoError(t, err, "Locking once it is allowed")
	t.Cleanup(func() { userslocking.WriteUnlock() })

	gPasswdExited := make(chan error)
	go func() {
		_, err := runGPasswd(t, "--add", "root", "testgroup")
		gPasswdExited <- err
	}()

	select {
	case <-time.After(sleepDuration(3 * time.Second)):
		// If we're time-outing: it's fine, it means we were locked!
	case err := <-gPasswdExited:
		require.ErrorIs(t, err, userslocking.ErrLock, "GPasswd should fail")
	}

	require.NoError(t, userslocking.WriteUnlock())
	<-gPasswdExited
}

func TestUnlockUnlockedOverridden(t *testing.T) {
	userslocking.Z_ForTests_OverrideLocking()
	t.Cleanup(userslocking.Z_ForTests_RestoreLocking)

	err := userslocking.WriteUnlock()
	require.ErrorIs(t, err, userslocking.ErrUnlock, "Unlocking unlocked should not be allowed")
}

func TestLockAndLockAgainGroupFile(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	err := userslocking.WriteLock()
	require.NoError(t, err, "Locking once it is allowed")

	err = userslocking.WriteLock()
	require.ErrorIs(t, err, userslocking.ErrLock, "Locking again should not be allowed")

	err = userslocking.WriteUnlock()
	require.NoError(t, err, "Unlocking should be allowed")
}

func TestUnlockUnlocked(t *testing.T) {
	require.Zero(t, os.Geteuid(), "Not root")

	err := userslocking.WriteUnlock()
	require.ErrorIs(t, err, userslocking.ErrUnlock, "Unlocking unlocked should not be allowed")
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

func sleepDuration(in time.Duration) time.Duration {
	return time.Duration(math.Round(float64(in) * testutils.SleepMultiplier()))
}
