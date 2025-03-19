// TiCS: disabled // Should only be built when running integration tests.

//go:build integrationtests

package main

import (
	"os"
	"strings"

	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/users/localentries"
)

// load any behaviour modifiers from env variable.
func init() {
	if os.Getenv("AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT") != "" {
		permissions.Z_ForTests_DefaultCurrentUserAsRoot()
	}

	gpasswdArgs := os.Getenv("AUTHD_INTEGRATIONTESTS_GPASSWD_ARGS")
	grpFilePath := os.Getenv("AUTHD_INTEGRATIONTESTS_GPASSWD_GRP_FILE_PATH")
	if gpasswdArgs == "" || grpFilePath == "" {
		panic("AUTHD_INTEGRATIONTESTS_GPASSWD_ARGS and AUTHD_INTEGRATIONTESTS_GPASSWD_GRP_FILE_PATH must be set")
	}
	localentries.Z_ForTests_SetGpasswdCmd(strings.Split(gpasswdArgs, " "))
	localentries.Z_ForTests_SetGroupPath(grpFilePath)
}
