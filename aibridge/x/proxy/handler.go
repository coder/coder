package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	aibclient "github.com/coder/coder/v2/aibridge/client"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

type forwardingHandler struct {
	provider  provider.Provider
	logger    slog.Logger
	metrics   *metrics.Metrics
	tracer    trace.Tracer
	transport http.RoundTripper
	failover  keypool.KeyFailoverConfig
	breaker   *circuitbreaker.ProviderCircuitBreakers
	proxy     *httputil.ReverseProxy
	inflight  *aibridge.InflightGate
	recorder  recorder.Recorder
}

var _ http.Handler = (*forwardingHandler)(nil)

// newForwardingHandler constructs one forwarding engine per provider snapshot.
// The router owns the returned transport and closes idle connections on retirement.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder) (*forwardingHandler, *http.Transport, error) {
	baseURL, err := url.Parse(prov.BaseURL())
	if err != nil {
		return nil, nil, xerrors.Errorf("configure provider %q base URL: %w", prov.Name(), err)
	}
	transport := utils.NewStreamingTransport()
	transport.DisableCompression = true
	h := &forwardingHandler{
		provider:  prov,
		logger:    logger,
		metrics:   m,
		tracer:    tracer,
		transport: apidump.NewPassthroughMiddleware(transport, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal()),
		failover:  prov.KeyFailoverConfig(logger),
		breaker:   circuitbreaker.NewProviderCircuitBreakers(prov.Name(), prov.CircuitBreakerConfig(), logger, m),
		inflight:  inflight,
		recorder:  rec,
	}
	h.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteForwardingURL(pr, prov.RoutePrefix(), baseURL)
			pr.Out.Header = prepareForwardingHeaders(pr)
		},
		Transport:     h,
		FlushInterval: -1,
		ErrorHandler: func(_ http.ResponseWriter, r *http.Request, err error) {
			observationFromContext(r.Context()).err = err
		},
		ErrorLog: slog.Stdlib(context.Background(), logger.With(slog.F("provider", prov.Name())), slog.LevelWarn),
	}
	return h, transport, nil
}

func rewriteForwardingURL(pr *httputil.ProxyRequest, routePrefix string, baseURL *url.URL) {
	pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, routePrefix)
	if pr.In.URL.RawPath != "" {
		pr.Out.URL.RawPath = strings.TrimPrefix(pr.In.URL.RawPath, routePrefix)
	}
	// ReverseProxy may sanitize query parameters; forwarding preserves the bytes.
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetURL(baseURL)
}

func prepareForwardingHeaders(pr *httputil.ProxyRequest) http.Header {
	prepared := headers.PrepareClientHeaders(pr.Out.Header)
	// Unlike SDK requests, forwarding preserves the client's encoding choice.
	if values, ok := pr.Out.Header["Accept-Encoding"]; ok {
		prepared["Accept-Encoding"] = values
	}
	// In contains only the selected credential. Restore it even if Connection
	// nominated it for removal by ReverseProxy.
	for _, name := range []string{headers.AuthHeaderAuthorization, headers.AuthHeaderXAPIKey} {
		if value := pr.In.Header.Get(name); value != "" {
			prepared.Set(name, value)
		}
	}
	return prepared
}

func (h *forwardingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	release, ok := h.inflight.Admit()
	if !ok {
		http.Error(w, "AI Gateway is shutting down", http.StatusServiceUnavailable)
		return
	}
	defer release()
	ctx, cleanup := h.inflight.RequestContext(r.Context())
	defer cleanup()
	ctx, span := h.tracer.Start(ctx, "Proxy")
	defer span.End()
	r = r.WithContext(ctx)
	record, cred := h.checkRequest(w, r)
	if record == nil {
		return
	}

	state := &responseObservation{credentialHint: record.CredentialHint, client: w}
	defer func() { h.finishForwarding(ctx, record, state.err) }()
	route := strings.TrimPrefix(r.Pattern, "/"+h.provider.Name())
	state.err = h.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		outbound, body := h.prepareForwarding(r.WithContext(context.WithValue(ctx, observationContextKey{}, state)), cred)
		return h.forward(rw, outbound, body, state)
	})
}

