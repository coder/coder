package proxy

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	aibclient "github.com/coder/coder/v2/aibridge/client"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
)

type forwardingHandler struct {
	provider provider.Provider
	logger   slog.Logger
	tracer   trace.Tracer
	inflight *aibridge.InflightGate
	recorder recorder.Recorder
	breaker  *circuitbreaker.ProviderCircuitBreakers
}

var _ http.Handler = (*forwardingHandler)(nil)

// newForwardingHandler constructs one bridged handler per provider snapshot.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder) *forwardingHandler {
	return &forwardingHandler{
		provider: prov,
		logger:   logger,
		tracer:   tracer,
		inflight: inflight,
		recorder: rec,
		breaker:  circuitbreaker.NewProviderCircuitBreakers(prov.Name(), prov.CircuitBreakerConfig(), logger, m),
	}
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
	route := strings.TrimPrefix(r.Pattern, "/"+h.provider.Name())
	err = h.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		outbound := h.prepareForwarding(r, cred)
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
		if r.Body != nil {
			_ = r.Body.Close()
		}
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
		http.Error(w, "no actor found", http.StatusBadRequest)
		return nil, nil
	}
	if h.recorder == nil {
		http.Error(w, "recorder unavailable", http.StatusInternalServerError)
		return nil, nil
	}
	cred, err := h.provider.ResolveCredential(r)
	if err != nil {
		http.Error(w, "upstream authentication unavailable", http.StatusBadGateway)
		return nil, nil
	}
	switch cred.(type) {
	case credential.BYOK, *credential.CentralizedPool:
	default:
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

func (*forwardingHandler) prepareForwarding(r *http.Request, _ credential.Credential) *http.Request {
	// TODO: prepare provider headers and a replayable request body.
	return r.Clone(r.Context())
}

func (*forwardingHandler) forward(w http.ResponseWriter, r *http.Request) error {
	// TODO: forward only when lifecycle recording is connected.
	http.NotFound(w, r)
	return nil
}

func (*forwardingHandler) finishForwarding(context.Context, *recorder.InterceptionRecord, error) {
	// TODO: finalize the outcome and lifecycle records, including breaker rejection.
}
