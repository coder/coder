package httpmw

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type (
	httpRouteInfoKey struct{}
)

type httpRouteInfo struct {
	Route  string
	Method string
}

// ExtractHTTPRoute retrieves just the HTTP route pattern from context.
// Returns empty string if not set.
func ExtractHTTPRoute(ctx context.Context) string {
	ri, _ := ctx.Value(httpRouteInfoKey{}).(httpRouteInfo)
	return ri.Route
}

// ExtractHTTPMethod retrieves just the HTTP method from context.
// Returns empty string if not set.
func ExtractHTTPMethod(ctx context.Context) string {
	ri, _ := ctx.Value(httpRouteInfoKey{}).(httpRouteInfo)
	return ri.Method
}

// HTTPRoute is middleware that stores the HTTP route pattern and method in
// context for use by downstream handlers and services (e.g. prometheus).
func HTTPRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := getRoutePattern(r)
		ctx := context.WithValue(r.Context(), httpRouteInfoKey{}, httpRouteInfo{
			Route:  route,
			Method: r.Method,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return ""
	}

	routePath := r.URL.Path
	if r.URL.RawPath != "" {
		routePath = r.URL.RawPath
	}

	tctx := chi.NewRouteContext()
	routes := rctx.Routes
	if routes != nil && !routes.Match(tctx, r.Method, routePath) {
		// No matching pattern. /api/* requests will be matched as "UNKNOWN"
		// All other ones will be matched as "STATIC".
		if strings.HasPrefix(routePath, "/api/") {
			return "UNKNOWN"
		}
		return "STATIC"
	}

	// tctx has the updated pattern, since Match mutates it
	return tctx.RoutePattern()
}
