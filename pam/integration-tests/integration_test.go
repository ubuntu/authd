package main_test

import (
	"log"
	"os"
	"testing"

	"github.com/ubuntu/authd/internal/testutils"
)

const authdCurrentUserRootEnvVariableContent = "AUTHD_INTEGRATIONTESTS_CURRENT_USER_AS_ROOT=1"

var daemonPath string

func TestMain(m *testing.M) {
	// Needed to skip the test setup when running the gpasswd mock.
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	var cleanup func()
	var err error
	daemonPath, cleanup, err = testutils.BuildDaemon()
	if err != nil {
		log.Printf("Setup: failed to build daemon: %v", err)
		os.Exit(1)
	}
	defer cleanup()

	m.Run()
}
