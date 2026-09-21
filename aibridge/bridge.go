package aibridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/clientmeta"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

// RequestBridge is an [http.Handler] which is capable of masquerading as AI providers' APIs;
// specifically, OpenAI's & Anthropic's at present.
// RequestBridge intercepts requests to - and responses from - these upstream services to provide
// a centralized governance layer.
//
// RequestBridge has no concept of authentication or authorization. It does have a concept of identity,
// in the narrow sense that it expects an [actor] to be defined in the context, to record the initiator
// of each interception.
//
// RequestBridge is safe for concurrent use.
type RequestBridge struct {
	handler http.Handler
	logger  slog.Logger

	mcpProxy mcp.ServerProxier

	// inflight provides the shared admission and drain machinery.
	inflight *InflightGate

	shutdownOnce sync.Once
}

var _ http.Handler = &RequestBridge{}

// NewRequestBridge creates a new *[RequestBridge] and registers the HTTP routes defined by the given providers.
// Any routes which are requested but not registered will be reverse-proxied to the upstream service.
//
// A [intercept.Recorder] is also required to record prompt, tool, and token use.
//
// mcpProxy will be closed when the [RequestBridge] is closed.
//
// Circuit breaker configuration is obtained from each provider's CircuitBreakerConfig() method.
// Providers returning nil will not have circuit breaker protection.
func NewRequestBridge(ctx context.Context, providers []provider.Provider, rec recorder.Recorder, mcpProxy mcp.ServerProxier, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, opts ...RequestBridgeOption) (*RequestBridge, error) {
	if err := provider.ValidateProviders(providers); err != nil {
		return nil, err
	}

	mux := routing.NewProviderMux(providers, logger)

	for _, prov := range providers {
		// The shared mux already returns 503 for disabled providers.
		if !prov.Enabled() {
			continue
		}

		cbs := circuitbreaker.NewProviderCircuitBreakersWithObservability(
			prov.Name(), prov.CircuitBreakerConfig(), logger, m,
		)

		// Add the known provider-specific routes which are bridged (i.e. intercepted and augmented).
		for _, path := range prov.BridgedRoutes() {
			handler := newInterceptionProcessor(prov, cbs, rec, mcpProxy, logger, m, tracer)
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				logger.Error(ctx, "failed to join path",
					slog.Error(err),
					slog.F("provider", prov.Name()),
					slog.F("prefix", prov.RoutePrefix()),
					slog.F("path", path),
				)
				return nil, xerrors.Errorf("failed to configure provider '%v': failed to join bridged path: %w", prov.Name(), err)
			}
			mux.Handle(route, handler)
		}

		// Any requests which passthrough to this will be reverse-proxied to the upstream.
		//
		// We have to whitelist the known-safe routes because an API key with elevated privileges (i.e. admin) might be
		// configured, so we should just reverse-proxy known-safe routes.
		ftr := newPassthroughRouter(prov, "/", logger.Named(fmt.Sprintf("passthrough.%s", prov.Name())), m, tracer)
		for _, path := range prov.PassthroughRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				logger.Error(ctx, "failed to join path",
					slog.Error(err),
					slog.F("provider", prov.Name()),
					slog.F("prefix", prov.RoutePrefix()),
					slog.F("path", path),
				)
				return nil, xerrors.Errorf("failed to configure provider '%v': failed to join passed through path: %w", prov.Name(), err)
			}
			mux.Handle(route, http.StripPrefix(prov.RoutePrefix(), ftr))
		}
	}

	b := &RequestBridge{
		logger:   logger,
		mcpProxy: mcpProxy,
		inflight: NewInflightGate(logger),
	}
	for _, opt := range opts {
		opt(b)
	}
	b.handler = b.inflight.Middleware(http.MaxBytesHandler(mux, routing.MaxRequestBodyBytes))
	return b, nil
}

type RequestBridgeOption func(*RequestBridge)

func WithClock(clock quartz.Clock) RequestBridgeOption {
	return func(b *RequestBridge) { b.inflight.clock = clock }
}

