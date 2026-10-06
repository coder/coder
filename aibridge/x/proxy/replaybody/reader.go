package replaybody

import (
	"io"
	"sync/atomic"
)

type reader struct {
	// Close drops the buffer reference to release the captured payload.
	// Read loads it before taking Buffer.mu, so it must be atomic.
	buffer atomic.Pointer[Buffer]
	offset int // Protected by Buffer.mu.
}

// NewReader returns an independent reader at the first byte, suitable for
// http.Request.Body or GetBody. Closing it unblocks pending reads without
// stopping capture or other readers. Retained bytes are followed by the capture
// error. Only bare io.EOF signals successful completion.
func (b *Buffer) NewReader() (io.ReadCloser, error) {
	r := &reader{}
	r.buffer.Store(b)
	return r, nil
}

func (r *reader) Read(p []byte) (int, error) {
	b := r.buffer.Load()
	if b == nil {
		return 0, io.ErrClosedPipe
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// Close may have run while we waited for the lock.
	if r.buffer.Load() == nil {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	for r.buffer.Load() != nil && r.offset == len(b.data) && b.captureErr == nil {
		b.readNotify.Wait() // Wait for new data of capture end
	}
	if r.buffer.Load() == nil {
		return 0, io.ErrClosedPipe
	}
	n := copy(p, b.data[r.offset:])
	r.offset += n
	// Report the terminal error only after consuming buffered bytes.
	if r.offset == len(b.data) {
		return n, b.captureErr
	}
	return n, nil
}

func (r *reader) Close() error {
	if b := r.buffer.Load(); b != nil {
		b.mu.Lock()
		b.readNotify.Broadcast()
		r.buffer.Store(nil)
		b.mu.Unlock()
	}
	return nil
}