// checkRequest validates a bridged request without reading its body. On rejection
// it writes the response and returns nil, nil. Otherwise it returns the initial
// interception record and credential without writing a response.
func (h *forwardingHandler) checkRequest(w http.ResponseWriter, r *http.Request) (*recorder.InterceptionRecord, credential.Credential) {
	ctx := r.Context()
	logger := h.logger.With(slog.F("provider", h.provider.Name()))
	client := aibclient.GuessClient(r)
	if headers.IsWebSocketUpgrade(r) {
		logger.Debug(ctx, "rejecting unsupported WebSocket upgrade",
			slog.F("route", strings.TrimPrefix(r.URL.Path, h.provider.RoutePrefix())),
			slog.F("client", string(client)),
		)
		http.Error(w, "WebSocket transport is not supported, use HTTP", http.StatusNotImplemented)
		return nil, nil
	}
	firewallID, firewallSequence, err := headers.ExtractAgentFirewallHeaders(r)
	if err != nil {
		logger.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.Error(err))
		http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
		return nil, nil
	}
	if r.ContentLength > routing.MaxRequestBodyBytes {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		logger.Warn(ctx, "rejecting oversized request body", slog.F("content_length", r.ContentLength))
		routing.WriteRequestBodyTooLarge(ctx, w)
		return nil, nil
	}

	actor := aibcontext.ActorFromContext(ctx)
	if actor == nil {
		logger.Warn(ctx, "rejecting request without an actor")
		http.Error(w, "no actor found", http.StatusBadRequest)
		return nil, nil
	}
	if h.recorder == nil {
		logger.Warn(ctx, "rejecting request without a recorder")
		http.Error(w, "recorder unavailable", http.StatusInternalServerError)
		return nil, nil
	}
	cred, err := h.provider.ResolveCredential(r)
	if err != nil {
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "failed to resolve credential")
		logger.Warn(ctx, "failed to resolve credential", slog.Error(err), slog.F("path", r.URL.Path))
		if errors.Is(err, provider.ErrNoCredential) {
			http.Error(w, "upstream authentication unavailable: no provider credentials supplied or configured", http.StatusForbidden)
		} else {
			http.Error(w, "upstream authentication unavailable", http.StatusInternalServerError)
		}
		return nil, nil
	}
	switch cred.(type) {
	case credential.BYOK, *credential.CentralizedPool:
	default:
		// The type name identifies the credential kind; the value may hold secrets.
		logger.Warn(ctx, "rejecting unsupported upstream credential", slog.F("credential_type", fmt.Sprintf("%T", cred)))
		http.Error(w, "upstream authentication is not supported in proxy mode", http.StatusNotImplemented)
		return nil, nil
	}
	var metadata recorder.Metadata
	if actor.Username != "" {
		metadata = recorder.Metadata{"Username": actor.Username}
	}
	return &recorder.InterceptionRecord{
		InitiatorID:                 actor.ID.String(),
		Metadata:                    metadata,
		Provider:                    h.provider.Type(),
		ProviderName:                h.provider.Name(),
		Client:                      string(client),
		UserAgent:                   r.UserAgent(),
		AgentFirewallSessionID:      firewallID,
		AgentFirewallSequenceNumber: firewallSequence,
		CredentialKind:              cred.Kind(),
		CredentialHint:              cred.Hint(),
	}, cred
}

func (*forwardingHandler) prepareForwarding(r *http.Request, cred credential.Credential) (*http.Request, *requestBuffer) {
	body := &requestBuffer{source: r.Body}
	if r.Body == nil || r.Body == http.NoBody {
		body.err = io.EOF
	}
	r = r.Clone(r.Context())
	if r.Body != nil && r.Body != http.NoBody && r.ContentLength == 0 {
		r.ContentLength = -1 // In-process callers may use zero for an unknown length.
	}
	r.Body, _ = body.getBody()
	r.GetBody = body.getBody
	r.Header.Del(headers.AuthHeaderAuthorization)
	r.Header.Del(headers.AuthHeaderXAPIKey)
	if byok, ok := credential.AsBYOK(cred); ok {
		value := byok.Secret
		if byok.Header == headers.AuthHeaderAuthorization {
			value = "Bearer " + value
		}
		r.Header.Set(byok.Header, value)
	}
	return r, body
}

