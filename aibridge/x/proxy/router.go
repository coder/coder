// Package proxy provides stateless AI Gateway request routing.
package proxy

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/quartz"
)

// Router serves the AI Gateway routes for an immutable provider snapshot and
// owns the snapshot's upstream transports. It holds no per-user or per-request
// state, so one router can serve concurrent requests and in-flight requests can
// safely retain an old snapshot while a replacement is published.
//
// Router does not authenticate or authorize requests. The caller must establish
// the request actor and remove Coder authentication before dispatching to it.
type Router struct {
	mux        *http.ServeMux
	providers  []provider.Provider
	transports []*http.Transport
}

var _ http.Handler = (*Router)(nil)

// NewRouter creates a Router for valid, uniquely named providers. A recorder is
// required when any eligible enabled provider has bridged routes.
func NewRouter(providers []provider.Provider, rec recorder.Recorder, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer) (_ *Router, outErr error) {
	if err := provider.ValidateProviders(providers); err != nil {
		return nil, err
	}

	if tracer == nil {
		tracer = noop.NewTracerProvider().Tracer("proxy")
	}

	snapshot := slices.Clone(providers)
	router := &Router{mux: routing.NewProviderMux(snapshot, logger), providers: snapshot}
	defer func() {
		if outErr != nil {
			router.CloseIdleConnections()
		}
	}()

	for _, prov := range snapshot {
		if prov.Type() == config.ProviderBedrock || !prov.Enabled() {
			continue
		}
		if rec == nil && len(prov.BridgedRoutes()) > 0 {
			return nil, xerrors.Errorf("configure provider %q: recorder is required for bridged routes", prov.Name())
		}
		baseURL, err := url.Parse(prov.BaseURL())
		if err != nil {
			return nil, xerrors.Errorf("configure provider %q base URL: %w", prov.Name(), err)
		}
		if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
			return nil, xerrors.Errorf("configure provider %q base URL: absolute HTTP or HTTPS URL with host required", prov.Name())
		}
		if _, err := url.JoinPath(baseURL.Path, "/"); err != nil {
			return nil, xerrors.Errorf("configure provider %q base URL path: %w", prov.Name(), err)
		}

		transport := &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
			DisableCompression:    true,
		}
		router.transports = append(router.transports, transport)
		dumpTransport := apidump.NewPassthroughMiddleware(transport, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal())

		providerName := prov.Name()
		onChange := func(endpoint, model string, from, to gobreaker.State) {
			logger.Info(context.Background(), "circuit breaker state change",
				slog.F("provider", providerName), slog.F("endpoint", endpoint),
				slog.F("model", model), slog.F("from", from.String()), slog.F("to", to.String()))
			if m != nil {
				m.CircuitBreakerState.WithLabelValues(providerName, endpoint, model).Set(circuitbreaker.StateToGaugeValue(to))
				if to == gobreaker.StateOpen {
					m.CircuitBreakerTrips.WithLabelValues(providerName, endpoint, model).Inc()
				}
			}
		}
		breakers := circuitbreaker.NewProviderCircuitBreakers(providerName, prov.CircuitBreakerConfig(), onChange, m)

		for _, path := range prov.BridgedRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q bridged route: %w", prov.Name(), err)
			}
			router.mux.Handle(route, &forwardingHandler{
				provider: prov, baseURL: baseURL, transport: dumpTransport,
				breaker: breakers, recorder: rec, logger: logger.Named("proxy." + prov.Name()),
				metrics: m, tracer: tracer, record: true,
			})
		}
		for _, path := range prov.PassthroughRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q passthrough route: %w", prov.Name(), err)
			}
			router.mux.Handle(route, &forwardingHandler{
				provider: prov, baseURL: baseURL, transport: dumpTransport,
				logger: logger.Named("passthrough." + prov.Name()), metrics: m,
				tracer: tracer,
			})
		}
	}
	return router, nil
}

// ServeHTTP caps request bodies as they are consumed by registered routes.
func (p *Router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		r.Body = http.MaxBytesReader(rw, r.Body, routing.MaxRequestBodyBytes)
	}
	p.mux.ServeHTTP(rw, r)
}

// KeyPools returns the non-nil key pools in this provider snapshot.
func (p *Router) KeyPools() []*keypool.Pool {
	return provider.CollectKeyPools(p.providers)
}

// CloseIdleConnections closes idle upstream connections owned by this router.
func (p *Router) CloseIdleConnections() {
	for _, transport := range p.transports {
		transport.CloseIdleConnections()
	}
}