// newInterceptionProcessor returns an [http.HandlerFunc] which is capable of creating a new interceptor and processing a given request
// using [Provider] p, recording all usage events using [Recorder] rec.
// If cbs is non-nil, circuit breaker protection is applied per endpoint/model tuple.
func newInterceptionProcessor(p provider.Provider, cbs *circuitbreaker.ProviderCircuitBreakers, rec recorder.Recorder, mcpProxy mcp.ServerProxier, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "Intercept")
		defer span.End()

		// We execute this before CreateInterceptor since the interceptors
		// read the request body and don't reset them.
		client := GuessClient(r)
		sessionID := GuessSessionID(client, r)

		if isWebSocketUpgrade(r) {
			route := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/%s", p.Name()))
			logger.Debug(ctx, "rejecting unsupported WebSocket upgrade",
				slog.F("provider", p.Name()),
				slog.F("route", route),
				slog.F("client", string(client)),
				slog.F("client_session_id", sessionID),
			)
			http.Error(w, "WebSocket transport is not supported, use HTTP", http.StatusNotImplemented)
			return
		}

		// Read and validate Agent Firewall correlation headers. The
		// values are captured here and recorded below; the headers
		// themselves are stripped from the upstream request by
		// PrepareClientHeaders. Fail closed: reject the request if the
		// headers are partial or malformed.
		agentFirewallSessionID, agentFirewallSeqNumber, err := extractAgentFirewallHeaders(r)
		if err != nil {
			logger.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.Error(err))
			http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
			return
		}

		interceptor, err := p.CreateInterceptor(w, r.WithContext(ctx), tracer)
		if err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("failed to create interceptor: %v", err))
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				routing.WriteRequestBodyTooLarge(ctx, w)
			} else {
				logger.Warn(ctx, "failed to create interceptor", slog.Error(err), slog.F("path", r.URL.Path))
				http.Error(w, fmt.Sprintf("failed to create %q interceptor", r.URL.Path), http.StatusInternalServerError)
			}
			return
		}

		if m != nil {
			start := time.Now()
			defer func() {
				m.InterceptionDuration.WithLabelValues(p.Name(), interceptor.Model()).Observe(time.Since(start).Seconds())
			}()
		}

		actor := aibcontext.ActorFromContext(ctx)
		if actor == nil {
			logger.Warn(ctx, "no actor found in context")
			http.Error(w, "no actor found", http.StatusBadRequest)
			return
		}

		cred := interceptor.Credential()
		traceAttrs := interceptor.TraceAttributes(r)
		span.SetAttributes(traceAttrs...)
		ctx = tracing.WithInterceptionAttributesInContext(ctx, traceAttrs)
		// Attach the interception ID and credential kind to the context so every
		// log line emitted with it can be correlated to the interception.
		ctx = slog.With(ctx,
			slog.F("interception_id", interceptor.ID()),
			slog.F("credential_kind", string(cred.Kind())),
		)
		r = r.WithContext(ctx)

		// Record usage in the background to not block request flow.
		asyncRecorder := recorder.NewAsyncRecorder(rec, recorder.DefaultAsyncTimeout)
		asyncRecorder.WithMetrics(m)
		asyncRecorder.WithProvider(p.Name())
		asyncRecorder.WithModel(interceptor.Model())
		asyncRecorder.WithInitiatorID(actor.ID)
		asyncRecorder.WithClient(string(client))
		interceptor.Setup(logger, asyncRecorder, mcpProxy)

		if err := rec.RecordInterception(ctx, &recorder.InterceptionRecord{
			ID:                          interceptor.ID().String(),
			InitiatorID:                 actor.ID,
			Metadata:                    actor.Metadata,
			Model:                       interceptor.Model(),
			Provider:                    p.Type(),
			ProviderName:                p.Name(),
			UserAgent:                   r.UserAgent(),
			Client:                      string(client),
			ClientSessionID:             sessionID,
			CorrelatingToolCallID:       interceptor.CorrelatingToolCallID(),
			AgentFirewallSessionID:      agentFirewallSessionID,
			AgentFirewallSequenceNumber: agentFirewallSeqNumber,
			CredentialKind:              string(cred.Kind()),
			CredentialHint:              cred.Hint(),
		}); err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("failed to record interception: %v", err))
			logger.Warn(ctx, "failed to record interception", slog.Error(err))
			http.Error(w, "failed to record interception", http.StatusInternalServerError)
			return
		}

		route := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/%s", p.Name()))
		log := logger.With(
			slog.F("route", route),
			slog.F("provider", p.Name()),
			slog.F("user_agent", r.UserAgent()),
			slog.F("streaming", interceptor.Streaming()),
		)

		log.Debug(ctx, "interception started",
			slog.F("credential_hint", cred.Hint()),
			slog.F("credential_length", cred.Length()),
		)
		if m != nil {
			m.InterceptionsInflight.WithLabelValues(p.Name(), interceptor.Model(), route).Add(1)
			defer func() {
				m.InterceptionsInflight.WithLabelValues(p.Name(), interceptor.Model(), route).Sub(1)
			}()
		}

		// Process request with circuit breaker protection if configured
		execErr := cbs.Execute(route, interceptor.Model(), w, func(rw http.ResponseWriter) error {
			return interceptor.ProcessRequest(rw, r)
		})
		// For a centralized pool, the hint now reflects the last key the
		// failover loop attempted.
		credCtx := intercept.WithCredentialInfo(ctx, cred)
		errType, errMsg := categorizeInterceptionError(p, execErr)
		if execErr != nil {
			if m != nil {
				m.InterceptionCount.WithLabelValues(p.Name(), interceptor.Model(), metrics.InterceptionCountStatusFailed, route, r.Method, actor.ID, string(client)).Add(1)
			}
			span.SetStatus(codes.Error, fmt.Sprintf("interception failed: %v", execErr))
			log.Warn(credCtx, "interception failed", slog.Error(execErr), slog.F("error_type", string(errType)))
		} else {
			if m != nil {
				m.InterceptionCount.WithLabelValues(p.Name(), interceptor.Model(), metrics.InterceptionCountStatusCompleted, route, r.Method, actor.ID, string(client)).Add(1)
			}
			log.Debug(credCtx, "interception ended")
		}

		_ = asyncRecorder.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
			ID:             interceptor.ID().String(),
			CredentialHint: cred.Hint(),
			ErrorType:      errType,
			ErrorMessage:   errMsg,
		})

		// Ensure all recording have completed before completing request.
		asyncRecorder.Wait()
	}
}

