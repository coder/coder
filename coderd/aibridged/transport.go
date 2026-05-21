package aibridged

import (
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
)

// NewTransportFactory returns a [aibridge.TransportFactory] whose RoundTripper
// dispatches requests to handler in-process, streaming the response body
// through an [io.Pipe] so SSE/NDJSON/chunked responses propagate token-by-token
// just as they would over the wire.
//
// handler is typically the aibridged HTTP entrypoint registered via
// [API.RegisterInMemoryAIBridgedHTTPHandler].
//
// providerFromHost maps an upstream hostname (e.g. "api.anthropic.com") to the
// aibridge provider name (e.g. "anthropic"). The roundtripper prepends
// "/{providerName}" to the request path when a match is found, since the
// aibridge mux dispatches on the provider-prefixed path
// (e.g. "/anthropic/v1/messages") rather than the upstream-native path
// ("/v1/messages") that SDK clients construct. May be nil; in that case the
// path is passed through unchanged and callers are expected to set the
// provider-prefixed path themselves.
func NewTransportFactory(handler http.Handler, providerFromHost func(host string) string) aibridge.TransportFactory {
	return &transportFactory{handler: handler, providerFromHost: providerFromHost}
}

type transportFactory struct {
	handler          http.Handler
	providerFromHost func(host string) string
}

// TransportFor returns an in-process [http.RoundTripper] for coder-agent
// traffic, or (nil, nil) when the caller should fall through to direct
// upstream behavior. The license carve-out only applies to coder-agent
// traffic; external callers continue through the gated HTTP route.
//
//nolint:nilnil,revive // (nil, nil) is the documented "fall through" signal; isCoderAgent is the carve-out gate, not a generic control flag.
func (f *transportFactory) TransportFor(_ uuid.UUID, isCoderAgent bool) (http.RoundTripper, error) {
	if f.handler == nil {
		return nil, xerrors.New("aibridged handler not registered")
	}
	if !isCoderAgent {
		return nil, nil
	}
	return &inMemoryRoundTripper{handler: f.handler, providerFromHost: f.providerFromHost}, nil
}

// inMemoryRoundTripper implements [http.RoundTripper] by invoking handler
// in a goroutine and streaming its response back through an [io.Pipe].
type inMemoryRoundTripper struct {
	handler          http.Handler
	providerFromHost func(host string) string
}

func (t *inMemoryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	pr, pw := io.Pipe()
	rw := &pipeResponseWriter{
		header:     http.Header{},
		body:       pw,
		gotHeaders: make(chan struct{}),
		status:     http.StatusOK,
	}

	// Cloning preserves caller-supplied headers and context but lets the
	// handler operate on its own request value without surprising the caller
	// if it mutates Headers or stores the request.
	served := req.Clone(req.Context())

	// SDK clients (e.g. fantasy/anthropic) construct URLs against the
	// upstream host ("https://api.anthropic.com/v1/messages") so the path
	// arrives here as "/v1/messages". The aibridge mux dispatches on the
	// provider-prefixed path ("/anthropic/v1/messages"), so we rewrite
	// based on the request's Host. If providerFromHost is nil or the host
	// is unknown, leave the path alone; the bridge will return 404 and
	// surface the misconfiguration.
	if t.providerFromHost != nil && served.URL != nil {
		host := served.URL.Host
		if host == "" {
			host = served.Host
		}
		if name := t.providerFromHost(host); name != "" {
			prefix := "/" + name
			if !strings.HasPrefix(served.URL.Path, prefix+"/") && served.URL.Path != prefix {
				served.URL.Path = prefix + served.URL.Path
				// Force net/http to recompute the escaped path from
				// the rewritten Path field on the next read.
				served.URL.RawPath = ""
				served.RequestURI = ""
			}
		}
	}

	go func() {
		defer func() {
			// Make sure we always unblock RoundTrip even if the handler
			// returns before writing headers (e.g. it panicked and the
			// outer http server would have written a 500).
			rw.ensureHeaders()
			// If the request context was canceled, surface that as a
			// body-read error so the caller sees a network-style failure
			// rather than EOF. Otherwise close cleanly.
			if cerr := served.Context().Err(); cerr != nil {
				_ = pw.CloseWithError(cerr)
				return
			}
			_ = pw.Close()
		}()
		t.handler.ServeHTTP(rw, served)
	}()

	// Close the pipe eagerly when the caller cancels, so an unresponsive
	// handler does not strand the consumer's body read. The handler's own
	// context derives from req.Context(), so it observes the same
	// cancellation independently.
	go func() {
		<-served.Context().Done()
		_ = pw.CloseWithError(served.Context().Err())
	}()

	select {
	case <-rw.gotHeaders:
	case <-served.Context().Done():
		return nil, served.Context().Err()
	}

	return &http.Response{
		Status:        http.StatusText(rw.status),
		StatusCode:    rw.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rw.header.Clone(),
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
	header http.Header
	body   *io.PipeWriter

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
