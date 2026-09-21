package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/clientmeta"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
)

// Proxy mode does not parse protocol payloads, so the model remains unknown
// until extractor integration.
const unknownModel = ""

type forwardingHandler struct {
	provider  provider.Provider
	baseURL   *url.URL
	transport http.RoundTripper
	breaker   *circuitbreaker.ProviderCircuitBreakers
	recorder  recorder.Recorder
	logger    slog.Logger
	metrics   *metrics.Metrics
	tracer    trace.Tracer
	record    bool
}

func (h *forwardingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "Proxy")
	defer span.End()
	r = r.WithContext(ctx)

	actor := aibcontext.ActorFromContext(ctx)
	if h.record && actor == nil {
		h.logger.Warn(ctx, "rejecting request without actor", slog.F("path", r.URL.Path))
		http.Error(w, "no actor found", http.StatusBadRequest)
		return
	}
	if clientmeta.IsWebSocketUpgrade(r) {
		h.logger.Debug(ctx, "rejecting unsupported WebSocket upgrade", slog.F("path", r.URL.Path))
		http.Error(w, "WebSocket transport is not supported, use HTTP", http.StatusNotImplemented)
		return
	}
	firewallSessionID, firewallSequence, err := clientmeta.ExtractAgentFirewallHeaders(r)
	if err != nil {
		// Do not log the malformed header values.
		h.logger.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.F("path", r.URL.Path))
		http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
		return
	}

	payload, err := captureRequestBody(w, r)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) || errors.Is(err, errDeclaredBodyTooLarge) {
			h.logger.Warn(ctx, "rejecting oversized request body", slog.F("path", r.URL.Path))
			routing.WriteRequestBodyTooLarge(ctx, w)
		} else {
			h.logger.Warn(ctx, "failed to read request body", slog.F("path", r.URL.Path))
			http.Error(w, "failed to read request body", http.StatusBadRequest)
		}
		return
	}
	setReplayBody(r, payload)

	client := clientmeta.GuessClient(r)
	sessionID := clientmeta.GuessSessionIDFromPayload(client, r, payload)
	failoverConfig := h.provider.KeyFailoverConfig(h.logger)
	credentialKind, credentialHint := credentialMetadata(h.provider, failoverConfig, r)
	route := strings.TrimPrefix(r.URL.Path, h.provider.RoutePrefix())

	var interceptionID string
	var actorID string
	if actor != nil {
		actorID = actor.ID
	}
	state := forwardingState{credentialHint: credentialHint}
	start := time.Now()
	if h.record {
		interceptionID = uuid.NewString()
		if err := h.recorder.RecordInterception(ctx, &recorder.InterceptionRecord{
			ID:                          interceptionID,
			InitiatorID:                 actor.ID,
			Metadata:                    actor.Metadata,
			Provider:                    h.provider.Type(),
			ProviderName:                h.provider.Name(),
			Model:                       unknownModel,
			UserAgent:                   r.UserAgent(),
			Client:                      string(client),
			ClientSessionID:             sessionID,
			AgentFirewallSessionID:      firewallSessionID,
			AgentFirewallSequenceNumber: firewallSequence,
			CredentialKind:              credentialKind,
			CredentialHint:              credentialHint,
		}); err != nil {
			span.SetStatus(codes.Error, "failed to record interception")
			h.logger.Warn(ctx, "failed to record interception", slog.Error(err), slog.F("path", r.URL.Path))
			http.Error(w, "failed to record interception", http.StatusInternalServerError)
			return
		}
	}

	defer func() {
		panicValue := recover()
		aborted := h.finalize(ctx, span, r, actorID, string(client), route, interceptionID, start, &state, panicValue)
		if panicValue != nil {
			panic(panicValue)
		}
		if aborted {
			panic(http.ErrAbortHandler)
		}
	}()

	if h.record && h.metrics != nil {
		h.metrics.InterceptionsInflight.WithLabelValues(h.provider.Name(), unknownModel, route).Inc()
		state.metricsStarted = true
	}
	if !h.record && h.metrics != nil {
		h.metrics.PassthroughCount.WithLabelValues(h.provider.Name(), route, r.Method).Inc()
	}

	if failoverConfig.InjectAuthKey != nil {
		inject := failoverConfig.InjectAuthKey
		failoverConfig.InjectAuthKey = func(headers *http.Header, key string) {
			state.credentialHint = utils.MaskSecret(key)
			inject(headers, key)
		}
	}
	if failoverConfig.BuildKeyPoolResponse != nil {
		build := failoverConfig.BuildKeyPoolResponse
		failoverConfig.BuildKeyPoolResponse = func(poolErr *keypool.Error) *http.Response {
			state.poolErr = poolErr
			return build(poolErr)
		}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteRequest(pr, h.provider.RoutePrefix(), h.baseURL)
		},
		Transport:     keypool.NewKeyFailoverTransport(h.transport, failoverConfig),
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			state.status = resp.StatusCode
			state.body = newObservedBody(resp.Body, nil)
			resp.Body = state.body
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
			state.transportErr = proxyErr
			h.logger.Warn(req.Context(), "reverse proxy error", slog.Error(proxyErr), slog.F("path", req.URL.Path))
			span.SetStatus(codes.Error, "upstream proxy error")
			http.Error(rw, "upstream proxy error", http.StatusBadGateway)
		},
	}

	execErr := h.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		writer := &errorCapturingResponseWriter{ResponseWriter: rw}
		defer func() { state.writeErr = writer.err }()
		proxy.ServeHTTP(writer, r)
		return nil
	})
	state.execErr = execErr
}

