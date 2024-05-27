//go:build integrationtests

package main

import (
	"os"

	permissionstestutils "github.com/ubuntu/authd/internal/services/permissions/testutils"
)

// load any behaviour modifiers from env variable.
func init() {
	if os.Getenv("AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT") != "" {
		permissionstestutils.DefaultCurrentUserAsRoot()
	}
}
