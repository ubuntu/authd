package testutils

import (
	"bytes"
	"sync"
)

// SyncBuffer is a mutex-protected buffer to avoid data races.
type SyncBuffer struct {
	mu  sync.RWMutex
	buf *bytes.Buffer
}

// NewSyncBuffer creates a new SyncBuffer.
func NewSyncBuffer() *SyncBuffer {
	return &SyncBuffer{buf: &bytes.Buffer{}}
}

func (s *SyncBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// Bytes returns the buffer content.
func (s *SyncBuffer) Bytes() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buf.Bytes()
}
