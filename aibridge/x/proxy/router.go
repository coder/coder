// Package proxy provides stateless AI Gateway request routing.
package proxy

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

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
	"github.com/coder/coder/v2/aibridge/utils"
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
		if !prov.Enabled() {
			continue
		}
		if prov.Type() == config.ProviderBedrock {
			// Bedrock signing is implemented only by interception mode. Keep the
			// existing catch-all 404 behavior, but make the skipped provider visible.
			logger.Warn(context.Background(), "skipping unsupported Bedrock provider in proxy mode", slog.F("provider", prov.Name()))
			continue
		}
		if rec == nil && len(prov.BridgedRoutes()) > 0 {
			return nil, xerrors.Errorf("configure provider %q: recorder is required for bridged routes", prov.Name())
		}
		failoverConfig := prov.KeyFailoverConfig(logger)
		if failoverConfig.Pool != nil {
			switch {
			case failoverConfig.IsBYOK == nil:
				return nil, xerrors.Errorf("configure provider %q key failover: IsBYOK callback is required with a key pool", prov.Name())
			case failoverConfig.InjectAuthKey == nil:
				return nil, xerrors.Errorf("configure provider %q key failover: InjectAuthKey callback is required with a key pool", prov.Name())
			case failoverConfig.BuildKeyPoolResponse == nil:
				return nil, xerrors.Errorf("configure provider %q key failover: BuildKeyPoolResponse callback is required with a key pool", prov.Name())
			}
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

		transport := utils.NewStreamingTransport()
		// Preserve encoded upstream bytes so response observation sees the same
		// stream sent to the client and never depends on transparent decompression.
		transport.DisableCompression = true
		router.transports = append(router.transports, transport)
		dumpTransport := apidump.NewPassthroughMiddleware(transport, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal())

		breakers := circuitbreaker.NewProviderCircuitBreakersWithObservability(
			prov.Name(), prov.CircuitBreakerConfig(), logger, m,
		)

		for _, path := range prov.BridgedRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q bridged route: %w", prov.Name(), err)
			}
			metricRoute := strings.TrimPrefix(route, "/"+prov.Name())
			router.mux.Handle(route, &forwardingHandler{
				provider: prov, baseURL: baseURL, transport: dumpTransport,
				breaker: breakers, failover: failoverConfig, recorder: rec,
				logger:  logger.Named("proxy." + prov.Name()),
				metrics: m, tracer: tracer, record: true, metricRoute: metricRoute,
			})
		}
		for _, path := range prov.PassthroughRoutes() {
			route, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q passthrough route: %w", prov.Name(), err)
			}
			router.mux.Handle(route, &forwardingHandler{
				provider: prov, baseURL: baseURL, transport: dumpTransport,
				failover: failoverConfig,
				logger:   logger.Named("passthrough." + prov.Name()), metrics: m,
				tracer: tracer, metricRoute: path,
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
