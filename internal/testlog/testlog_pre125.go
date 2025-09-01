//go:build !go1.25

package testlog

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
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
//
//nolint:thelper // we're not using t in any way that requires the helper annotation'
func NewTestWriter(t *testing.T) *TestWriter {
	return &TestWriter{t: t}
}

// Write implements io.Writer. It buffers until a newline is seen,
// then flushes complete lines to t.Log. Remainders are held until
// the next Write call.
func (w *TestWriter) Write(p []byte) (n int, err error) {
	w.t.Helper()

	w.mu.Lock()
	defer w.mu.Unlock()

	n, err = w.buf.Write(p)
	for {
		line, errLine := w.buf.ReadString('\n')
		if errors.Is(errLine, io.EOF) {
			// leftover data without newline — keep it for next Write
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		// Trim the newline, let t.Log add its own
		w.t.Log(strings.TrimSuffix(line, "\n"))
	}

	return n, err
}

// testWriterOrStderr returns a TestWriter if t is non-nil, or os.Stderr otherwise.
//
//nolint:thelper // we're not using t in any way that requires the helper annotation'
func testWriterOrStderr(t *testing.T) io.Writer {
	if t != nil {
		return NewTestWriter(t)
	}
	return os.Stderr
}
