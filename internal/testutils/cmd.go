package testutils

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/ubuntu/authd/log"
)

// RunWithTiming runs the given command while logging its duration with the provided message.
func RunWithTiming(msg string, cmd *exec.Cmd) error {
	log.Infof(context.Background(), "%s: %s", msg, strings.Join(cmd.Args, " "))
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	log.Infof(context.Background(), "Finished in %.3fs", duration.Seconds())
	return err
}
