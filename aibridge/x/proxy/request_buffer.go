package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
)

// requestBuffer gives each reader its own position in the buffered input.
// A reader at the buffered end pulls more input; others can still read the prefix.
type requestBuffer struct {
	source  io.Reader
	readMu  sync.Mutex // Serializes reads from source, not reads of buffered bytes.
	mu      sync.Mutex // Protects payload and err.
	payload bytes.Buffer
	err     error
}

func (b *requestBuffer) getBody() (io.ReadCloser, error) {
	b.mu.Lock()
	empty := b.payload.Len() == 0 && errors.Is(b.err, io.EOF)
	b.mu.Unlock()
	if empty {
		return http.NoBody, nil
	}
	// The server owns the input body; closing one reader must not close it.
	return io.NopCloser(&requestBufferReader{buffer: b}), nil
}

type requestBufferReader struct {
	buffer *requestBuffer
	offset int
}

func (r *requestBufferReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b := r.buffer
	n, err := b.readBuffered(p, r.offset)
	if n == 0 && err == nil {
		b.readMu.Lock()
		defer b.readMu.Unlock()
		// Another reader may have extended the buffer while we waited.
		n, err = b.readBuffered(p, r.offset)
		if n == 0 && err == nil {
			n, err = io.TeeReader(b.source, b).Read(p)
			b.mu.Lock()
			b.err = err
			b.mu.Unlock()
		}
	}
	r.offset += n
	return n, err
}

func (b *requestBuffer) readBuffered(p []byte, offset int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := copy(p, b.payload.Bytes()[offset:])
	if offset+n == b.payload.Len() {
		return n, b.err
	}
	return n, nil
}

func (b *requestBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.payload.Write(p)
}
