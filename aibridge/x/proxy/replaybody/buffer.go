// Package replaybody captures a body once for independent readers.
package replaybody

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Buffer captures body bytes once for independent readers and a shared payload.
// Capture starts at construction and runs until EOF, failure, or Stop.
// The caller must limit the body size and call Buffer.Close or Buffer.Stop.
// Buffer and its readers are safe for concurrent use.
type Buffer struct {
	source         *bodySource
	mu             sync.Mutex
	data           []byte
	captureErr     error
	stopCause      error
	readNotify     *sync.Cond // Wakes waiting goroutines on new data, completion, or reader close.
	done           chan struct{}
	closeSource    sync.Once
	sourceCloseErr error
}

// NewRequest clones r and captures its body for independent Body and GetBody
// readers. It takes ownership of r.Body.
// Canceling the request context stops capture asynchronously.
// Buffer.Close or Buffer.Stop must be called before ServeHTTP returns.
//
// For nonempty HTTP/1 bodies, the handler must use the returned writer. It owns
// the Connection response header and prevents reuse for replies before EOF.
// Stopping before EOF interrupts the upload with an inbound read deadline.
// Nil and http.NoBody bodies return w, a clone with http.NoBody, and a
// completed empty buffer.
func NewRequest(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, *http.Request, *Buffer) {
	r = r.Clone(r.Context())
	if r.Body == nil || r.Body == http.NoBody {
		r.Body = http.NoBody
		r.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		return w, r, newBuffer(r.Context(), nil)
	}
	controller := http.NewResponseController(w)
	source := &bodySource{ReadCloser: r.Body, controller: controller, ctx: r.Context()}
	if r.ProtoMajor == 1 {
		w.Header().Set("Connection", "close")
		w = &responseWriter{ResponseWriter: w, source: source}
	}
	body := newBuffer(r.Context(), source)
	//nolint:errcheck // NewReader always returns a nil error.
	r.Body, _ = body.NewReader()
	r.GetBody = body.NewReader
	if r.ContentLength == 0 {
		// ReverseProxy drops bodies at length zero; use -1 for unknown length.
		r.ContentLength = -1
	}
	return w, r, body
}

// NewResponse shallow-copies r and replaces its body with a reader.
// It takes ownership of r.Body. Canceling ctx stops capture asynchronously.
// The caller must still call Buffer.Close or Buffer.Stop when finished.
// Header and Trailer stay shared so transport trailers remain visible after
// capture completes. NewResponse initializes r.Trailer if needed.
// A 101 response returns a nil buffer, caller retains the original body.
// Nil and http.NoBody bodies return original response with http.NoBody and a empty buffer.
func NewResponse(ctx context.Context, r *http.Response) (*http.Response, *Buffer) {
	response := *r
	if response.StatusCode == http.StatusSwitchingProtocols {
		return &response, nil
	}
	if response.Body == nil || response.Body == http.NoBody {
		response.Body = http.NoBody
		return &response, newBuffer(ctx, nil)
	}
	if response.Trailer == nil {
		// The transport may populate undeclared trailers after capture starts.
		r.Trailer = make(http.Header)
		response.Trailer = r.Trailer
	}
	body := newBuffer(ctx, &bodySource{ReadCloser: response.Body, ctx: ctx})
	//nolint:errcheck // NewReader always returns a nil error.
	response.Body, _ = body.NewReader()
	return &response, body
}

// newBuffer creates a new buffer and starts capture.
// Empty bodies complete synchronously.
// Source must unblock pending reads when closed.
func newBuffer(ctx context.Context, source *bodySource) *Buffer {
	if source == nil {
		source = &bodySource{}
	}
	b := &Buffer{
		source: source,
		done:   make(chan struct{}),
	}
	b.readNotify = sync.NewCond(&b.mu)
	if source.ReadCloser == nil || source.ReadCloser == http.NoBody {
		source.ReadCloser = http.NoBody
		source.eof.Store(true)
		b.captureErr = io.EOF
		close(b.done)
		return b
	}
	stopAfterFunc := context.AfterFunc(ctx, func() {
		//nolint:errcheck // Stop caches the error and Buffer.Close returns it.
		_ = b.Stop(context.Cause(ctx))
	})
	go b.capture(stopAfterFunc)
	return b
}

