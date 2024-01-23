// Package tests export users test functionalities used by other packages to change cmdline and group file.
package tests

//nolint:gci // We import unsafe as it is needed for go:linkname, but the nolint comment confuses gofmt and it adds
// a blank space between the imports, which creates problems with gci so we need to ignore it.
import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	//nolint:revive,nolintlint // needed for go:linkname, but only used in tests. nolintlint as false positive then.
	_ "unsafe"

	"github.com/stretchr/testify/require"
)

var (
	//go:linkname defaultOptions github.com/ubuntu/authd/internal/users/localgroups.defaultOptions
	defaultOptions struct {
		groupPath    string
		gpasswdCmd   []string
		getUsersFunc func() []string
	}
)

// OverrideDefaultOptions allow to change groupPath and gpasswdCmd without using options.
// This is used for tests when we don’t have access to the users object directly, like integration tests.
// Tests using this can't be run in parallel.
func OverrideDefaultOptions(t *testing.T, groupPath string, gpasswdCmd []string, getUsersFunc func() []string) {
	t.Helper()

	origin := defaultOptions
	t.Cleanup(func() { defaultOptions = origin })

	defaultOptions.groupPath = groupPath
	defaultOptions.gpasswdCmd = gpasswdCmd

	if getUsersFunc != nil {
		defaultOptions.getUsersFunc = getUsersFunc
	}
}

// IdempotentGPasswdOutput sort and trim spaces around mock gpasswd output.
func IdempotentGPasswdOutput(t *testing.T, cmdsFilePath string) string {
	t.Helper()

	d, err := os.ReadFile(cmdsFilePath)
	require.NoError(t, err, "Teardown: could not read dest trace file")

	// need to sort out all operations
	ops := strings.Split(string(d), "\n")
	slices.Sort(ops)
	content := strings.TrimSpace(strings.Join(ops, "\n"))

	return content
}

// Mockgpasswd is the gpasswd mock.
func Mockgpasswd(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_DEST") == "" {
		return
	}

	args := os.Args
	for len(args) > 0 {
		if args[0] != "--" {
			args = args[1:]
			continue
		}
		args = args[1:]
		break
	}

	d, err := os.ReadFile(os.Getenv("GO_WANT_HELPER_PROCESS_GROUPFILE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mock: error reading group file: %v", err)
		os.Exit(1)
	}

	// Error if the group is not in the groupfile (we don’t handle substrings in the mock)
	group := args[len(args)-1]
	if !strings.Contains(string(d), group+":") {
		fmt.Fprintf(os.Stderr, "Error: %s in not in the group file", group)
		os.Exit(3)
	}

	// Other error
	if slices.Contains(args, "gpasswdfail") {
		fmt.Fprint(os.Stderr, "Error requested in mock")
		os.Exit(1)
	}

	dest := os.Getenv("GO_WANT_HELPER_PROCESS_DEST")
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mock: error opening file in append mode: %v", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Write([]byte(strings.Join(args, " ") + "\n")); err != nil {
		fmt.Fprintf(os.Stderr, "Mock: error while writing in file: %v", err)
		f.Close()
		os.Exit(1)
	}
}
