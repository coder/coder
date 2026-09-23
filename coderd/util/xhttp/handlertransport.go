package xhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"golang.org/x/xerrors"
)

// HandlerTransport returns an [http.RoundTripper] that serves each request
// with h in the calling process. RoundTrip returns when h writes the
// response header. The body streams through an [io.Pipe], so SSE and
// chunked responses arrive as h writes them. When the request context
// ends, a body read returns the context error. When h panics, the status
// is 500 and a body read returns an error.
func HandlerTransport(h http.Handler) http.RoundTripper {
	return &handlerTransport{handler: h}
}

type handlerTransport struct {
	handler http.Handler
}

func (t *handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	pr, pw := io.Pipe()
	rw := &pipeResponseWriter{
		header:     http.Header{},
		body:       pw,
		gotHeaders: make(chan struct{}),
	}
	// Cloning lets the handler mutate or store its request without
	// surprising the caller.
	served := req.Clone(ctx)

	// Close the pipe when the caller cancels, so an unresponsive handler
	// does not strand the body read.
	stop := context.AfterFunc(ctx, func() { _ = pw.CloseWithError(ctx.Err()) })
	go func() {
		defer func() {
			stop()
			if r := recover(); r != nil {
				// Mirror net/http.Server behavior: a panicking handler
				// produces a 500 instead of crashing the process.
				rw.WriteHeader(http.StatusInternalServerError)
				_ = pw.CloseWithError(xerrors.Errorf("handler panicked: %v", r))
			}
			// Unblock RoundTrip when the handler returns without writing.
			rw.WriteHeader(http.StatusOK)
			// A canceled request ends the body with the context error, as
			// a network failure would. Otherwise the body ends with EOF.
			_ = pw.CloseWithError(ctx.Err())
		}()
		t.handler.ServeHTTP(rw, served)
	}()

	select {
	case <-rw.gotHeaders:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status)),
		StatusCode:    rw.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rw.frozenHeader,
		Body:          pr,
		Request:       req,
		ContentLength: -1,
	}, nil
}

// pipeResponseWriter is an [http.ResponseWriter] that streams the response
// body into an [io.PipeWriter]. The first call to WriteHeader (implicit or
// explicit) closes gotHeaders so that RoundTrip can return while the handler
// keeps writing.
type pipeResponseWriter struct {
	header       http.Header
	frozenHeader http.Header
	body         *io.PipeWriter

	once       sync.Once
	gotHeaders chan struct{}
	status     int
}

func (w *pipeResponseWriter) Header() http.Header {
	return w.header
}

func (w *pipeResponseWriter) WriteHeader(status int) {
	w.once.Do(func() {
		w.status = status
		w.frozenHeader = w.header.Clone()
		close(w.gotHeaders)
	})
}

func (w *pipeResponseWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	// An empty pipe write blocks until a read consumes it, and the read
	// returns zero bytes. Readers such as bufio fail after repeated
	// zero-byte reads, so skip the write.
	if len(p) == 0 {
		return 0, nil
	}
	return w.body.Write(p)
}

// Flush is a no-op because each pipe write returns only after the reader
// consumes it. It satisfies [http.Flusher], so handlers that type-assert it
// for SSE do not fall back to buffering.
func (*pipeResponseWriter) Flush() {}

var (
	_ http.ResponseWriter = (*pipeResponseWriter)(nil)
	_ http.Flusher        = (*pipeResponseWriter)(nil)
)
