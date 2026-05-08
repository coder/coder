package vncproxy

import (
	"io"
	"sync"
)

// tracingReader wraps an io.Reader and remembers the last tailSize bytes
// seen by Read calls. It also tracks the running byte offset so error
// messages can pinpoint where in the stream the parser desynced. Used
// only when called from BicopyDropClipboard with debug tracing on. It
// is safe for sequential use by a single goroutine; the per-direction
// pumps each own one instance and never share readers across
// goroutines.
type tracingReader struct {
	r        io.Reader
	mu       sync.Mutex
	tail     []byte
	tailSize int
	offset   int64
}

const traceTailSize = 96

func newTracingReader(r io.Reader) *tracingReader {
	return &tracingReader{r: r, tailSize: traceTailSize}
}

func (t *tracingReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		t.mu.Lock()
		t.offset += int64(n)
		t.tail = append(t.tail, p[:n]...)
		if len(t.tail) > t.tailSize {
			t.tail = t.tail[len(t.tail)-t.tailSize:]
		}
		t.mu.Unlock()
	}
	return n, err
}

// snapshot returns the current byte offset and a copy of the trailing
// bytes most recently seen on this side of the stream. Callers should
// log these together when surfacing parser errors.
func (t *tracingReader) snapshot() (int64, []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]byte, len(t.tail))
	copy(out, t.tail)
	return t.offset, out
}