// isWebSocketUpgrade reports whether r is a WebSocket opening handshake.
func isWebSocketUpgrade(r *http.Request) bool {
	return clientmeta.IsWebSocketUpgrade(r)
}

// ServeHTTP exposes the internal http.Handler, which has all [Provider]s' routes registered.
// It also tracks inflight requests.
func (b *RequestBridge) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	b.handler.ServeHTTP(rw, r)
}

// Shutdown drains requests until ctx expires, then cancels remaining requests
// without waiting for their handlers to return. MCP cleanup is attempted even
// while canceled handlers are still running.
func (b *RequestBridge) Shutdown(ctx context.Context) error {
	var err error
	b.shutdownOnce.Do(func() {
		err = b.inflight.Shutdown(ctx)
		if err != nil {
			b.logger.Debug(ctx, "shutdown context canceled; canceling inflight requests", slog.Error(err))
		}
		b.inflight.Close()

		if b.mcpProxy != nil {
			// It's ok that we reuse the ctx here even if it's done, since the
			// Shutdown method will just immediately use the more aggressive close
			// since the ctx is already expired.
			err = multierror.Append(err, b.mcpProxy.Shutdown(ctx))
		}
	})

	return err
}

// extractAgentFirewallHeaders validates and returns Agent Firewall correlation
// metadata without forwarding the headers to provider-specific processing.
func extractAgentFirewallHeaders(r *http.Request) (*string, *int32, error) {
	return clientmeta.ExtractAgentFirewallHeaders(r)
}
