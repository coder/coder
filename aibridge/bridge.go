package aibridge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
)

const (
	// The duration after which an async recording will be aborted.
	recordingTimeout = time.Second * 5

	// ErrorCodeProviderDisabled is the code written in the response
	// body when a request targets a configured-but-disabled provider.
	// Paired with HTTP 503.
	ErrorCodeProviderDisabled = "provider_disabled"
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
	mux    *http.ServeMux
	logger slog.Logger

	mcpProxy mcp.ServerProxier

	inflightReqs atomic.Int32
	inflightWG   sync.WaitGroup // For graceful shutdown.

	inflightCtx    context.Context
	inflightCancel func()

	shutdownOnce sync.Once
	closed       chan struct{}
}

var _ http.Handler = &RequestBridge{}

// validProviderName matches names containing only lowercase alphanumeric characters and hyphens.
var validProviderName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// validateProviders checks that provider names are valid and unique.
func validateProviders(providers []provider.Provider) error {
	names := make(map[string]bool, len(providers))
	for _, prov := range providers {
		name := prov.Name()
		if !validProviderName.MatchString(name) {
			return xerrors.Errorf("invalid provider name %q: must contain only lowercase alphanumeric characters and hyphens", name)
		}
		if names[name] {
			return xerrors.Errorf("duplicate provider name: %q", name)
		}
		names[name] = true
	}
	return nil
}

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
	if err := validateProviders(providers); err != nil {
		return nil, err
	}

	var options requestBridgeOptions
	for _, opt := range opts {
		opt(&options)
	}
	hooks := policyHooks{byProvider: options.byProvider, resolver: options.resolver}
	// Policy pipelines are sourced per-provider from the DB by the caller (see
	// coderd/aibridged) and passed via WithPolicyHooks. A provider with no
	// configured pipeline runs no policies (pass-through).

	mux := http.NewServeMux()

	for _, prov := range providers {
		// Disabled providers serve a 503 sentinel on every path under
		// "/<name>/". Bound to the bare name (not RoutePrefix) so paths
		// outside the provider's normal "/v1" subtree are also caught.
		if !prov.Enabled() {
			prefix := fmt.Sprintf("/%s/", prov.Name())
			mux.HandleFunc(prefix, disabledProviderHandler(prov.Name(), logger))
			continue
		}
		// Create per-provider circuit breaker if configured
		cfg := prov.CircuitBreakerConfig()
		providerName := prov.Name()
		onChange := func(endpoint, model string, from, to gobreaker.State) {
			logger.Info(context.Background(), "circuit breaker state change",
				slog.F("provider", providerName),
				slog.F("endpoint", endpoint),
				slog.F("model", model),
				slog.F("from", from.String()),
				slog.F("to", to.String()),
			)
			if m != nil {
				m.CircuitBreakerState.WithLabelValues(providerName, endpoint, model).Set(circuitbreaker.StateToGaugeValue(to))
				if to == gobreaker.StateOpen {
					m.CircuitBreakerTrips.WithLabelValues(providerName, endpoint, model).Inc()
				}
			}
		}
		cbs := circuitbreaker.NewProviderCircuitBreakers(providerName, cfg, onChange, m)

		// Add the known provider-specific routes which are bridged (i.e. intercepted and augmented).
		for _, path := range prov.BridgedRoutes() {
			handler := newInterceptionProcessor(prov, cbs, rec, mcpProxy, logger, m, tracer, hooks)
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				logger.Error(ctx, "failed to join path",
					slog.Error(err),
					slog.F("provider", providerName),
					slog.F("prefix", prov.RoutePrefix()),
					slog.F("path", path),
				)
				return nil, xerrors.Errorf("failed to configure provider '%v': failed to join bridged path: %w", providerName, err)
			}
			mux.Handle(route, handler)
		}

		// Any requests which passthrough to this will be reverse-proxied to the upstream.
		//
		// We have to whitelist the known-safe routes because an API key with elevated privileges (i.e. admin) might be
		// configured, so we should just reverse-proxy known-safe routes.
		ftr := newPassthroughRouter(prov, logger.Named(fmt.Sprintf("passthrough.%s", prov.Name())), m, tracer)
		for _, path := range prov.PassthroughRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				logger.Error(ctx, "failed to join path",
					slog.Error(err),
					slog.F("provider", providerName),
					slog.F("prefix", prov.RoutePrefix()),
					slog.F("path", path),
				)
				return nil, xerrors.Errorf("failed to configure provider '%v': failed to join passed through path: %w", providerName, err)
			}
			mux.Handle(route, http.StripPrefix(prov.RoutePrefix(), ftr))
		}
	}

	// Catch-all.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Warn(r.Context(), "route not supported", slog.F("path", r.URL.Path), slog.F("method", r.Method))
		http.Error(w, fmt.Sprintf("route not supported: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
	})

	inflightCtx, cancel := context.WithCancel(context.Background())
	return &RequestBridge{
		mux:            mux,
		logger:         logger,
		mcpProxy:       mcpProxy,
		inflightCtx:    inflightCtx,
		inflightCancel: cancel,

		closed: make(chan struct{}, 1),
	}, nil
}

