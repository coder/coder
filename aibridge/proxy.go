package aibridge

import (
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
// are added in AIGOV-615.
func NewProxyRouter(providers []Provider, logger slog.Logger) (*ProxyRouter, error) {
	if err := validateProviders(providers); err != nil {
		return nil, err
	}

	snapshot := slices.Clone(providers)
	mux := newProviderMux(snapshot, logger)

	return &ProxyRouter{
		mux:       mux,
		providers: snapshot,
	}, nil
}

// ServeHTTP serves the routes registered for the router's providers.
func (p *ProxyRouter) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	serveProviderRequest(rw, r, p.mux)
}

// KeyPools returns the key pools of the router's providers, skipping
// providers which have none. It feeds [keypool.NewStateCollector].
func (p *ProxyRouter) KeyPools() []*keypool.Pool {
	return CollectKeyPools(p.providers)
}
