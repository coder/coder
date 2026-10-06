package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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
	"github.com/coder/quartz"
)

type forwardingHandler struct {
	provider  provider.Provider
	logger    slog.Logger
	tracer    trace.Tracer
	transport http.RoundTripper
	failover  keypool.KeyFailoverConfig
	proxy     *httputil.ReverseProxy
	inflight  *aibridge.InflightGate
	recorder  recorder.Recorder
	breaker   *circuitbreaker.ProviderCircuitBreakers
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
		tracer:    tracer,
		transport: apidump.NewPassthroughMiddleware(transport, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal()),
		failover:  prov.KeyFailoverConfig(logger),
		inflight:  inflight,
		recorder:  rec,
		breaker:   circuitbreaker.NewProviderCircuitBreakers(prov.Name(), prov.CircuitBreakerConfig(), logger, m),
	}
	h.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteForwardingURL(pr, prov.RoutePrefix(), baseURL)
			pr.Out.Header = prepareForwardingHeaders(pr)
		},
		Transport:     h,
		FlushInterval: -1,
		ErrorLog:      slog.Stdlib(context.Background(), logger.With(slog.F("provider", prov.Name())), slog.LevelWarn),
	}
	return h, nil
}

// RoundTrip forwards through the provider's key pool, or uses the selected BYOK
// credential without retrying against pooled keys.
func (h *forwardingHandler) RoundTrip(r *http.Request) (*http.Response, error) {
	return keypool.NewKeyFailoverTransport(h.transport, h.failover).RoundTrip(r)
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

	var err error
	defer func() { h.finishForwarding(ctx, record, err) }()
	route := strings.TrimPrefix(r.URL.Path, "/"+h.provider.Name())
	err = h.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		outbound, _ := h.prepareForwarding(r, cred)
		return h.forward(rw, outbound)
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

func (*forwardingHandler) forward(w http.ResponseWriter, _ *http.Request) error {
	// TODO: forward only when lifecycle recording is connected.
	http.Error(w, "bridged routes are not yet implemented in proxy mode", http.StatusNotImplemented)
	return nil
}

func (*forwardingHandler) finishForwarding(context.Context, *recorder.InterceptionRecord, error) {
	// TODO: finalize the outcome and lifecycle records, including breaker rejection.
}
