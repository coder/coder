package aibridge

import (
	"fmt"
	"net/http"

	"cdr.dev/slog/v3"
)

// newProviderMux registers mode-independent provider errors. Callers add
// their enabled-provider routes to the returned mux.
func newProviderMux(providers []Provider, logger slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	for _, prov := range providers {
		// Match the bare provider name so disabled providers also reject
		// paths outside their normal /v1 subtree.
		if !prov.Enabled() {
			mux.HandleFunc(fmt.Sprintf("/%s/", prov.Name()), disabledProviderHandler(prov.Name(), logger))
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Warn(r.Context(), "route not supported", slog.F("path", r.URL.Path), slog.F("method", r.Method))
		http.Error(w, fmt.Sprintf("route not supported: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
	})
	return mux
}

// serveProviderRequest caps the body as it is read. Routes retain control of
// their response status, including sentinels that never read the body.
func serveProviderRequest(rw http.ResponseWriter, r *http.Request, next http.Handler) {
	r.Body = http.MaxBytesReader(rw, r.Body, maxRequestBodyBytes)
	next.ServeHTTP(rw, r)
}
