// Package replaybody captures a streaming body once for independent readers.
package replaybody

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// Buffer retains one append-only payload. Capture starts at construction and
// proceeds independently of readers until EOF, failure, or Stop.
// The caller must limit the source size and call Close to release the source.
// Buffer is safe for concurrent use.
type Buffer struct {
	source      io.ReadCloser
	mu          sync.Mutex
	data        []byte
	err         error
	stop        error
	ready       *sync.Cond
	done        chan struct{}
	closeSource sync.Once
	closeErr    error
}

// New takes ownership of source and starts capture without blocking. Source.Close
// must be safe concurrently with Read and make outstanding and subsequent reads
// return promptly. A source whose Close drains or waits needs an abort adapter.
func New(source io.ReadCloser) *Buffer {
	b := &Buffer{
		source: source,
		done:   make(chan struct{}),
	}
	b.ready = sync.NewCond(&b.mu)
	go b.capture()
	return b
}

// NewReader returns an independent cursor from the beginning, suitable for
// http.Request.Body or GetBody. Closing a reader affects neither capture nor
// other readers. Readers created after capture ends replay the retained bytes
// followed by the same terminal error.
func (b *Buffer) NewReader() (io.ReadCloser, error) {
	r := &reader{}
	r.buffer.Store(b)
	return r, nil
}

// Wait returns the shared, sealed payload and its terminal error, with EOF
// reported as nil. A non-nil error means capture is partial. The returned bytes
// must not be modified and remain valid after Close. Canceling ctx cancels only
// this wait, not capture.
func (b *Buffer) Wait(ctx context.Context) ([]byte, error) {
	select {
	case <-b.done:
		b.mu.Lock()
		defer b.mu.Unlock()
		err := b.err
		if err == io.EOF { //nolint:errorlint // Only bare EOF denotes a complete io.Reader stream.
			err = nil
		}
		return b.data[:len(b.data):len(b.data)], err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Stop closes the source once and joins capture, retaining any bytes returned by
// an in-flight read. The first cause wins unless capture already ended. A nil
// cause or an EOF cause becomes io.ErrClosedPipe. It returns the source's Close
// error.
func (b *Buffer) Stop(cause error) error {
	if cause == nil || errors.Is(cause, io.EOF) {
		cause = io.ErrClosedPipe
	}
	b.mu.Lock()
	if b.err == nil && b.stop == nil {
		b.stop = cause
	}
	b.mu.Unlock()
	b.closeSource.Do(func() { b.closeErr = b.source.Close() })
	<-b.done
	return b.closeErr
}

// Close stops unfinished capture with io.ErrClosedPipe and releases the source.
// It does not invalidate retained payloads or readers.
func (b *Buffer) Close() error { return b.Stop(io.ErrClosedPipe) }

// finish is called with mu held, after all captured bytes have been published.
func (b *Buffer) finish(err error) {
	b.err = err
	b.ready.Broadcast()
	close(b.done)
}

func (b *Buffer) capture() {
	// Hide WriterTo so source optimizations cannot defer publishing input.
	_, err := io.Copy(captureWriter{b}, struct{ io.Reader }{b.source})
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stop != nil {
		err = b.stop
	} else if err == nil {
		err = io.EOF
	}
	b.finish(err)
}

// captureWriter publishes input without exposing writes on Buffer itself.
type captureWriter struct{ buffer *Buffer }

func (w captureWriter) Write(p []byte) (int, error) {
	b := w.buffer
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	b.ready.Broadcast()
	return len(p), b.stop
}

type reader struct {
	buffer atomic.Pointer[Buffer]
	offset int // Protected by Buffer.mu.
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
	// Wait releases the lock so capture or Close can wake us.
	for r.buffer.Load() != nil && r.offset == len(b.data) && b.err == nil {
		b.ready.Wait()
	}
	if r.buffer.Load() == nil {
		return 0, io.ErrClosedPipe
	}
	n := copy(p, b.data[r.offset:])
	r.offset += n
	// Report the terminal error only after consuming buffered bytes.
	if r.offset == len(b.data) {
		return n, b.err
	}
	return n, nil
}

func (r *reader) Close() error {
	if b := r.buffer.Load(); b != nil {
		b.mu.Lock()
		b.ready.Broadcast()
		r.buffer.Store(nil)
		b.mu.Unlock()
	}
	return nil
}
