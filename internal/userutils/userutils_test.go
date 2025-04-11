package userutils_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/userutils"
	"github.com/ubuntu/authd/log"
)

func TestLockAndUnlockGroupFile(t *testing.T) {
	// Create a temporary group file
	tempGroupFile := t.TempDir() + "/group"
	//nolint:gosec // G306 The group file is expected to have permissions 0644
	err := os.WriteFile(tempGroupFile, []byte("testgroup:x:1001:testuser"), 0644)
	require.NoError(t, err)

	// Set the group file to the temporary file
	userutils.SetGroupFile(tempGroupFile)

	// Try using gpasswd to modify the group file. This should succeed, because
	// the group file is not locked.
	output, err := runGPasswdHelper(t, tempGroupFile, "--add", "root", "testgroup")
	require.NoError(t, err, string(output))

	// Lock the group file
	err = userutils.LockGroupFile()
	require.NoError(t, err)

	// Try using gpasswd to modify the group file. This should fail, because
	// the group file is locked.
	output, err = runGPasswdHelper(t, tempGroupFile, "--delete", "root", "testgroup")
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
	output, err = runGPasswdHelper(t, tempGroupFile, "--delete", "root", "testgroup")
	require.NoError(t, err, string(output))
}

func runGPasswdHelper(t *testing.T, groupFile string, args ...string) ([]byte, error) {
	t.Helper()

	args = append([]string{"-test.run=TestHelperProcess", "--", "gpasswd", groupFile}, args...)
	// #nosec:G204 it's fine that we allow passing arbitrary arguments to gpasswd
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd.CombinedOutput()
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// Get the arguments after the '--'
	args := os.Args
	for len(args) > 0 {
		if args[0] != "--" {
			args = args[1:]
			continue
		}
		args = args[1:]
		break
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "gpasswd":
		if len(args) == 0 {
			t.Fatal("gpasswd command requires at least one argument")
		}
		groupFile, args := args[0], args[1:]
		execGPasswd(t, groupFile, args...)
	default:
		t.Fatalf("unknown command: %s", cmd)
	}
}

func execGPasswd(t *testing.T, groupFile string, args ...string) {
	t.Helper()

	if filepath.Base(groupFile) != "group" {
		t.Fatalf("the group file must be named 'group'.")
	}

	bwrapPath, err := exec.LookPath("bwrap")
	require.NoError(t, err)

	args = append([]string{
		bwrapPath,
		"--unshare-user",
		"--uid", "0",
		"--ro-bind", "/", "/",
		"--bind", filepath.Dir(groupFile), "/etc",
		"--ro-bind", "/etc/passwd", "/etc/passwd",
		"gpasswd",
	}, args...)

	log.Infof(context.Background(), "Executing command: %s", strings.Join(args, " "))
	//nolint:gosec // G204 there is no security issue with the arguments passed to syscall.Exec
	err = syscall.Exec(args[0], args, os.Environ())
	require.NoError(t, err)
}
