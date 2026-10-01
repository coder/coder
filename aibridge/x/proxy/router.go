// Package proxy provides stateless AI Gateway request routing.
package proxy

import (
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
	providers []provider.Provider
}

var _ http.Handler = (*Router)(nil)

// NewRouter creates a [*Router] for the given set of providers.
// Provider names must be valid and unique.
//
// Disabled providers serve 503 on every path under their name. Enabled
// providers proxy passthrough routes upstream, bridged routes return 404
// after validation succeeds. Bedrock bridged routes return 404 without validation.
// All routes reuse the same inflight gate across a server's snapshots.
// Shutdown drains all admitted requests.
// rec is shared across requests and must read identity from the request context.
func NewRouter(providers []provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder) (*Router, error) {
	if err := provider.ValidateProviders(providers); err != nil {
		return nil, err
	}

	snapshot := slices.Clone(providers)
	mux := http.NewServeMux()
	mux.Handle("/", inflight.Middleware(routing.NewProviderMux(snapshot, logger)))
	router := &Router{providers: snapshot, mux: mux}
	for _, prov := range snapshot {
		if !prov.Enabled() {
			continue
		}

		bridged := inflight.Middleware(http.NotFoundHandler())
		// Bedrock is excluded before validation because proxy mode does not
		// forward it. Providers without bridged routes need no handler.
		if prov.Type() != config.ProviderBedrock && len(prov.BridgedRoutes()) > 0 {
			bridged = newForwardingHandler(prov, logger, m, tracer, inflight, rec)
		}
		for _, path := range prov.BridgedRoutes() {
			pattern, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q bridged route: %w", prov.Name(), err)
			}
			mux.Handle(pattern, bridged)
		}

		passthrough := inflight.Middleware(http.StripPrefix(prov.RoutePrefix(), aibridge.NewPassthroughHandler(prov, logger.Named(fmt.Sprintf("passthrough.%s", prov.Name())), m, tracer)))
		for _, path := range prov.PassthroughRoutes() {
			pattern, err := url.JoinPath(prov.RoutePrefix(), path)
			if err != nil {
				return nil, xerrors.Errorf("configure provider %q passthrough route: %w", prov.Name(), err)
			}
			mux.Handle(pattern, passthrough)
		}
	}

	return router, nil
}

// ServeHTTP dispatches a request through the provider snapshot's shared mux.
// Authentication and actor identity are supplied by the caller on r.Context().
func (p *Router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = http.MaxBytesReader(rw, r.Body, routing.MaxRequestBodyBytes)
	}
	p.mux.ServeHTTP(rw, r)
}

// KeyPools returns the non-nil key pools from this router's provider snapshot.
func (p *Router) KeyPools() []*keypool.Pool {
	return provider.CollectKeyPools(p.providers)
}
