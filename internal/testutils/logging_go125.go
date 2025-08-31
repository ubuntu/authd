//go:build go1.25

package testutils

import (
	"io"
	"os"
	"testing"
)

// NewTestWriter creates a new TestWriter that logs to t.
//
//nolint:thelper // we're not using t in any way that requires the helper annotation
func NewTestWriter(t *testing.T) io.Writer {
	return t.Output()
}

//nolint:thelper // we're not using t in any way that requires the helper annotation
func testWriterOrStderr(t *testing.T) io.Writer {
	if t != nil {
		return t.Output()
	}
	return os.Stderr
}
