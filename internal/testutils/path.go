package testutils

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ProjectRoot returns the absolute path to the project root.
func ProjectRoot() string {
	// p is the path to the current file, in this case -> {PROJECT_ROOT}/internal/testutils/path.go
	_, p, _, _ := runtime.Caller(0)

	// Walk up the tree to get the path of the project root
	l := strings.Split(p, "/")

	// Ignores the last 3 elements -> /internal/testutils/path.go
	l = l[:len(l)-3]

	// strings.Split removes the first "/" that indicated an AbsPath, so we append it back in the final string.
	return "/" + filepath.Join(l...)
}

// MakeReadOnly makes dest read only and restore permission on cleanup.
func MakeReadOnly(t *testing.T, dest string) {
	t.Helper()

	// Get current dest permissions
	fi, err := os.Stat(dest)
	require.NoError(t, err, "Cannot stat %s", dest)
	mode := fi.Mode()

	var perms fs.FileMode = 0444
	if fi.IsDir() {
		perms = 0555
	}
	err = os.Chmod(dest, perms)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := os.Stat(dest)
		if errors.Is(err, os.ErrNotExist) {
			return
		}

		err = os.Chmod(dest, mode)
		require.NoError(t, err)
	})
}
