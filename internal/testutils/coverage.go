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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

var (
	goMainCoverProfile     string
	goMainCoverProfileOnce sync.Once

	coveragesToMerge   []string
	coveragesToMergeMu sync.Mutex
)

// TrackTestCoverage starts tracking coverage in a dedicated file based on current test name.
// This file will be merged to the current coverage main file.
// It’s up to the test use the returned path to file golang-compatible cover format content.
// To collect all coverages, then MergeCoverages() should be called after m.Run().
// If coverage is not enabled, nothing is done.
func TrackTestCoverage(t *testing.T) (testCoverFile string) {
	t.Helper()

	coverProfile := CoverProfile()
	if coverProfile == "" {
		return ""
	}

	coverAbsPath, err := filepath.Abs(coverProfile)
	require.NoError(t, err, "Setup: can't transform go cover profile to absolute path")

	testCoverFile = fmt.Sprintf("%s.%s", coverAbsPath, strings.ReplaceAll(
		strings.ReplaceAll(t.Name(), "/", "_"),
		"\\", "_"))

	MarkCoverageForMerging(testCoverFile)

	return testCoverFile
}

// MarkCoverageForMerging marks the coverage file to be merged to the main go cover profile.
func MarkCoverageForMerging(coverFile string) {
	coveragesToMergeMu.Lock()
	defer coveragesToMergeMu.Unlock()
	if slices.Contains(coveragesToMerge, coverFile) {
		panic(fmt.Sprintf("Trying to adding a second time %q to the list of file to cover. This will create some overwrite and thus, should be only called once", coverFile))
	}
	coveragesToMerge = append(coveragesToMerge, coverFile)
}

// MergeCoverages append all coverage files marked for merging to main Go Cover Profile.
// This has to be called after m.Run() in TestMain so that the main go cover profile is created.
// This has no action if profiling is not enabled.
func MergeCoverages() error {
	coveragesToMergeMu.Lock()
	defer coveragesToMergeMu.Unlock()
	var err error
	for _, cov := range coveragesToMerge {
		err = errors.Join(err, appendToFile(cov, goMainCoverProfile))
	}

	if err != nil {
		err = fmt.Errorf("teardown: couldn't inject coverages into the golang one: %w", err)
	}
	coveragesToMerge = nil
	return err
}

// WantCoverage returns true if coverage was requested in test.
func WantCoverage() bool {
	for _, arg := range os.Args {
		if !strings.HasPrefix(arg, "-test.coverprofile=") {
			continue
		}
		return true
	}
	return false
}

// appendToFile appends src to the dst coverprofile file at the end.
func appendToFile(src, dst string) error {
	f, err := os.Open(filepath.Clean(src))
	if err != nil {
		return fmt.Errorf("can't open coverage file: %w", err)
	}
	defer f.Close()

	d, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("can't open golang cover profile file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "mode: ") {
			continue
		}
		if _, err := d.Write([]byte(scanner.Text() + "\n")); err != nil {
			return fmt.Errorf("can't write to golang cover profile file: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error while scanning golang cover profile file: %w", err)
	}
	return nil
}

// fqdnToPath allows to return the fqdn path for this file relative to go.mod.
func fqdnToPath(t *testing.T, path string) string {
	t.Helper()

	srcPath, err := filepath.Abs(path)
	require.NoError(t, err, "Setup: can't calculate absolute path")

	d := srcPath
	for d != "/" {
		f, err := os.Open(filepath.Clean(filepath.Join(d, "go.mod")))
		if err != nil {
			d = filepath.Dir(d)
			continue
		}
		defer func() { assert.NoError(t, f.Close(), "Setup: can’t close go.mod") }()

		r := bufio.NewReader(f)
		l, err := r.ReadString('\n')
		require.NoError(t, err, "can't read go.mod first line")
		if !strings.HasPrefix(l, "module ") {
			t.Fatal(`Setup: failed to find "module" line in go.mod`)
		}

		prefix := strings.TrimSpace(strings.TrimPrefix(l, "module "))
		relpath := strings.TrimPrefix(srcPath, d)
		return filepath.Join(prefix, relpath)
	}

	t.Fatal("failed to find go.mod")
	return ""
}

// CoverProfile parses the arguments to find the cover profile file.
func CoverProfile() string {
	goMainCoverProfileOnce.Do(func() {
		for _, arg := range os.Args {
			if !strings.HasPrefix(arg, "-test.coverprofile=") {
				continue
			}
			goMainCoverProfile = strings.TrimPrefix(arg, "-test.coverprofile=")
		}
	})
	return goMainCoverProfile
}

// writeGoCoverageLine writes given line in go coverage format to w.
func writeGoCoverageLine(t *testing.T, w io.Writer, file string, lineNum, lineLength int, covered string) {
	t.Helper()

	_, err := w.Write([]byte(fmt.Sprintf("%s:%d.1,%d.%d 1 %s\n", file, lineNum, lineNum, lineLength, covered)))
	require.NoErrorf(t, err, "Teardown: can't write a write to golang compatible cover file : %v", err)
}
