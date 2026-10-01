package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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
	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	"github.com/coder/quartz"
)

type forwardingHandler struct {
	provider provider.Provider
	baseURL  *url.URL
	logger   slog.Logger
	metrics  *metrics.Metrics
	tracer   trace.Tracer
	// connections holds the provider's upstream connections.
	connections *http.Transport
	// transport sends forwarded requests over connections.
	transport http.RoundTripper
	failover  keypool.KeyFailoverConfig
	breaker   *circuitbreaker.ProviderCircuitBreakers
	inflight  *aibridge.InflightGate
	recorder  recorder.Recorder
}

var _ http.Handler = (*forwardingHandler)(nil)

// newForwardingHandler constructs one forwarding engine per provider snapshot.
// The handler owns its upstream connections. Call CloseIdleConnections when
// the snapshot is retired.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder) (*forwardingHandler, error) {
	if rec == nil {
		return nil, xerrors.New("recorder is required")
	}
	baseURL, err := url.Parse(prov.BaseURL())
	if err != nil {
		return nil, xerrors.Errorf("configure provider %q base URL: %w", prov.Name(), err)
	}
	connections := utils.NewStreamingTransport()
	// Forwarding passes the client's Accept-Encoding and the response bytes
	// through unchanged, so the transport must not negotiate or decode gzip.
	connections.DisableCompression = true
	h := &forwardingHandler{
		provider:    prov,
		baseURL:     baseURL,
		logger:      logger.With(slog.F("provider", prov.Name())),
		metrics:     m,
		tracer:      tracer,
		connections: connections,
		transport:   apidump.NewPassthroughMiddleware(connections, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal()),
		failover:    prov.KeyFailoverConfig(logger),
		breaker:     circuitbreaker.NewProviderCircuitBreakers(prov.Name(), prov.CircuitBreakerConfig(), logger, m),
		inflight:    inflight,
		recorder:    rec,
	}
	return h, nil
}

func (h *forwardingHandler) newProxy(cred credential.Credential) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteForwardingURL(pr, h.provider.RoutePrefix(), h.baseURL)
			pr.Out.Header = prepareForwardingHeaders(pr.Out.Header, cred)
		},
		Transport:     h,
		FlushInterval: -1,
		ErrorHandler: func(_ http.ResponseWriter, r *http.Request, err error) {
			observationFromContext(r.Context()).err = err
		},
		ErrorLog: slog.Stdlib(context.Background(), h.logger, slog.LevelWarn),
	}
}

// CloseIdleConnections closes upstream connections that are not in use.
// Requests in flight keep their connections until they finish.
func (h *forwardingHandler) CloseIdleConnections() {
	h.connections.CloseIdleConnections()
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

func prepareForwardingHeaders(clientHeaders http.Header, cred credential.Credential) http.Header {
	prepared := headers.PrepareClientHeaders(clientHeaders)
	// Unlike SDK requests, forwarding preserves the client's encoding choice.
	if values, ok := clientHeaders["Accept-Encoding"]; ok {
		prepared["Accept-Encoding"] = values
	}
	if byok, ok := credential.AsBYOK(cred); ok {
		value := byok.Secret
		if byok.Header == headers.AuthHeaderAuthorization {
			value = "Bearer " + value
		}
		prepared.Set(byok.Header, value)
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
	logger, record, _ := h.checkRequest(w, r)
	if record == nil {
		return
	}

	// Forwarding captures the request body as it streams upstream. Without full
	// duplex, the HTTP/1 server drains the original body past the capture before
	// writing an early reply. Writers that cannot enable it keep the default drain.
	if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
		logger.Debug(ctx, "full duplex unavailable, early replies drain the request body first", slog.Error(err))
	}

	state := &responseObservation{credentialHint: record.CredentialHint, client: w}
	defer func() { h.finishForwarding(ctx, record, logger, state.err) }()
	route := strings.TrimPrefix(r.URL.Path, "/"+h.provider.Name())
	state.err = h.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		// Capture stays dormant until lifecycle recording activates forwarding.
		return h.forward(rw, r.WithContext(context.WithValue(ctx, observationContextKey{}, state)), nil, state, logger)
	})
}

