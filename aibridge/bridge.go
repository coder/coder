package aibridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/quartz"
)

const (
	// The duration after which an async recording will be aborted.
	recordingTimeout = time.Second * 5

	// maxRequestBodyBytes caps the request body size for AI Gateway
	// provider endpoints to prevent denial-of-service via memory exhaustion.
	// Anthropic enforces 32 MB on the direct API, 30 MB on Vertex AI,
	// and 20 MB on Amazon Bedrock.
	// See https://docs.anthropic.com/en/api/overview#request-size-limits
	// OpenAI and GitHub Copilot do not document an equivalent HTTP body size limit.
	// Using highest documented provider limit (32 MiB).
	//
	// NOTE: aibridge does not currently proxy file-upload endpoints
	// (e.g. /v1/files). Those endpoints accept much larger bodies
	// (up to 500 MB for Anthropic, 50 MB for OpenAI). If file-upload
	// routes are added, they will need a per-route limit instead of
	// this single global cap.
	maxRequestBodyBytes = 32 << 20 // 32 MiB

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

	// inflightMu orders inflightWG.Add (ServeHTTP, read-held) before
	// close(b.closed) (Shutdown, write-held), so Add never races Wait.
	inflightMu sync.RWMutex

	inflightCtx    context.Context
	inflightCancel func()

	clock quartz.Clock

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
			handler := newInterceptionProcessor(prov, cbs, rec, mcpProxy, logger, m, tracer)
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
	b := &RequestBridge{
		mux:            mux,
		logger:         logger,
		mcpProxy:       mcpProxy,
		inflightCtx:    inflightCtx,
		inflightCancel: cancel,
		clock:          quartz.NewReal(),

		closed: make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

type RequestBridgeOption func(*RequestBridge)

func WithClock(clock quartz.Clock) RequestBridgeOption {
	return func(b *RequestBridge) { b.clock = clock }
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
func newInterceptionProcessor(p provider.Provider, cbs *circuitbreaker.ProviderCircuitBreakers, rec recorder.Recorder, mcpProxy mcp.ServerProxier, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "Intercept")
		defer span.End()

		// We execute this before CreateInterceptor since the interceptors
		// read the request body and don't reset them.
		client := GuessClient(r)
		sessionID := GuessSessionID(client, r)

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
				writeRequestBodyTooLarge(w)
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
		asyncRecorder := recorder.NewAsyncRecorder(logger, rec, recordingTimeout)
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

// writeRequestBodyTooLarge writes a human-readable 413 response indicating that
// the request body exceeded maxRequestBodyBytes.
func writeRequestBodyTooLarge(w http.ResponseWriter) {
	http.Error(w, fmt.Sprintf(
		"Request body too large. The maximum allowed request body size is %dMiB.",
		maxRequestBodyBytes>>20,
	), http.StatusRequestEntityTooLarge)
}

// ServeHTTP exposes the internal http.Handler, which has all [Provider]s' routes registered.
// It also tracks inflight requests.
func (b *RequestBridge) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	b.inflightMu.RLock()
	select {
	case <-b.closed:
		b.inflightMu.RUnlock()
		http.Error(rw, "server closed", http.StatusInternalServerError)
		return
	default:
	}

	// Trap point for deterministic race tests.
	_ = b.clock.Now("serve_admission")

	b.inflightReqs.Add(1)
	b.inflightWG.Add(1)
	b.inflightMu.RUnlock()
	defer func() {
		b.inflightReqs.Add(-1)
		b.inflightWG.Done()
	}()

	// We want to abide by the context passed in without losing any of its
	// functionality, but we still want to link our shutdown context to each
	// request.
	ctx := mergeContexts(r.Context(), b.inflightCtx)

	// Enforce the request body size limit. MaxBytesReader counts bytes as
	// they are read from the connection and fails when the limit is exceeded.
	r.Body = http.MaxBytesReader(rw, r.Body, maxRequestBodyBytes)
	b.mux.ServeHTTP(rw, r.WithContext(ctx))
}

// Shutdown will attempt to gracefully shutdown. This entails waiting for all requests to
// complete, and shutting down the MCP server proxier.
// TODO: add tests.
func (b *RequestBridge) Shutdown(ctx context.Context) error {
	var err error
	b.shutdownOnce.Do(func() {
		// Close under inflightMu so no ServeHTTP sits mid-admission (see inflightMu).
		b.inflightMu.Lock()
		close(b.closed)
		b.inflightMu.Unlock()

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

// extractAgentFirewallHeaders reads and parses the Agent Firewall
// correlation headers from the request. Both headers must be present
// together with a valid UUID session ID and a non-negative int32
// sequence number, or both must be absent. Partial or malformed headers
// return an error so the caller can reject the request (fail closed).
func extractAgentFirewallHeaders(r *http.Request) (sessionID *string, seqNumber *int32, err error) {
	rawSessionID := r.Header.Get(agplaibridge.HeaderAgentFirewallSessionID)
	rawSeqNumber := r.Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber)

	hasSessionID := rawSessionID != ""
	hasSeqNumber := rawSeqNumber != ""

	switch {
	case !hasSessionID && !hasSeqNumber:
		// Neither header present; request did not traverse Agent Firewall.
		return nil, nil, nil
	case hasSessionID && !hasSeqNumber:
		return nil, nil, xerrors.Errorf("agent firewall session ID header present without sequence number")
	case !hasSessionID && hasSeqNumber:
		return nil, nil, xerrors.Errorf("agent firewall sequence number header present without session ID")
	}

	// Both headers present; validate the session ID is a UUID. Storing an
	// invalid value would silently drop the firewall correlation to NULL
	// downstream, so reject it here instead.
	if _, parseErr := uuid.Parse(rawSessionID); parseErr != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall session ID %q: %w", rawSessionID, parseErr)
	}

	// Parse the sequence number.
	n, err := strconv.ParseInt(rawSeqNumber, 10, 32)
	if err != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall sequence number %q: %w", rawSeqNumber, err)
	}
	if n < 0 {
		return nil, nil, xerrors.Errorf("invalid agent firewall sequence number %q: must be non-negative", rawSeqNumber)
	}

	n32 := int32(n)
	return &rawSessionID, &n32, nil
}
