package testutils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestWriter is an io.Writer that sends its output to a testing.T via t.Log,
// ensuring logs are attributed to the correct test.
// It buffers partial writes until a newline is seen.
type TestWriter struct {
	t   *testing.T
	mu  sync.Mutex
	buf bytes.Buffer
}

// NewTestWriter creates a new TestWriter that logs to t.
func NewTestWriter(t *testing.T) *TestWriter {
	return &TestWriter{t: t}
}

// Write implements io.Writer. It buffers until a newline is seen,
// then flushes complete lines to t.Log. Remainders are held until
// the next Write call.
func (w *TestWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err = w.buf.Write(p)
	for {
		line, errLine := w.buf.ReadString('\n')
		if errLine == io.EOF {
			// leftover data without newline — keep it for next Write
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		// Trim the newline, let t.Log add its own
		w.t.Log(strings.TrimSuffix(line, "\n"))
	}
	return
}

// RunWithTiming runs the given command while logging its duration with the provided message.
//
//nolint:thelper // we do call t.Helper() if t is not nil
func RunWithTiming(t *testing.T, msg string, cmd *exec.Cmd) error {
	if t != nil {
		t.Helper()
	}
	w := testWriterOrStderr(t)

	LogCommand(t, msg, cmd)
	start := time.Now()
	err := cmd.Run()
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

func testWriterOrStderr(t *testing.T) io.Writer {
	if t != nil {
		return NewTestWriter(t)
	}
	return os.Stderr
}
