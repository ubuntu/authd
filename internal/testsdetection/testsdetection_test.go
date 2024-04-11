package testsdetection_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testsdetection"
)

var coverDir string

func TestMustBeTestingInTests(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		require.Nil(t, r, "MustBeTesting should not panic as we are running in tests")
	}()

	testsdetection.MustBeTesting()
}

func TestMustBeTestingInProcess(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		integrationtestsTag bool

		wantPanic bool
	}{
		"Pass when called in an integration tests build": {integrationtestsTag: true, wantPanic: false},

		"Error (panics) when called in non tests and no integration tests": {integrationtestsTag: false, wantPanic: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			temp := t.TempDir()
			testBinary := filepath.Join(temp, "testbin")

			buildCmd := []string{"build", "-o", testBinary}
			if tc.integrationtestsTag {
				buildCmd = append(buildCmd, "-tags=integrationtests")
			}
			env := os.Environ()
			if coverDir != "" {
				buildCmd = append(buildCmd, "-cover")
				env = append(env, "GOCOVERDIR="+coverDir)
			}
			buildCmd = append(buildCmd, "testdata/binary.go")

			//nolint:gosec // G204 we are in control of the arguments in our tests.
			out, err := exec.Command("go", buildCmd...).CombinedOutput()
			require.NoErrorf(t, err, "Setup: Could not build test binary: %s", out)

			// Execute our subprocess
			//nolint:gosec // G204 we are in control of the arguments in our tests.
			cmd := exec.Command(testBinary)
			cmd.Env = env
			out, err = cmd.CombinedOutput()

			if tc.wantPanic {
				require.Errorf(t, err, "MustBeTesting should have panicked the subprocess: %s", string(out))
				return
			}
			require.NoErrorf(t, err, "MustBeTesting should have returned without panicking the subprocess", string(out))
		})
	}
}

func TestMain(m *testing.M) {
	if testing.CoverMode() != "" {
		for _, arg := range os.Args {
			if !strings.HasPrefix(arg, "-test.gocoverdir=") {
				continue
			}
			coverDir = strings.TrimPrefix(arg, "-test.gocoverdir=")
		}
	}

	m.Run()
}
