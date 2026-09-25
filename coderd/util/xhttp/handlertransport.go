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
// with h in the calling process. RoundTrip returns when h commits the
// response header: on WriteHeader, Write, or Flush, or when h returns.
// The body streams through an [io.Pipe], so SSE and chunked responses
// arrive as h writes them. As with net/http.Server, the request that h
// serves ends when the caller cancels or h returns: its context ends and
// its body closes. When the caller cancels, a body read returns the
// cancellation cause. When h panics, the status is 500 and a body read
// returns an error.
func HandlerTransport(h http.Handler) http.RoundTripper {
	return &handlerTransport{handler: h}
}

type handlerTransport struct {
	handler http.Handler
}

func (t *handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// ctx ends when the caller cancels or h returns. Its cause ends the
	// response body.
	ctx, end := context.WithCancelCause(req.Context())
	// Cloning lets the handler mutate or store its request without
	// surprising the caller.
	served := req.Clone(ctx)
	// Match net/http.Server: an empty path becomes "/" and the body is
	// never nil.
	if served.URL.Path == "" {
		served.URL.Path = "/"
	}
	if served.Body == nil {
		served.Body = http.NoBody
	}
	reqBody := served.Body

	pr, pw := io.Pipe()
	rw := &pipeResponseWriter{
		header:     http.Header{},
		body:       pw,
		gotHeaders: make(chan struct{}),
	}
	// Release both bodies once, however the exchange ends, so neither a
	// body producer nor a body reader waits on an unresponsive handler.
	context.AfterFunc(ctx, func() {
		_ = reqBody.Close()
		_ = pw.CloseWithError(context.Cause(ctx))
	})
	go func() {
		defer func() {
			cause := io.EOF
			if r := recover(); r != nil {
				// Mirror net/http.Server behavior: a panicking handler
				// produces a 500 instead of crashing the process.
				rw.WriteHeader(http.StatusInternalServerError)
				cause = xerrors.Errorf("handler panicked: %v", r)
			}
			// Unblock RoundTrip when the handler returns without writing.
			rw.WriteHeader(http.StatusOK)
			end(cause)
		}()
		t.handler.ServeHTTP(rw, served)
	}()

	select {
	case <-rw.gotHeaders:
	case <-req.Context().Done():
		return nil, req.Context().Err()
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

// Flush commits the response header with status 200 if the handler has not
// written it, as net/http does. Body bytes need no flush, because each pipe
// write returns only after the reader consumes it.
func (w *pipeResponseWriter) Flush() {
	w.WriteHeader(http.StatusOK)
}

var (
	_ http.ResponseWriter = (*pipeResponseWriter)(nil)
	_ http.Flusher        = (*pipeResponseWriter)(nil)
)