var errDeclaredBodyTooLarge = xerrors.New("declared request body too large")

func captureRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.ContentLength > routing.MaxRequestBodyBytes {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, errDeclaredBodyTooLarge
	}
	if r.Body == nil {
		return nil, nil
	}
	body := r.Body
	defer body.Close()
	return io.ReadAll(http.MaxBytesReader(w, body, routing.MaxRequestBodyBytes))
}

func setReplayBody(r *http.Request, payload []byte) {
	newBody := func() io.ReadCloser { return io.NopCloser(bytes.NewReader(payload)) }
	r.Body = newBody()
	r.GetBody = func() (io.ReadCloser, error) { return newBody(), nil }
	r.ContentLength = int64(len(payload))
}

func rewriteRequest(pr *httputil.ProxyRequest, routePrefix string, baseURL *url.URL) {
	pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, routePrefix)
	if pr.In.URL.RawPath != "" {
		pr.Out.URL.RawPath = strings.TrimPrefix(pr.In.URL.RawPath, routePrefix)
	}
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetURL(baseURL)
	trace.SpanFromContext(pr.Out.Context()).SetAttributes(attribute.String(tracing.PassthroughUpstreamURL, pr.Out.URL.String()))
	utils.StripCoderHeaders(pr.Out.Header)
	stripProxyHeaders(pr.Out.Header)
	if _, ok := pr.Out.Header["User-Agent"]; !ok {
		// A nil value suppresses net/http's default User-Agent.
		pr.Out.Header["User-Agent"] = nil
	}
}

func stripProxyHeaders(headers http.Header) {
	for name := range headers {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-ai-bridge-actor") || lower == "forwarded" || strings.HasPrefix(lower, "x-forwarded-") {
			delete(headers, name)
		}
	}
}

func credentialMetadata(prov provider.Provider, failoverConfig keypool.KeyFailoverConfig, r *http.Request) (kind, hint string) {
	byok := prov.Type() == config.ProviderCopilot
	if failoverConfig.IsBYOK != nil {
		byok = failoverConfig.IsBYOK(r)
	}
	if !byok {
		return recorder.CredentialKindCentralized, recorder.CredentialHintFailoverKey
	}
	secret := ""
	if prov.Type() == config.ProviderAnthropic {
		secret = r.Header.Get("X-Api-Key")
	}
	if secret == "" {
		secret = utils.ExtractBearerToken(r.Header.Get("Authorization"))
	}
	if secret == "" {
		secret = r.Header.Get(prov.AuthHeader())
	}
	return recorder.CredentialKindBYOK, utils.MaskSecret(secret)
}

type forwardingState struct {
	status         int
	credentialHint string
	poolErr        error
	transportErr   error
	execErr        error
	writeErr       error
	body           *observedBody
	metricsStarted bool
}

func (h *forwardingHandler) finalize(ctx context.Context, span trace.Span, r *http.Request, actorID, client, route, interceptionID string, start time.Time, state *forwardingState, panicValue any) bool {
	aborted, terminalErr := terminalError(r, state, panicValue)
	if !h.record {
		return aborted
	}
	if h.metrics != nil {
		if state.metricsStarted {
			h.metrics.InterceptionsInflight.WithLabelValues(h.provider.Name(), unknownModel, route).Dec()
		}
		h.metrics.InterceptionDuration.WithLabelValues(h.provider.Name(), unknownModel).Observe(time.Since(start).Seconds())
	}

	errType, errMessage := interceptionerror.Categorize(h.provider, terminalErr, state.status)
	status := metrics.InterceptionCountStatusCompleted
	if terminalErr != nil || (state.status >= 400) {
		status = metrics.InterceptionCountStatusFailed
		span.SetStatus(codes.Error, errMessage)
	}
	if h.metrics != nil {
		h.metrics.InterceptionCount.WithLabelValues(h.provider.Name(), unknownModel, status, route, r.Method, actorID, client).Inc()
	}

	async := recorder.NewAsyncRecorder(h.recorder, recorder.DefaultAsyncTimeout)
	_ = async.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
		ID: interceptionID, CredentialHint: state.credentialHint,
		ErrorType: errType, ErrorMessage: errMessage,
	})
	async.Wait()
	return aborted
}

func terminalError(r *http.Request, state *forwardingState, panicValue any) (bool, error) {
	var abortErr error
	switch {
	case r.Context().Err() != nil && (state.body == nil || !state.body.eof):
		abortErr = r.Context().Err()
	case state.body != nil && state.body.readErr != nil:
		abortErr = state.body.readErr
	case state.writeErr != nil:
		abortErr = state.writeErr
	case panicValue != nil:
		abortErr = xerrors.Errorf("proxy stream aborted: %v", panicValue)
	case state.body != nil && !state.body.eof:
		abortErr = xerrors.New("proxy stream aborted before EOF")
	}

	switch {
	case state.poolErr != nil:
		return abortErr != nil, state.poolErr
	case state.execErr != nil:
		return abortErr != nil, state.execErr
	case abortErr != nil:
		return true, abortErr
	case state.transportErr != nil:
		return false, state.transportErr
	default:
		return false, nil
	}
}

type errorCapturingResponseWriter struct {
	http.ResponseWriter
	err error
}

func (w *errorCapturingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

func (w *errorCapturingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *errorCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, xerrors.New("response writer does not support hijacking")
	}
	return h.Hijack()
}

func (w *errorCapturingResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
