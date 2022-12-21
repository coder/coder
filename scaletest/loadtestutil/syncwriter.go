package loadtestutil

import (
	"io"
	"sync"
)

// SyncWriter wraps an io.Writer in a sync.Mutex.
type SyncWriter struct {
	mut *sync.Mutex
	w   io.Writer
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
	return sw.w.Write(p)
}
