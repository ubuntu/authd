package testutils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	goCoverDir     string
	goCoverDirOnce sync.Once
)

// fqdnToPath allows to return the fqdn path for this file relative to go.mod.
func fqdnToPath(path string) (res string, err error) {
	srcPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	d := srcPath
	for d != "/" {
		f, err := os.Open(filepath.Clean(filepath.Join(d, "go.mod")))
		if err != nil {
			d = filepath.Dir(d)
			continue
		}
		defer func() {
			err = errors.Join(err, f.Close())
		}()

		r := bufio.NewReader(f)
		l, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}

		if !strings.HasPrefix(l, "module ") {
			return "", fmt.Errorf("go.mod doesn't contain a module declaration: %s", l)
		}

		prefix := strings.TrimSpace(strings.TrimPrefix(l, "module "))
		relpath := strings.TrimPrefix(srcPath, d)
		return filepath.Join(prefix, relpath), nil
	}

	return "", fmt.Errorf("can't find go.mod for %s", srcPath)
}

// CoverDirEnv returns the cover dir env variable to run a go binary, if coverage is enabled.
func CoverDirEnv() string {
	if CoverDirForTests() == "" {
		return ""
	}
	return fmt.Sprintf("GOCOVERDIR=%s", CoverDirForTests())
}

// AppendCovEnv returns the env needed to enable coverage when running a go binary,
// if coverage is enabled.
func AppendCovEnv(env []string) []string {
	coverDir := CoverDirEnv()
	if coverDir == "" {
		return env
	}
	return append(env, coverDir)
}

// CoverDirForTests parses the test arguments and return the cover profile directory,
// if coverage is enabled.
func CoverDirForTests() string {
	goCoverDirOnce.Do(func() {
		if testing.CoverMode() == "" {
			return
		}

		for _, arg := range os.Args {
			if !strings.HasPrefix(arg, "-test.gocoverdir=") {
				continue
			}
			goCoverDir = strings.TrimPrefix(arg, "-test.gocoverdir=")
		}
	})

	return goCoverDir
}

// writeGoCoverageLine writes given line in go coverage format to w.
func writeGoCoverageLine(t *testing.T, w io.Writer, file string, lineNum, lineLength int, covered string) {
	t.Helper()

	_, err := fmt.Fprintf(w, "%s:%d.1,%d.%d 1 %s\n", file, lineNum, lineNum, lineLength, covered)
	require.NoErrorf(t, err, "Teardown: can't write a write to golang compatible cover file : %v", err)
}