// checkRequest validates a bridged request without reading its body. On rejection
// it writes the response and returns a nil record and credential. Otherwise it
// returns the request logger, initial interception record, and credential
// without writing a response.
func (h *forwardingHandler) checkRequest(w http.ResponseWriter, r *http.Request) (slog.Logger, *recorder.InterceptionRecord, credential.Credential) {
	ctx := r.Context()
	client := aibclient.GuessClient(r)

	logger := h.logger.With(
		slog.F("path", r.URL.Path),
		slog.F("user_agent", r.UserAgent()),
		slog.F("client", string(client)),
	)

	if headers.IsWebSocketUpgrade(r) {
		logger.Debug(ctx, "rejecting unsupported WebSocket upgrade")
		http.Error(w, "WebSocket transport is not supported, use HTTP", http.StatusNotImplemented)
		return logger, nil, nil
	}

	if r.ContentLength > routing.MaxRequestBodyBytes {
		logger.Debug(ctx, "rejecting oversized request body", slog.F("content_length", r.ContentLength))
		routing.WriteRequestBodyTooLarge(ctx, w)
		return logger, nil, nil
	}

	firewallID, firewallSequence, err := headers.ExtractAgentFirewallHeaders(r)
	if err != nil {
		logger.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.Error(err))
		http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
		return logger, nil, nil
	}

	actor := aibcontext.ActorFromContext(ctx)
	if actor == nil {
		logger.Warn(ctx, "rejecting request without an actor")
		http.Error(w, "no actor found", http.StatusBadRequest)
		return logger, nil, nil
	}
	logger = logger.With(slog.F("initiator_id", actor.ID.String()))

	cred, err := h.provider.ResolveCredential(r)
	if err != nil {
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "failed to resolve credential")
		logger.Warn(ctx, "failed to resolve credential", slog.Error(err))
		if errors.Is(err, provider.ErrNoCredential) {
			http.Error(w, "upstream authentication unavailable: no provider credentials supplied or configured", http.StatusForbidden)
		} else {
			http.Error(w, "upstream authentication unavailable", http.StatusInternalServerError)
		}
		return logger, nil, nil
	}

	switch cred.(type) {
	case credential.BYOK, *credential.CentralizedPool:
	default:
		// The type name identifies the credential kind; the value may hold secrets.
		// TODO: https://linear.app/codercom/issue/AIGOV-628
		logger.Warn(ctx, "rejecting unsupported upstream credential", slog.F("credential_type", fmt.Sprintf("%T", cred)))
		http.Error(w, "upstream authentication is not supported in proxy mode", http.StatusNotImplemented)
		return logger, nil, nil
	}
	var metadata recorder.Metadata
	if actor.Username != "" {
		metadata = recorder.Metadata{"Username": actor.Username}
	}
	ir := &recorder.InterceptionRecord{
		StartedAt:                   time.Now().UTC(),
		ID:                          uuid.NewString(),
		InitiatorID:                 actor.ID.String(),
		Metadata:                    metadata,
		Provider:                    h.provider.Type(),
		ProviderName:                h.provider.Name(),
		UserAgent:                   r.UserAgent(),
		Client:                      string(client),
		AgentFirewallSessionID:      firewallID,
		AgentFirewallSequenceNumber: firewallSequence,
		CredentialKind:              cred.Kind(),
		CredentialHint:              cred.Hint(),
		// Model:                    TODO, depends on extractor
		// ClientSessionID:          TODO, depends on request buffering
		// CorrelatingToolCallID:    TODO, depends on extractor
		// TODO https://linear.app/codercom/issue/AIGOV-613
	}

	return logger, ir, cred
}

func (*forwardingHandler) prepareForwarding(w http.ResponseWriter, r *http.Request, _ slog.Logger) (http.ResponseWriter, *http.Request, *replaybody.TeeBuffer) {
	r = r.Clone(r.Context())
	body := replaybody.NewRequest(r)
	return w, r, body
}

func (*forwardingHandler) forward(w http.ResponseWriter, _ *http.Request, _ *replaybody.TeeBuffer, _ *responseObservation, _ slog.Logger) error {
	// TODO: connect forwardPrepared only when lifecycle recording is connected.
	// https://linear.app/codercom/issue/AIGOV-615
	http.Error(w, "bridged routes are not yet implemented in proxy mode", http.StatusNotImplemented)
	return nil
}

func (*forwardingHandler) finishForwarding(context.Context, *recorder.InterceptionRecord, slog.Logger, error) {
	// TODO: finalize the outcome and lifecycle records, including breaker rejection.
	// https://linear.app/codercom/issue/AIGOV-615
}

// forwardPrepared delivers the prepared request and observes its terminal response.
func (h *forwardingHandler) forwardPrepared(w http.ResponseWriter, r *http.Request, cred credential.Credential, _ *replaybody.TeeBuffer, state *responseObservation, logger slog.Logger) error {
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
	h.newProxy(cred).ServeHTTP(writer, r)
	if state.body == nil && state.err != nil {
		//nolint:errcheck // AsType returns the match; only its presence matters.
		if _, ok := errors.AsType[*http.MaxBytesError](state.err); ok {
			routing.WriteRequestBodyTooLarge(ctx, writer)
		} else {
			logger.Warn(ctx, "upstream proxy error", slog.Error(state.err))
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
// callbacks and bookkeeping belong to this request, not the shared handler.
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

func (w *errorCapturingWriter) Flush() {
	//nolint:errcheck // FlushError caches failures; Flusher cannot return them.
	_ = w.FlushError()
}

func (w *errorCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}
func (w *errorCapturingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
