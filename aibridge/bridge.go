package aibridge

import (
	"context"
	"fmt"
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
	"github.com/coder/aibridge/circuitbreaker"
	aibcontext "github.com/coder/aibridge/context"
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/metrics"
	"github.com/coder/aibridge/provider"
	"github.com/coder/aibridge/recorder"
	"github.com/coder/aibridge/tracing"
)

const (
	// The duration after which an async recording will be aborted.
	recordingTimeout = time.Second * 5
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
func NewRequestBridge(ctx context.Context, providers []provider.Provider, rec recorder.Recorder, mcpProxy mcp.ServerProxier, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer) (*RequestBridge, error) {
	if err := validateProviders(providers); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	for _, prov := range providers {
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
	return &RequestBridge{
		mux:            mux,
		logger:         logger,
		mcpProxy:       mcpProxy,
		inflightCtx:    inflightCtx,
		inflightCancel: cancel,

		closed: make(chan struct{}, 1),
	}, nil
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

		interceptor, err := p.CreateInterceptor(w, r.WithContext(ctx), tracer)
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

		actor := aibcontext.ActorFromContext(ctx)
		if actor == nil {
			logger.Warn(ctx, "no actor found in context")
			http.Error(w, "no actor found", http.StatusBadRequest)
			return
		}

		traceAttrs := interceptor.TraceAttributes(r)
		span.SetAttributes(traceAttrs...)
		ctx = tracing.WithInterceptionAttributesInContext(ctx, traceAttrs)
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
			Metadata:              actor.Metadata,
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
			slog.F("interception_id", interceptor.ID()),
			slog.F("user_agent", r.UserAgent()),
			slog.F("streaming", interceptor.Streaming()),
			slog.F("credential_kind", string(cred.Kind)),
			slog.F("credential_hint", cred.Hint),
			slog.F("credential_length", cred.Length),
		)

		log.Debug(ctx, "interception started")
		if m != nil {
			m.InterceptionsInflight.WithLabelValues(p.Name(), interceptor.Model(), route).Add(1)
			defer func() {
				m.InterceptionsInflight.WithLabelValues(p.Name(), interceptor.Model(), route).Sub(1)
			}()
		}

		// Process request with circuit breaker protection if configured
		if err := cbs.Execute(route, interceptor.Model(), w, func(rw http.ResponseWriter) error {
			return interceptor.ProcessRequest(rw, r)
		}); err != nil {
			if m != nil {
				m.InterceptionCount.WithLabelValues(p.Name(), interceptor.Model(), metrics.InterceptionCountStatusFailed, route, r.Method, actor.ID, string(client)).Add(1)
			}
			span.SetStatus(codes.Error, fmt.Sprintf("interception failed: %v", err))
			log.Warn(ctx, "interception failed", slog.Error(err))
		} else {
			if m != nil {
				m.InterceptionCount.WithLabelValues(p.Name(), interceptor.Model(), metrics.InterceptionCountStatusCompleted, route, r.Method, actor.ID, string(client)).Add(1)
			}
			log.Debug(ctx, "interception ended")
		}

		_ = asyncRecorder.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{ID: interceptor.ID().String()})

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
