package loadtestutil

import (
	"io"
	"sync"
)

// SyncWriter wraps an io.Writer in a sync.Mutex.
type SyncWriter struct {
	mut    *sync.Mutex
	w      io.Writer
	closed bool
}

func NewSyncWriter(w io.Writer) *SyncWriter {
	return &SyncWriter{
		mut: &sync.Mutex{},
		w:   w,
	}
}

// Write implements io.Writer.
func (sw *SyncWriter) Write(p []byte) (n int, err error) {
	sw.mut.Lock()
	defer sw.mut.Unlock()
	if sw.closed {
		return -1, io.ErrClosedPipe
	}
	return sw.w.Write(p)
}

func (sw *SyncWriter) Close() error {
	sw.mut.Lock()
	defer sw.mut.Unlock()
	sw.closed = true
	return nil
}
