package aibridge

import (
	"fmt"
	"net/http"
	"slices"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/keypool"
)

// ProxyRouter is an [http.Handler] which serves the AI Gateway routes of an
// immutable snapshot of providers. Unlike [RequestBridge] it holds no
// per-user or per-request state, so a single instance serves every actor and
// is replaced wholesale when the provider configuration changes.
//
// ProxyRouter has no concept of authentication or authorization; the caller is
// responsible for authenticating requests before they reach the router.
//
// ProxyRouter is safe for concurrent use.
type ProxyRouter struct {
	mux *http.ServeMux

	// providers is an owned copy of the provider list, fixed at construction.
	providers []Provider
}

var _ http.Handler = (*ProxyRouter)(nil)

// NewProxyRouter creates a [*ProxyRouter] for the given providers, whose
// names must be valid and unique.
//
// Each configured-but-disabled provider serves a 503 sentinel on every path
// under its name. Enabled providers have no routes registered yet, so their
// requests reach the catch-all 404 until the bridged and passthrough routes
// are added.
func NewProxyRouter(providers []Provider, logger slog.Logger) (*ProxyRouter, error) {
	if err := validateProviders(providers); err != nil {
		return nil, err
	}

	snapshot := slices.Clone(providers)
	mux := http.NewServeMux()

	for _, prov := range snapshot {
		// Disabled providers serve a 503 sentinel on every path under
		// "/<name>/". Bound to the bare name (not RoutePrefix) so paths
		// outside the provider's normal "/v1" subtree are also caught.
		if !prov.Enabled() {
			mux.HandleFunc(fmt.Sprintf("/%s/", prov.Name()), disabledProviderHandler(prov.Name(), logger))
		}
	}

	// Catch-all.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Warn(r.Context(), "route not supported", slog.F("path", r.URL.Path), slog.F("method", r.Method))
		http.Error(w, fmt.Sprintf("route not supported: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
	})

	return &ProxyRouter{
		mux:       mux,
		providers: snapshot,
	}, nil
}

// ServeHTTP serves the routes registered for the router's providers.
func (p *ProxyRouter) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Enforce the request body size limit. MaxBytesReader counts bytes as
	// they are read from the connection and fails when the limit is exceeded;
	// routes built on httputil.ReverseProxy answer 413 through
	// [proxyErrorHandler].
	r.Body = http.MaxBytesReader(rw, r.Body, maxRequestBodyBytes)
	p.mux.ServeHTTP(rw, r)
}

// KeyPools returns the key pools of the router's providers, skipping
// providers which have none. It feeds [keypool.NewStateCollector].
func (p *ProxyRouter) KeyPools() []*keypool.Pool {
	pools := make([]*keypool.Pool, 0, len(p.providers))
	for _, prov := range p.providers {
		if pool := prov.KeyPool(); pool != nil {
			pools = append(pools, pool)
		}
	}
	return pools
}
