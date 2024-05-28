// Package localgrouptestutils export users test functionalities used by other packages to change cmdline and group file.
package localgrouptestutils

//nolint:gci // We import unsafe as it is needed for go:linkname, but the nolint comment confuses gofmt and it adds
// a blank space between the imports, which creates problems with gci so we need to ignore it.
import (

	//nolint:revive,nolintlint // needed for go:linkname, but only used in tests. nolintlint as false positive then.
	_ "unsafe"

	"github.com/ubuntu/authd/internal/testsdetection"
)

func init() {
	// No import outside of testing environment.
	testsdetection.MustBeTesting()
}

var (
	//go:linkname defaultOptions github.com/ubuntu/authd/internal/users/localgroups.defaultOptions
	defaultOptions struct {
		groupPath    string
		gpasswdCmd   []string
		getUsersFunc func() []string
	}
)

// SetGroupPath sets the groupPath for the defaultOptions.
// Tests using this can't be run in parallel.
func SetGroupPath(groupPath string) {
	defaultOptions.groupPath = groupPath
}

// SetGpasswdCmd sets the gpasswdCmd for the defaultOptions.
// Tests using this can't be run in parallel.
func SetGpasswdCmd(gpasswdCmd []string) {
	defaultOptions.gpasswdCmd = gpasswdCmd
}