// disabledProviderHandler returns 503 with a body containing
// [ErrorCodeProviderDisabled] and the provider name for every request
// targeting name.
func disabledProviderHandler(name string, logger slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug(r.Context(), "refusing request for disabled ai provider",
			slog.F("provider", name),
			slog.F("path", r.URL.Path),
			slog.F("method", r.Method),
		)
		http.Error(w, fmt.Sprintf("%s: AI provider %q is disabled", ErrorCodeProviderDisabled, name), http.StatusServiceUnavailable)
	}
}

// newInterceptionProcessor returns an [http.HandlerFunc] which is capable of creating a new interceptor and processing a given request
// using [Provider] p, recording all usage events using [Recorder] rec.
// If cbs is non-nil, circuit breaker protection is applied per endpoint/model tuple.
func newInterceptionProcessor(p provider.Provider, cbs *circuitbreaker.ProviderCircuitBreakers, rec recorder.Recorder, mcpProxy mcp.ServerProxier, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, hooks policyHooks) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "Intercept")
		defer span.End()
		r = r.WithContext(ctx)

		// Read the body once upfront. GuessSessionID, policy hooks, and
		// CreateInterceptor all consume it from the shared payload.
		client := GuessClient(r)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("failed to read request body: %v", err))
			logger.Warn(ctx, "failed to read request body", slog.Error(err), slog.F("path", r.URL.Path))
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		payload := intercept.NewPayload(body)
		sessionID := GuessSessionID(client, r, payload)

		actor := aibcontext.ActorFromContext(ctx)
		if actor == nil {
			logger.Warn(ctx, "no actor found in context")
			http.Error(w, "no actor found", http.StatusBadRequest)
			return
		}

		// Resolve which pipeline version evaluates this request. Absence =
		// pass-through: a provider with no configured pipelines runs no policies.
		// An owner-only header rehearses a specific version against real traffic
		// (§10.9); a non-owner sending it is rejected loudly rather than silently
		// falling through to the active version.
		ph := hooks.byProvider[p.Name()]
		if version := strings.TrimSpace(r.Header.Get(HeaderAIGatewayPipelineVersion)); version != "" {
			// Strip the header so it is never forwarded upstream.
			r.Header.Del(HeaderAIGatewayPipelineVersion)
			if !actorIsOwner(actor) {
				logger.Warn(ctx, "non-owner attempted version-targeted evaluation",
					slog.F("actor", actor.ID))
				http.Error(w, "version-targeted evaluation is restricted to owners", http.StatusForbidden)
				return
			}
			if hooks.resolver == nil {
				http.Error(w, "version-targeted evaluation is unavailable", http.StatusBadRequest)
				return
			}
			resolved, err := hooks.resolver.ResolvePipelineVersion(ctx, p.Name(), version)
			if err != nil {
				if xerrors.Is(err, ErrPipelineVersionNotFound) {
					logger.Warn(ctx, "unknown or foreign pipeline version requested",
						slog.F("pipeline_version", version))
					http.Error(w, "unknown pipeline version", http.StatusBadRequest)
					return
				}
				logger.Error(ctx, "resolve pipeline version", slog.Error(err),
					slog.F("pipeline_version", version))
				http.Error(w, "failed to resolve pipeline version", http.StatusInternalServerError)
				return
			}
			logger.Debug(ctx, "evaluating version-targeted pipeline",
				slog.F("pipeline_version", version), slog.F("staged", true))
			ph = resolved
		}

		// Run the policy hooks before creating the interceptor: they may block
		// the request, rewrite the payload, or attach annotations.
		payload, annotations, modifications, blocked := hooks.apply(w, r, payload, actor, ph, p.Name(), m, logger)
		if blocked {
			return
		}

		// Attach the per-tool-call gate (pre-tool hook) to the context so the
		// streaming interceptor can evaluate each assembled, client-bound tool
		// call before releasing it. A nil gate (no pre-tool pipeline) is a
		// no-op. The gate captures the identity and accumulated annotations; the
		// pre-tool envelope gates the tool call itself, not the request body.
		if gate := hooks.toolGate(ph, p.Name(), actor, annotations, m, logger); gate != nil {
			ctx = intercept.WithToolGate(ctx, gate)
			r = r.WithContext(ctx)
		}

		interceptor, err := p.CreateInterceptor(w, r, payload, tracer)
		if err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("failed to create interceptor: %v", err))
			logger.Warn(ctx, "failed to create interceptor", slog.Error(err), slog.F("path", r.URL.Path))
			http.Error(w, fmt.Sprintf("failed to create %q interceptor", r.URL.Path), http.StatusInternalServerError)
			return
		}

		if m != nil {
			start := time.Now()
			defer func() {
				m.InterceptionDuration.WithLabelValues(p.Name(), interceptor.Model()).Observe(time.Since(start).Seconds())
			}()
		}

		traceAttrs := interceptor.TraceAttributes(r)
		span.SetAttributes(traceAttrs...)
		ctx = tracing.WithInterceptionAttributesInContext(ctx, traceAttrs)
		// Attach the interception ID to the context so every log line
		// emitted with this context can be correlated to the interception.
		ctx = slog.With(ctx, slog.F("interception_id", interceptor.ID()))
		r = r.WithContext(ctx)

		// Record usage in the background to not block request flow.
		asyncRecorder := recorder.NewAsyncRecorder(logger, rec, recordingTimeout)
		asyncRecorder.WithMetrics(m)
		asyncRecorder.WithProvider(p.Name())
		asyncRecorder.WithModel(interceptor.Model())
		asyncRecorder.WithInitiatorID(actor.ID)
		asyncRecorder.WithClient(string(client))
		interceptor.Setup(logger, asyncRecorder, mcpProxy)

		cred := interceptor.Credential()
		if err := rec.RecordInterception(ctx, &recorder.InterceptionRecord{
			ID:                    interceptor.ID().String(),
			InitiatorID:           actor.ID,
			Metadata:              metadataWithPolicyResults(actor.Metadata, annotations, modifications),
			Model:                 interceptor.Model(),
			Provider:              p.Type(),
			ProviderName:          p.Name(),
			UserAgent:             r.UserAgent(),
			Client:                string(client),
			ClientSessionID:       sessionID,
			CorrelatingToolCallID: interceptor.CorrelatingToolCallID(),
			CredentialKind:        string(cred.Kind),
			CredentialHint:        cred.Hint,
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
			slog.F("credential_kind", string(cred.Kind)),
		)

		// Log BYOK credentials. Centralized credentials are set by
		// the key failover loop.
		credLogFields := []slog.Field{}
		if cred.Kind == intercept.CredentialKindBYOK {
			credLogFields = append(credLogFields,
				slog.F("credential_hint", cred.Hint),
				slog.F("credential_length", cred.Length),
			)
		}
		log.Debug(ctx, "interception started", credLogFields...)
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
		// For centralized, the hint now reflects the last attempted
		// key from the failover loop.
		credHint := interceptor.Credential().Hint
		credLen := interceptor.Credential().Length
		if execErr != nil {
			if m != nil {
				m.InterceptionCount.WithLabelValues(p.Name(), interceptor.Model(), metrics.InterceptionCountStatusFailed, route, r.Method, actor.ID, string(client)).Add(1)
			}
			span.SetStatus(codes.Error, fmt.Sprintf("interception failed: %v", execErr))
			log.Warn(ctx, "interception failed", slog.Error(execErr), slog.F("credential_hint", credHint), slog.F("credential_length", credLen))
		} else {
			if m != nil {
				m.InterceptionCount.WithLabelValues(p.Name(), interceptor.Model(), metrics.InterceptionCountStatusCompleted, route, r.Method, actor.ID, string(client)).Add(1)
			}
			log.Debug(ctx, "interception ended", slog.F("credential_hint", credHint), slog.F("credential_length", credLen))
		}

		_ = asyncRecorder.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
			ID:             interceptor.ID().String(),
			CredentialHint: credHint,
		})

		// Ensure all recording have completed before completing request.
		asyncRecorder.Wait()
	}
}

