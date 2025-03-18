package userutils_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/userutils"
)

//go:generate go build -o testhelpers/gpasswd ./testhelpers/gpasswd.go

func TestLockAndUnlockGroupFile(t *testing.T) {
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

func runGPasswd(groupFile string, args ...string) ([]byte, error) {
	cmd := exec.Command("testhelpers/gpasswd", args...)
	cmd.Env = append(os.Environ(), "GROUP_FILE="+groupFile)
	return cmd.CombinedOutput()
}
