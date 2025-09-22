// Package testlog provides utilities for logging test output.
package testlog

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

type runWithTimingOptions struct {
	doNotSetStdoutAndStderr         bool
	onlyPrintStdoutAndStderrOnError bool
}

// RunWithTimingOption is a function that configures the RunWithTiming function.
type RunWithTimingOption func(options *runWithTimingOptions)

// DoNotSetStdoutAndStderr prevents RunWithTiming from setting the stdout and stderr of the given command.
// By default, RunWithTiming sets stdout and stderr to the test's output.
func DoNotSetStdoutAndStderr() RunWithTimingOption {
	return func(options *runWithTimingOptions) {
		options.doNotSetStdoutAndStderr = true
	}
}

// OnlyPrintStdoutAndStderrOnError makes RunWithTiming only print the stdout and stderr of the given command if it fails.
func OnlyPrintStdoutAndStderrOnError() RunWithTimingOption {
	return func(options *runWithTimingOptions) {
		options.onlyPrintStdoutAndStderrOnError = true
	}
}

// RunWithTiming runs the given command while logging its duration with the provided message.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func RunWithTiming(t *testing.T, msg string, cmd *exec.Cmd, options ...RunWithTimingOption) error {
	if t != nil {
		t.Helper()
	}

	w := testWriterOrStderr(t)

	opts := runWithTimingOptions{}
	for _, f := range options {
		f(&opts)
	}

	if opts.doNotSetStdoutAndStderr && opts.onlyPrintStdoutAndStderrOnError {
		panic("onlyPrintStdoutAndStderrOnError and doNotSetStdoutAndStderr cannot be used together")
	}

	if !opts.doNotSetStdoutAndStderr && !opts.onlyPrintStdoutAndStderrOnError {
		cmd.Stdout = w
		cmd.Stderr = w
	}

	LogCommand(t, msg, cmd)
	start := time.Now()

	var err error
	var out []byte
	if opts.onlyPrintStdoutAndStderrOnError {
		out, err = cmd.CombinedOutput()
		if err != nil {
			_, _ = w.Write(out)
		}
	} else {
		err = cmd.Run()
	}
	duration := time.Since(start)

	if err != nil {
		fmt.Fprintf(w, redSeparatorf("%s failed in %.3fs with %v", msg, duration.Seconds(), err)+"\n")
	} else {
		fmt.Fprintf(w, separatorf("%s finished in %.3fs", msg, duration.Seconds())+"\n")
	}

	return err
}

// LogCommand logs the given command to stderr.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func LogCommand(t *testing.T, msg string, cmd *exec.Cmd) {
	if t != nil {
		t.Helper()
	}
	w := testWriterOrStderr(t)

	sep := "----------------------------------------"
	fmt.Fprintf(w, "\n"+separator(msg)+"command: %s\n%s\nenvironment: %s\n%s\n", cmd.String(), sep, cmd.Env, sep)
}

// LogStartSeparatorf logs a separator to stderr with the given formatted message.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func LogStartSeparatorf(t *testing.T, s string, args ...any) {
	if t != nil {
		t.Helper()
	}
	w := testWriterOrStderr(t)

	fmt.Fprintf(w, "\n"+separatorf(s, args...))
}

// LogStartSeparator logs a separator to stderr with the given message.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func LogStartSeparator(t *testing.T, args ...any) {
	if t != nil {
		t.Helper()
	}

	LogStartSeparatorf(t, fmt.Sprint(args...))
}

// LogEndSeparatorf logs a separator to stderr with the given formatted message.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func LogEndSeparatorf(t *testing.T, s string, args ...any) {
	if t != nil {
		t.Helper()
	}
	w := testWriterOrStderr(t)

	fmt.Fprintf(w, separatorf(s, args...)+"\n")
}

// LogEndSeparator logs a separator to stderr with the given message.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func LogEndSeparator(t *testing.T, args ...any) {
	if t != nil {
		t.Helper()
	}

	LogEndSeparatorf(t, fmt.Sprint(args...))
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
