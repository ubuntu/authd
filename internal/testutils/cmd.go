package testutils

import (
	"log"
	"os/exec"
	"strings"
	"time"
)

// RunWithTiming runs the given command while logging its duration with the provided message.
func RunWithTiming(msg string, cmd *exec.Cmd) error {
	log.Printf("%s: %s", msg, strings.Join(cmd.Args, " "))
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	log.Printf("Finished in %.3fs", duration.Seconds())
	return err
}

// CombinedOutputWithTiming runs the given command while logging its duration with the provided message
// and returns its combined standard output and standard error.
func CombinedOutputWithTiming(msg string, cmd *exec.Cmd) ([]byte, error) {
	log.Printf("%s: %s", msg, strings.Join(cmd.Args, " "))
	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)
	log.Printf("Finished in %.3fs", duration.Seconds())
	return out, err
}