// Wait returns the shared payload and capture error once capture finishes.
// A nil error means successful capture; wrapped EOF is a failure.
// The returned bytes are read-only and remain valid after Buffer.Close.
// Canceling ctx cancels only this wait, not capture.
func (b *Buffer) Wait(ctx context.Context) ([]byte, error) {
	select {
	case <-b.done:
		b.mu.Lock()
		defer b.mu.Unlock()
		err := b.captureErr
		if err == io.EOF { //nolint:errorlint // A wrapped EOF is a failure, not complete input.
			err = nil
		}
		// Clamp capacity so appending bytes allocates new storage.
		return b.data[:len(b.data):len(b.data)], err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Stop closes the source once and waits for capture, retaining in-flight bytes.
// Finished capture keeps its result. For unfinished capture, source EOF wins
// over stopping; otherwise the first stop cause wins. Interrupted capture wraps
// io.ErrClosedPipe and any supplied cause.
// Stop returns the source Close error, not the capture error.
func (b *Buffer) Stop(cause error) error {
	b.mu.Lock()
	if b.captureErr == nil && b.stopCause == nil {
		b.stopCause = errors.Join(io.ErrClosedPipe, cause)
	}
	b.mu.Unlock()
	b.closeSource.Do(func() { b.sourceCloseErr = b.source.Close() })
	<-b.done
	return b.sourceCloseErr
}

// Close stops unfinished capture with io.ErrClosedPipe and waits for capture.
// It closes the source without invalidating retained payloads or readers.
func (b *Buffer) Close() error { return b.Stop(nil) }

// finish is called with mu held, after all captured bytes have been published.
func (b *Buffer) finish(err error) {
	b.captureErr = err
	b.readNotify.Broadcast()
	close(b.done)
}

func (b *Buffer) capture(stopAfterFunc func() bool) {
	_, err := io.Copy(captureWriter{b}, b.source)
	stopAfterFunc()
	b.mu.Lock()
	defer b.mu.Unlock()
	// Source EOF wins even if the final write reports a stop cause.
	if err == nil || b.source.eof.Load() {
		err = io.EOF
	} else if b.stopCause != nil {
		err = b.stopCause
	}
	b.finish(err)
}

// captureWriter publishes in-flight bytes even when Stop has been requested.
type captureWriter struct{ buffer *Buffer }

func (w captureWriter) Write(p []byte) (int, error) {
	b := w.buffer
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	b.readNotify.Broadcast()
	return len(p), b.stopCause
}

// bodySource interrupts unfinished HTTP server reads with a read deadline.
// Its underlying ReadCloser must unblock pending reads when closed.
type bodySource struct {
	io.ReadCloser
	controller *http.ResponseController
	ctx        context.Context
	eof        atomic.Bool
}

func (s *bodySource) Read(p []byte) (int, error) {
	n, err := s.ReadCloser.Read(p)
	if err == io.EOF { // A wrapped EOF is a failure, not complete input.
		s.eof.Store(true)
		return n, err
	}
	// Wrap cancellation so even an EOF cause remains a read failure.
	if cause := context.Cause(s.ctx); cause != nil {
		return n, errors.Join(io.ErrClosedPipe, cause)
	}
	return n, err
}

func (s *bodySource) Close() error {
	if s.controller != nil && !s.eof.Load() {
		//nolint:errcheck // Deadline interruption is best-effort.
		_ = s.controller.SetReadDeadline(time.Now())
	}
	return s.ReadCloser.Close()
}
