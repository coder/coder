package aibridged

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
)

// NewTransportFactory returns an [aibridge.TransportFactory] whose RoundTripper
// dispatches requests to handler in-process, streaming the response body
// through an [io.Pipe] so SSE/NDJSON/chunked responses propagate token-by-token
// just as they would over the wire.
//
// handler is typically the aibridged HTTP entrypoint registered via
// [API.RegisterInMemoryAIBridgedHTTPHandler].
func NewTransportFactory(handler http.Handler) aibridge.TransportFactory {
	return &transportFactory{handler: handler}
}

type transportFactory struct {
	handler http.Handler
}

// TransportFor returns an in-process [http.RoundTripper] that dispatches
// requests through the aibridged handler. The provider name is the routing
// key the daemon mounts on; the round-tripper rewrites each request's URL
// path to "/api/v2/ai-gateway/<providerName>/..." before dispatching so
// callers can build upstream-shaped requests and stay agnostic of the
// daemon's mount layout. The source is attached to the request context for
// downstream logging; routing does not depend on it.
func (f *transportFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	if f.handler == nil {
		return nil, xerrors.New("aibridged handler not registered")
	}
	if providerName == "" {
		return nil, xerrors.New("provider name is required")
	}
	return &inMemoryRoundTripper{handler: f.handler, providerName: providerName, source: source}, nil
}

// inMemoryRoundTripper implements [http.RoundTripper] by invoking handler
// in a goroutine and streaming its response back through an [io.Pipe].
type inMemoryRoundTripper struct {
	handler      http.Handler
	providerName string
	source       aibridge.Source
}

func (t *inMemoryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// The in-process transport requires the caller to have placed the
	// delegated API key ID on the context. Without it, aibridged has no
	// identity to act under. Fail fast at the transport boundary so the
	// handler can assume the invariant.
	if _, ok := aibridge.DelegatedAPIKeyIDFromContext(req.Context()); !ok {
		return nil, xerrors.New("aibridged in-memory transport requires WithDelegatedAPIKeyID on the request context")
	}

	// Adapt the caller's upstream-shaped URL to the daemon's mount layout:
	// "/api/v2/ai-gateway/<providerName>/<original-path>". Done here so
	// callers do not need to encode the mount prefix or the provider
	// routing key into the requests they hand to the transport.
	newPath, err := url.JoinPath(aibridge.AIGatewayRootPath, t.providerName, req.URL.Path)
	if err != nil {
		return nil, xerrors.Errorf("rewrite request URL for provider %q: %w", t.providerName, err)
	}
	req = req.Clone(req.Context())
	req.URL.Path = newPath

	pr, pw := io.Pipe()
	rw := &pipeResponseWriter{
		header:     http.Header{},
		body:       pw,
		gotHeaders: make(chan struct{}),
		status:     http.StatusOK,
	}

	// Cloning preserves caller-supplied headers and context but lets the
	// handler operate on its own request value without surprising the caller
	// if it mutates Headers or stores the request. The Source is attached to
	// the served context so downstream handlers can log the call site.
	served := req.Clone(aibridge.WithSource(req.Context(), t.source))

	handlerDone := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Mirror net/http.Server behavior: a panicking handler
				// produces a 500 instead of crashing the process.
				rw.WriteHeader(http.StatusInternalServerError)
				_ = pw.CloseWithError(xerrors.Errorf("handler panicked: %v", r))
			}
			// Make sure we always unblock RoundTrip even if the handler
			// returns before writing headers (e.g. handler returns early
			// without writing).
			rw.ensureHeaders()
			// If the request context was canceled, surface that as a
			// body-read error so the caller sees a network-style failure
			// rather than EOF. Otherwise close cleanly.
			if cerr := served.Context().Err(); cerr != nil {
				_ = pw.CloseWithError(cerr)
			} else {
				_ = pw.Close()
			}
			close(handlerDone)
		}()
		t.handler.ServeHTTP(rw, served)
	}()

	// Close the pipe eagerly when the caller cancels, so an unresponsive
	// handler does not strand the consumer's body read. The handler's own
	// context derives from req.Context(), so it observes the same
	// cancellation independently. The goroutine also exits when the handler
	// completes normally (handlerDone closes) to avoid leaking a parked
	// goroutine per successful request.
	go func() {
		select {
		case <-served.Context().Done():
			_ = pw.CloseWithError(served.Context().Err())
		case <-handlerDone:
			// Handler finished; nothing to cancel.
		}
	}()

	select {
	case <-rw.gotHeaders:
	case <-served.Context().Done():
		return nil, served.Context().Err()
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
		ContentLength: -1, // streaming; unknown length
	}, nil
}

// pipeResponseWriter is an [http.ResponseWriter] that streams the response
// body into an [io.PipeWriter]. The first call to WriteHeader (implicit or
// explicit) closes gotHeaders so the RoundTrip caller can return an
// *http.Response while the handler keeps writing.
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
	// net/http semantics: an implicit 200 OK on first Write if the handler
	// did not call WriteHeader explicitly.
	w.WriteHeader(http.StatusOK)
	return w.body.Write(p)
}

// Flush is a no-op: pipe writes are already synchronous with the reader, so
// each Write is observed as soon as the reader consumes it. We satisfy
// [http.Flusher] so handlers that type-assert it (the aibridge library does
// for SSE) do not fall back to buffered mode.
func (*pipeResponseWriter) Flush() {}

// ensureHeaders closes gotHeaders if it has not already been closed, with the
// current status. Used to unblock RoundTrip on handler return-without-write.
func (w *pipeResponseWriter) ensureHeaders() {
	w.once.Do(func() {
		close(w.gotHeaders)
	})
}

var (
	_ http.ResponseWriter = (*pipeResponseWriter)(nil)
	_ http.Flusher        = (*pipeResponseWriter)(nil)
)
