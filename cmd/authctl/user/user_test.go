package user_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ubuntu/authd/internal/testutils"
	"github.com/ubuntu/authd/internal/testutils/golden"
)

var authctlPath string

func TestUserCommand(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args             []string
		expectedExitCode int
	}{
		"Usage_message_when_no_args": {expectedExitCode: 0},
		"Help_flag":                  {args: []string{"--help"}, expectedExitCode: 0},

		"Error_on_invalid_command": {args: []string{"invalid-command"}, expectedExitCode: 1},
		"Error_on_invalid_flag":    {args: []string{"--invalid-flag"}, expectedExitCode: 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			//nolint:gosec // G204 it's safe to use exec.Command with a variable here
			cmd := exec.Command(authctlPath, append([]string{"user"}, tc.args...)...)
			t.Logf("Running command: %s", strings.Join(cmd.Args, " "))
			outputBytes, err := cmd.CombinedOutput()
			output := string(outputBytes)
			exitCode := cmd.ProcessState.ExitCode()

			if tc.expectedExitCode == 0 && err != nil {
				t.Logf("Command output:\n%s", output)
				t.Errorf("Expected no error, but got: %v", err)
			}

			if exitCode != tc.expectedExitCode {
				t.Logf("Command output:\n%s", output)
				t.Errorf("Expected exit code %d, got %d", tc.expectedExitCode, exitCode)
			}

			golden.CheckOrUpdate(t, output)
		})
	}
}

func TestMain(m *testing.M) {
	var cleanup func()
	var err error
	authctlPath, cleanup, err = testutils.BuildAuthctl()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Setup: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	m.Run()
}
