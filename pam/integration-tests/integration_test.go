package main_test

import (
	"log"
	"os"
	"testing"

	"github.com/ubuntu/authd/internal/testutils"
)

func TestMain(m *testing.M) {
	// Needed to skip the test setup when running the gpasswd mock.
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		os.Exit(m.Run())
	}

	execPath, daemonCleanup, err := testutils.BuildDaemon("-tags=withexamplebroker,integrationtests")
	if err != nil {
		log.Printf("Setup: Failed to build authd daemon: %v", err)
		os.Exit(1)
	}
	defer daemonCleanup()
	daemonPath = execPath

	m.Run()
}
