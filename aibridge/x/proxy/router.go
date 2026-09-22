// Package proxy provides stateless AI Gateway request routing.
package proxy

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
)

// Router is an [http.Handler] which serves the AI Gateway routes of an
// immutable snapshot of providers. It holds no per-user or per-request state,
// so a single instance serves every actor and is replaced wholesale when the
// provider configuration changes.
//
// Router has no concept of authentication or authorization; the caller is
// responsible for authenticating requests before they reach the router.
//
// Router is safe for concurrent use.
type Router struct {
	mux *http.ServeMux

	// providers is an owned copy of the provider list, fixed at construction.
	providers  []provider.Provider
	transports []*http.Transport
}

var _ http.Handler = (*Router)(nil)

// NewRouter creates a [*Router] for the given providers, whose
// names must be valid and unique.
//
// Each configured-but-disabled provider serves a 503 sentinel on every path
// under its name. Enabled providers have no routes registered yet, so their
// requests reach the catch-all 404.
func NewRouter(providers []provider.Provider, logger slog.Logger) (*Router, error) {
	if err := provider.ValidateProviders(providers); err != nil {
		return nil, err
	}

	snapshot := slices.Clone(providers)
	mux := routing.NewProviderMux(snapshot, logger)

	return &Router{
		mux:       mux,
		providers: snapshot,
	}, nil
}

// buildRouter constructs the complete proxy router while the exported
// constructor retains its placeholder behavior.
func buildRouter(providers []provider.Provider, rec recorder.Recorder, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer) (_ *Router, outErr error) {
	if err := provider.ValidateProviders(providers); err != nil {
		return nil, err
	}

	snapshot := slices.Clone(providers)
	router := &Router{mux: routing.NewProviderMux(snapshot, logger), providers: snapshot}
	defer func() {
		if outErr != nil {
			router.CloseIdleConnections()
		}
	}()

	for _, prov := range snapshot {
		if !prov.Enabled() {
			continue
		}
		if prov.Type() == config.ProviderBedrock {
			logger.Warn(context.Background(), "skipping unsupported Bedrock provider in proxy mode", slog.F("provider", prov.Name()))
			continue
		}
		if rec == nil && len(prov.BridgedRoutes()) > 0 {
			return nil, xerrors.Errorf("configure provider %q: recorder is required for bridged routes", prov.Name())
		}

		baseHandler, transport, err := newForwardingHandler(prov, logger, m, tracer, "")
		if err != nil {
			return nil, err
		}
		router.transports = append(router.transports, transport)
		breakers := circuitbreaker.NewProviderCircuitBreakersWithObservability(
			prov.Name(), prov.CircuitBreakerConfig(), logger, m,
		)

		for _, path := range prov.BridgedRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q bridged route: %w", prov.Name(), err)
			}
			handler := *baseHandler
			handler.breaker = breakers
			handler.recorder = rec
			handler.logger = logger.Named("proxy." + prov.Name())
			handler.record = true
			handler.metricRoute = strings.TrimPrefix(route, "/"+prov.Name())
			router.mux.Handle(route, &handler)
		}
		for _, path := range prov.PassthroughRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q passthrough route: %w", prov.Name(), err)
			}
			handler := *baseHandler
			handler.logger = logger.Named("passthrough." + prov.Name())
			handler.metricRoute = path
			router.mux.Handle(route, &handler)
		}
	}
	return router, nil
}

// ServeHTTP serves the routes registered for the router's providers.
func (p *Router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Cap the body as it is read; routes that do not read it retain their status.
	if r.Body != nil {
		r.Body = http.MaxBytesReader(rw, r.Body, routing.MaxRequestBodyBytes)
	}
	p.mux.ServeHTTP(rw, r)
}

// KeyPools returns the non-nil key pools from this router's provider snapshot.
func (p *Router) KeyPools() []*keypool.Pool {
	return provider.CollectKeyPools(p.providers)
}

// CloseIdleConnections closes idle upstream connections owned by this router.
func (p *Router) CloseIdleConnections() {
	for _, transport := range p.transports {
		transport.CloseIdleConnections()
	}
}
