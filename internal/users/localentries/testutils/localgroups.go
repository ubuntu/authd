// Package localgrouptestutils export users test functionalities used by other packages to change cmdline and group file.
package localgrouptestutils

import (
	"github.com/ubuntu/authd/internal/testsdetection"
)

func init() {
	// No import outside of testing environment.
	testsdetection.MustBeTesting()
}

var (
	defaultOptions struct {
		groupPath  string
		gpasswdCmd []string
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
