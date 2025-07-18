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
	execPath, daemonCleanup, err := testutils.BuildDaemon("-tags=withexamplebroker,integrationtests")
	if err != nil {
		log.Printf("Setup: Failed to build authd daemon: %v", err)
		os.Exit(1)
	}
	defer daemonCleanup()
	daemonPath = execPath

	m.Run()
}
