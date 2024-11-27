package localgroups

import "github.com/ubuntu/authd/internal/testsdetection"

var originalDefaultOptions = defaultOptions

// Z_ForTests_RestoreDefaultOptions restores the defaultOptions to their original values.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_RestoreDefaultOptions() {
	testsdetection.MustBeTesting()

	defaultOptions = originalDefaultOptions
}

// Z_ForTests_SetGroupPath sets the groupPath for the defaultOptions.
// Tests using this can't be run in parallel.
// Call Z_ForTests_RestoreDefaultOptions to restore the original value.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_SetGroupPath(groupPath string) {
	testsdetection.MustBeTesting()

	defaultOptions.groupPath = groupPath
}

// Z_ForTests_SetGpasswdCmd sets the gpasswdCmd for the defaultOptions.
// Tests using this can't be run in parallel.
// Call Z_ForTests_RestoreDefaultOptions to restore the original value.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_SetGpasswdCmd(gpasswdCmd []string) {
	testsdetection.MustBeTesting()

	defaultOptions.gpasswdCmd = gpasswdCmd
}
