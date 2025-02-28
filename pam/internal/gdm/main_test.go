package gdm

import (
	"os"
	"testing"

	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)

	exit := m.Run()
	pam_test.MaybeDoLeakCheck()
	os.Exit(exit)
}
