// Package proxy provides stateless AI Gateway request routing.
package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/coderd/util/xurl"
)

// Router serves the AI Gateway routes of an immutable snapshot of providers.
// It holds no per-user or per-request state. Single instance serves every
// actor and is replaced wholesale when the provider configuration changes.
//
// Router has no concept of authentication or authorization. The caller is
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

// NewRouter creates a [*Router] for the given set of providers.
// Provider names must be valid and unique.
//
// Disabled providers serve 503 on every path under their name. Passthrough
// routes use the passthrough handler. Bridged routes forward with the shared
// recorder and authenticated actor from the request context. Bedrock bridged
// routes return 404 without validation regardless of credentials. All snapshots
// must share the server's inflight gate so shutdown drains their requests.
func NewRouter(ctx context.Context, providers []provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder) (_ *Router, outErr error) {
	if err := provider.ValidateProviders(providers); err != nil {
		return nil, err
	}

	snapshot := slices.Clone(providers)
	mux := http.NewServeMux()
	mux.Handle("/", inflight.Middleware(routing.NewProviderMux(snapshot, logger)))
	router := &Router{providers: snapshot, mux: mux}
	defer func() {
		if outErr != nil {
			router.CloseIdleConnections()
		}
	}()
	for _, prov := range snapshot {
		if !prov.Enabled() {
			continue
		}

		bridged := inflight.Middleware(http.NotFoundHandler())
		// Bedrock is excluded before validation because proxy mode does not
		// forward it. Providers without bridged routes need no handler.
		if prov.Type() != config.ProviderBedrock && len(prov.BridgedRoutes()) > 0 {
			transport := utils.NewStreamingTransport()
			transport.DisableCompression = true
			router.transports = append(router.transports, transport)
			handler, err := newForwardingHandler(prov, logger, m, tracer, inflight, rec, transport)
			if err != nil {
				return nil, err
			}
			bridged = handler
		}
		for _, path := range prov.BridgedRoutes() {
			pattern, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q bridged route: %w", prov.Name(), err)
			}
			mux.Handle(pattern, bridged)
			logger.Debug(ctx, "registered bridged route",
				slog.F("provider", prov.Name()),
				slog.F("path", pattern),
			)
		}

		passthrough := inflight.Middleware(http.StripPrefix(prov.RoutePrefix(), aibridge.NewPassthroughHandler(prov, logger.Named(fmt.Sprintf("passthrough.%s", prov.Name())), m, tracer)))
		for _, path := range prov.PassthroughRoutes() {
			pattern, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q passthrough route: %w", prov.Name(), err)
			}
			mux.Handle(pattern, passthrough)
			logger.Debug(ctx, "registered passthrough route",
				slog.F("provider", prov.Name()),
				slog.F("path", pattern),
			)
		}
	}

	return router, nil
}

// ServeHTTP dispatches a request through the provider snapshot's shared mux.
// Authentication and actor identity are supplied by the caller on r.Context().
func (p *Router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if xurl.ContainsEncodedPath(r.URL) {
		http.Error(rw, routing.InvalidPathMessage, http.StatusBadRequest)
		return
	}
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = http.MaxBytesReader(rw, r.Body, routing.MaxRequestBodyBytes)
	}
	p.mux.ServeHTTP(rw, r)
}

// KeyPools returns the non-nil key pools from this router's provider snapshot.
func (p *Router) KeyPools() []*keypool.Pool {
	return provider.CollectKeyPools(p.providers)
}

// CloseIdleConnections releases idle connections owned by bridged forwarding.
// Active requests retain their connections until they finish.
func (p *Router) CloseIdleConnections() {
	for _, transport := range p.transports {
		transport.CloseIdleConnections()
	}
}
