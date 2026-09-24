// Package routing provides shared provider routing for AI Gateway handlers.
package routing

import (
	"fmt"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/provider"
)

const (
	// ErrorCodeProviderDisabled is the code written in the response body when a
	// request targets a configured-but-disabled provider. Paired with HTTP 503.
	ErrorCodeProviderDisabled = "provider_disabled"
)

const metricMethodOther = "OTHER"

// MetricMethod returns a bounded HTTP method label.
func MetricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return method
	default:
		return metricMethodOther
	}
}

// NewProviderMux registers mode-independent provider errors. Callers add
// their enabled-provider routes to the returned mux.
func NewProviderMux(providers []provider.Provider, logger slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	for _, prov := range providers {
		// Match the bare provider name so disabled providers also reject
		// paths outside their normal /v1 subtree.
		if !prov.Enabled() {
			mux.HandleFunc(fmt.Sprintf("/%s/", prov.Name()), disabledProviderHandler(prov.Name(), logger))
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug(r.Context(), "route not supported", slog.F("path", r.URL.Path), slog.F("method", r.Method))
		http.Error(w, fmt.Sprintf("route not supported: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
	})
	return mux
}

// disabledProviderHandler returns 503 with a body containing
// [ErrorCodeProviderDisabled] and the provider name for every request
// targeting name.
func disabledProviderHandler(name string, logger slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug(r.Context(), "refusing request for disabled ai provider",
			slog.F("provider", name),
			slog.F("path", r.URL.Path),
			slog.F("method", r.Method),
		)
		http.Error(w, fmt.Sprintf("%s: AI provider %q is disabled", ErrorCodeProviderDisabled, name), http.StatusServiceUnavailable)
	}
}
