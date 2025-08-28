package testutils

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// RunWithTiming runs the given command while logging its duration with the provided message.
func RunWithTiming(msg string, cmd *exec.Cmd) error {
	LogCommand(msg, cmd)
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, redSeparatorf("%s failed in %.3fs with %v", msg, duration.Seconds(), err)+"\n")
	} else {
		fmt.Fprintf(os.Stderr, separatorf("%s finished in %.3fs", msg, duration.Seconds())+"\n")
	}
	return err
}

// LogCommand logs the given command to stderr.
func LogCommand(msg string, cmd *exec.Cmd) {
	sep := "----------------------------------------"
	fmt.Fprintf(os.Stderr, "\n"+separator(msg)+"command: %s\n%s\nenvironment: %s\n%s\n", cmd.String(), sep, cmd.Env, sep)
}

// LogStartSeparatorf logs a separator to stderr with the given formatted message.
func LogStartSeparatorf(s string, args ...any) {
	fmt.Fprintf(os.Stderr, "\n"+separatorf(s, args...))
}

// LogStartSeparator logs a separator to stderr with the given message.
func LogStartSeparator(args ...any) {
	LogStartSeparatorf(fmt.Sprint(args...))
}

// LogEndSeparatorf logs a separator to stderr with the given formatted message.
func LogEndSeparatorf(s string, args ...any) {
	fmt.Fprintf(os.Stderr, separatorf(s, args...)+"\n")
}

// LogEndSeparator logs a separator to stderr with the given message.
func LogEndSeparator(args ...any) {
	LogEndSeparatorf(fmt.Sprint(args...))
}

// separatorf returns a formatted separator string for logging purposes.
func separatorf(s string, args ...any) string {
	return highCyan("===== " + fmt.Sprintf(s, args...) + " =====\n")
}

// separator returns a separator string for logging purposes.
func separator(args ...any) string {
	return separatorf(fmt.Sprint(args...))
}

func redSeparatorf(s string, args ...any) string {
	return highRed("===== " + fmt.Sprintf(s, args...) + " =====\n")
}

// highCyan returns a string with the given text in high-intensity cyan color for terminal output.
func highCyan(s string) string {
	return fmt.Sprintf("\033[1;36m%s\033[0m", s)
}

// highRed returns a string with the given text in high-intensity red color for terminal output.
func highRed(s string) string {
	return fmt.Sprintf("\033[1;31m%s\033[0m", s)
}
