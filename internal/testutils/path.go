package testutils

import (
	"path/filepath"
	"runtime"
	"strings"
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