func (*forwardingHandler) forward(w http.ResponseWriter, r *http.Request, _ *requestBuffer, _ *responseObservation) error {
	// TODO: connect forwardPrepared only when lifecycle recording is connected.
	http.NotFound(w, r)
	return nil
}

func (*forwardingHandler) finishForwarding(context.Context, *recorder.InterceptionRecord, error) {
	// TODO: finalize the outcome and lifecycle records, including breaker rejection.
}

// forwardPrepared delivers the prepared request and observes its terminal response.
func (h *forwardingHandler) forwardPrepared(w http.ResponseWriter, r *http.Request, _ *requestBuffer, state *responseObservation) error {
	ctx := r.Context()
	writer := &errorCapturingWriter{ResponseWriter: w, client: state.client}
	defer func() {
		if state.err == nil {
			state.err = writer.Error()
		}
		if state.body != nil && !state.body.closed {
			_ = state.body.Close()
		}
	}()
	h.proxy.ServeHTTP(writer, r)
	if state.body == nil && state.err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](state.err); ok {
			routing.WriteRequestBodyTooLarge(ctx, writer)
		} else {
			h.logger.Warn(ctx, "upstream proxy error", slog.Error(state.err))
			http.Error(writer, "upstream proxy error", http.StatusBadGateway)
		}
	}
	if writer.Error() != nil || (state.body != nil && !state.body.eof) {
		if state.err == nil {
			state.err = writer.Error()
		}
		panic(http.ErrAbortHandler)
	}
	return state.err
}

// RoundTrip observes only the final response returned by key failover. Its
// callbacks and bookkeeping belong to this request, not the shared proxy.
func (h *forwardingHandler) RoundTrip(r *http.Request) (*http.Response, error) {
	state := observationFromContext(r.Context())
	cfg := h.failover
	if cfg.Pool != nil {
		inject, build := cfg.InjectAuthKey, cfg.BuildKeyPoolResponse
		cfg.InjectAuthKey = func(header *http.Header, key string) {
			state.credentialHint = utils.MaskSecret(key)
			inject(header, key)
		}
		cfg.BuildKeyPoolResponse = func(err *keypool.Error) *http.Response {
			state.err = err
			return build(err)
		}
	}
	resp, err := keypool.NewKeyFailoverTransport(h.transport, cfg).RoundTrip(r)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		_ = resp.Body.Close()
		return nil, xerrors.New("upstream protocol upgrades are not supported")
	}
	state.status = resp.StatusCode
	state.body = &observedBody{ReadCloser: resp.Body}
	resp.Body = state.body
	return resp, nil
}

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

type errorCapturingWriter struct {
	http.ResponseWriter
	client http.ResponseWriter
	// ReverseProxy's initial flush can run on its timer goroutine.
	err atomic.Pointer[error]
}

func (w *errorCapturingWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		// The existing breaker tracks the first status. Informational responses
		// must reach the client without hiding the final status from the breaker.
		w.client.WriteHeader(status)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *errorCapturingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	w.recordError(err)
	return n, err
}

func (w *errorCapturingWriter) recordError(err error) {
	if err != nil {
		w.err.CompareAndSwap(nil, &err)
	}
}

func (w *errorCapturingWriter) Error() error {
	if err := w.err.Load(); err != nil {
		return *err
	}
	return nil
}

func (w *errorCapturingWriter) FlushError() error {
	// The breaker wrapper only exposes Flush, which discards flush errors.
	err := http.NewResponseController(w.client).Flush()
	w.recordError(err)
	return err
}

func (w *errorCapturingWriter) Flush() { _ = w.FlushError() }
func (w *errorCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}
func (w *errorCapturingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
