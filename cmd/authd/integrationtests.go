//go:build integrationtests

package main

import (
	"os"

	"github.com/ubuntu/authd/internal/services/authorizer/authorizertests"
)

// load any behaviour modifiers from env variable.
func init() {
	if os.Getenv("AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT") != "" {
		authorizertests.DefaultCurrentUserAsRoot()
	}
}
