package gdm

import (
	"os"
	"testing"

	"github.com/ubuntu/authd/pam/internal/pam_test"
)

func TestMain(m *testing.M) {
	exit := m.Run()
	pam_test.MaybeDoLeakCheck()
	os.Exit(exit)
}
