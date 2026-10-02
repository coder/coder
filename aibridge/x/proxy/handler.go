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
	"go.opentelemetry.io/otel/attribute"
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
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
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
// The caller owns the transport and its connection lifecycle.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder, transport http.RoundTripper) (*forwardingHandler, error) {
	baseURL, err := url.Parse(prov.BaseURL())
	if err != nil {
		return nil, xerrors.Errorf("configure provider %q base URL: %w", prov.Name(), err)
	}
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
	return h, nil
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
	start := time.Now()
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
	record.ID, record.StartedAt = uuid.NewString(), start.UTC()
	fields := []slog.Field{slog.F("interception_id", record.ID), slog.F("provider", h.provider.Name()), slog.F("credential_kind", string(cred.Kind()))}
	ctx = slog.With(ctx, fields...)
	span.SetAttributes(attribute.String(tracing.InterceptionID, record.ID), attribute.String(tracing.InitiatorID, record.InitiatorID), attribute.String(tracing.Provider, h.provider.Name()))
	// The start record must exist before forwarding or recording its outcome.
	if err := h.recorder.RecordInterception(ctx, record); err != nil {
		span.SetStatus(codes.Error, "failed to record interception")
		h.logger.Warn(ctx, "failed to record interception", slog.Error(err))
		http.Error(w, "failed to record interception", http.StatusInternalServerError)
		return
	}
	asyncRecorder := recorder.NewAsyncRecorder(h.recorder, recorder.DefaultAsyncTimeout)
	defer asyncRecorder.Wait()
	state := &responseObservation{credentialHint: record.CredentialHint, client: w}
	var body *requestBuffer

	route := strings.TrimPrefix(r.Pattern, "/"+h.provider.Name())
	if h.metrics != nil {
		h.metrics.InterceptionsInflight.WithLabelValues(h.provider.Name(), "", route).Inc()
	}
	defer func() {
		panicValue := recover()
		h.finishForwarding(ctx, r, start, record, body, state, asyncRecorder, panicValue)
		if panicValue != nil {
			panic(panicValue)
		}
	}()
	state.err = h.breaker.Execute(strings.TrimPrefix(r.URL.Path, "/"+h.provider.Name()), "", w, func(rw http.ResponseWriter) error {
		outbound, buffer := h.prepareForwarding(r.WithContext(context.WithValue(ctx, observationContextKey{}, state)), cred)
		body = buffer
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
		logger.Debug(ctx, "rejecting oversized request body",
			slog.F("route", strings.TrimPrefix(r.URL.Path, h.provider.RoutePrefix())),
			slog.F("client", string(client)),
			slog.F("content_length", r.ContentLength),
		)
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
	// Body-derived fields remain unset; this step never reads the request body.
	return &recorder.InterceptionRecord{
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

// forward delivers the prepared request and observes its terminal response.
func (h *forwardingHandler) forward(w http.ResponseWriter, r *http.Request, _ *requestBuffer, state *responseObservation) error {
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

// finishForwarding records the outcome after response delivery finishes or aborts.
func (h *forwardingHandler) finishForwarding(ctx context.Context, r *http.Request, start time.Time, record *recorder.InterceptionRecord, body *requestBuffer, state *responseObservation, rec *recorder.AsyncRecorder, panicValue any) {
	end := time.Now()
	terminal := state.err
	if body != nil {
		body.mu.Lock()
		requestErr := body.err
		body.mu.Unlock()
		if terminal == nil && state.status < http.StatusBadRequest && !errors.Is(requestErr, io.EOF) {
			terminal = requestErr
		}
	}
	if terminal == nil && ctx.Err() != nil && (state.body == nil || !state.body.eof) {
		terminal = ctx.Err()
	}
	if terminal == nil && state.body != nil {
		terminal = state.body.readErr
	}
	if terminal == nil && panicValue != nil {
		terminal = xerrors.New("response stream aborted")
	}
	errType, message := interceptionerror.Categorize(h.provider, terminal)
	if _, ok := errors.AsType[*http.MaxBytesError](terminal); ok {
		errType = recorder.ErrorTypeBadRequest
	}
	if terminal == nil && state.status >= http.StatusBadRequest {
		errType, message = recorder.ErrorTypeFromStatus(state.status), http.StatusText(state.status)
	}
	status := metrics.InterceptionCountStatusCompleted
	if errType != "" {
		status = metrics.InterceptionCountStatusFailed
		trace.SpanFromContext(ctx).SetStatus(codes.Error, message)
	}
	if h.metrics != nil {
		route := strings.TrimPrefix(r.Pattern, "/"+h.provider.Name())
		h.metrics.InterceptionsInflight.WithLabelValues(h.provider.Name(), "", route).Dec()
		h.metrics.InterceptionDuration.WithLabelValues(h.provider.Name(), "").Observe(end.Sub(start).Seconds())
		h.metrics.InterceptionCount.WithLabelValues(h.provider.Name(), "", status, route, r.Method, record.InitiatorID, record.Client).Inc()
	}
	_ = rec.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
		ID:             record.ID,
		EndedAt:        end.UTC(),
		CredentialHint: state.credentialHint,
		ErrorType:      errType,
		ErrorMessage:   message,
	})
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