// ServeHTTP exposes the internal http.Handler, which has all [Provider]s' routes registered.
// It also tracks inflight requests.
func (b *RequestBridge) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	select {
	case <-b.closed:
		http.Error(rw, "server closed", http.StatusInternalServerError)
		return
	default:
	}

	// We want to abide by the context passed in without losing any of its
	// functionality, but we still want to link our shutdown context to each
	// request.
	ctx := mergeContexts(r.Context(), b.inflightCtx)

	b.inflightReqs.Add(1)
	b.inflightWG.Add(1)
	defer func() {
		b.inflightReqs.Add(-1)
		b.inflightWG.Done()
	}()

	b.mux.ServeHTTP(rw, r.WithContext(ctx))
}

// Shutdown will attempt to gracefully shutdown. This entails waiting for all requests to
// complete, and shutting down the MCP server proxier.
// TODO: add tests.
func (b *RequestBridge) Shutdown(ctx context.Context) error {
	var err error
	b.shutdownOnce.Do(func() {
		// Prevent any new requests from being accepted.
		close(b.closed)

		// Wait for inflight requests to complete or context cancellation.
		done := make(chan struct{})
		go func() {
			b.inflightWG.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			// Cancel all inflight requests, if any are still running.
			b.logger.Debug(ctx, "shutdown context canceled; canceling inflight requests", slog.Error(ctx.Err()))
			b.inflightCancel()
			<-done
			err = ctx.Err()
		case <-done:
		}

		if b.mcpProxy != nil {
			// It's ok that we reuse the ctx here even if it's done, since the
			// Shutdown method will just immediately use the more aggressive close
			// since the ctx is already expired.
			err = multierror.Append(err, b.mcpProxy.Shutdown(ctx))
		}
	})

	return err
}

func (b *RequestBridge) InflightRequests() int32 {
	return b.inflightReqs.Load()
}

// mergeContexts merges two contexts together, so that if either is canceled
// the returned context is canceled. The context values will only be used from
// the first context.
func mergeContexts(base, other context.Context) context.Context {
	ctx, cancel := context.WithCancel(base)
	go func() {
		defer cancel()
		select {
		case <-base.Done():
		case <-other.Done():
		}
	}()
	return ctx
}
