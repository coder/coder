// Package proxy provides stateless AI Gateway request routing.
package proxy

import (
	"net/http"
	"slices"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
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
	providers []provider.Provider
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

// ServeHTTP serves the routes registered for the router's providers.
func (p *Router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// Cap the body as it is read; routes that do not read it retain their status.
	r.Body = http.MaxBytesReader(rw, r.Body, routing.MaxRequestBodyBytes)
	p.mux.ServeHTTP(rw, r)
}

// KeyPools returns the non-nil key pools from this router's provider snapshot.
func (p *Router) KeyPools() []*keypool.Pool {
	return provider.CollectKeyPools(p.providers)
}
