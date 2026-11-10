// Package replaybody retains the bytes forwarded from an HTTP body so other
// consumers can process them after forwarding.
package replaybody

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// maxSourceRead bounds one source read.
const maxSourceRead = 32 << 10

// TeeBuffer retains every byte read from source. Only the replayBody that
// NewRequest and NewResponse install reads the source. The caller must limit
// the body size and call Close when finished.
// TeeBuffer is safe for concurrent use.
type TeeBuffer struct {
	source io.ReadCloser

	mu            sync.Mutex // Guards data, err, and readingSource.
	notify        *sync.Cond // Signals new data, completion, or a closed reader.
	data          []byte
	err           error         // Terminal state. Bare io.EOF means the source completed.
	readingSource bool          // A source read is in flight.
	done          chan struct{} // Closed by finish. data and err do not change afterwards.

	closeOnce sync.Once
	closeErr  error
}

// NewRequest replaces r.Body and r.GetBody with a replayBody and returns the connected buffer.
// HTTP/1 handlers must enable full duplex, or the server will drain the body before an early reply.
func NewRequest(r *http.Request) *TeeBuffer {
	// HTTP/2 uses ContentLength == 0 instead of nil or http.NoBody.
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		r.Body = http.NoBody
		r.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		return newBuffer(http.NoBody)
	}
	// The server closes the original body after the handler returns.
	// Closing the buffer must not close the original body.
	b := newBuffer(io.NopCloser(r.Body))
	r.Body = b.newBody()
	r.GetBody = func() (io.ReadCloser, error) { return b.newBody(), nil }
	return b
}

// NewResponse replaces response Body with a replayBody and returns the connected buffer.
// It takes ownership of the original Body and closes it when the buffer is closed.
func NewResponse(r *http.Response) *TeeBuffer {
	if r.Body == nil || r.Body == http.NoBody {
		r.Body = http.NoBody
		return newBuffer(http.NoBody)
	}
	b := newBuffer(r.Body)
	r.Body = b.newBody()
	return b
}

// newBuffer creates TeeBuffer for the given source.
func newBuffer(source io.ReadCloser) *TeeBuffer {
	b := &TeeBuffer{source: source, done: make(chan struct{})}
	b.notify = sync.NewCond(&b.mu)
	if source == http.NoBody {
		b.finish(io.EOF)
	}
	return b
}

// Wait returns the retained bytes once capture finishes. A nil error means
// the source completed, io.ErrClosedPipe means Close interrupted it, and any
// other error came from the source. The bytes are read-only and stay valid
// after Close. Canceling ctx cancels only this wait.
func (b *TeeBuffer) Wait(ctx context.Context) ([]byte, error) {
	select {
	case <-b.done:
		err := b.err
		if err == io.EOF { //nolint:errorlint // Only bare EOF means completion.
			err = nil
		}
		// Clamp capacity so appending to the result allocates new storage.
		return b.data[:len(b.data):len(b.data)], err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// teeRead reads from the source until it makes progress, appends read
// bytes to the buffer and notifies waiting readers. It always ends in
// new bytes or the terminal state.
func (b *TeeBuffer) teeRead(size int) {
	var n int
	var err error
	p := make([]byte, min(size, maxSourceRead))

	for n == 0 && err == nil {
		n, err = b.source.Read(p)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.readingSource = false
	if b.err == nil {
		b.data = append(b.data, p[:n]...)
		if err != nil {
			b.finish(err)
			return
		}
	}
	b.notify.Broadcast()
}

// Close marks unfinished capture as interrupted and closes the source. It
// does not wait for an in-flight source read, whose bytes are dropped. It
// returns the source close error, cached for later calls.
func (b *TeeBuffer) Close() error {
	b.mu.Lock()
	if b.err == nil {
		b.finish(io.ErrClosedPipe)
	}
	b.mu.Unlock()
	b.closeOnce.Do(func() { b.closeErr = b.source.Close() })
	return b.closeErr
}

// finish records the terminal capture result and wakes readers.
// The caller must hold b.mu unless the buffer is still private to the constructor.
func (b *TeeBuffer) finish(err error) {
	b.err = err
	close(b.done)
	b.notify.Broadcast()
}

// reader reads retained bytes only.
// Close unblocks its pending reads without affecting capture.
type reader struct {
	buffer *TeeBuffer
	closed bool // Guarded by TeeBuffer.mu.
	offset int  // Guarded by TeeBuffer.mu.
}

func (r *reader) Read(p []byte) (int, error) {
	b := r.buffer
	b.mu.Lock()
	defer b.mu.Unlock()

	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}

	for !r.closed && r.offset == len(b.data) && b.err == nil {
		b.notify.Wait()
	}

	if r.closed {
		return 0, io.ErrClosedPipe
	}

	n := copy(p, b.data[r.offset:])
	r.offset += n
	// Report the terminal error only after the last retained byte.
	if r.offset == len(b.data) {
		return n, b.err
	}
	return n, nil
}

// Close invalidates only this reader or replayBody.
// The buffer and source are not affected.
func (r *reader) Close() error {
	b := r.buffer
	b.mu.Lock()
	defer b.mu.Unlock()
	r.closed = true
	b.notify.Broadcast()
	return nil
}

// replayBody is a reader that reads from the source when it runs out of buffered
// bytes. Bytes read from source are teed into the buffer and waiting readers are
// notified about updates.
type replayBody struct {
	reader
}

func (b *TeeBuffer) newBody() io.ReadCloser {
	return &replayBody{reader{buffer: b}}
}

// Read reads from the buffer and, once the buffer is exhausted, from the source.
func (t *replayBody) Read(p []byte) (int, error) {
	b := t.buffer
	b.mu.Lock()
	if !t.closed && len(p) > 0 && t.offset == len(b.data) && b.err == nil && !b.readingSource {
		b.readingSource = true
		// Running teeRead in a goroutine so that Close can
		// unblock Read without waiting for the source read to complete.
		go b.teeRead(len(p))
	}
	b.mu.Unlock()
	return t.reader.Read(p)
}
