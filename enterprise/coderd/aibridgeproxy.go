package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// API route prefixes for the AI Gateway Proxy and legacy AI Bridge Proxy endpoints.
const (
	AIGatewayProxyPath = agplaibridge.AIGatewayRootPath + "/proxy"
	AIBridgeProxyPath  = agplaibridge.AIBridgeRootPath + "/proxy"
)

// RegisterInMemoryAIBridgeProxydHTTPHandler mounts [aibridgeproxyd.Server]'s HTTP handler
// onto [API]'s router, so that requests to aibridgedproxy will be relayed from Coder's API server
// to the in-memory aibridgedproxy.
func (api *API) RegisterInMemoryAIBridgeProxydHTTPHandler(srv http.Handler) {
	if srv == nil {
		panic("aibridgeproxyd cannot be nil")
	}

	api.aibridgeproxydHandler = srv
}

// aibridgeProxyHTTPHandler returns the legacy /api/v2/aibridge/proxy route tree.
// Kept for backward compatibility only.
//
// NOTE: new endpoints must be registered on the enterprise API
// handler under /api/v2/ai-gateway, not in this shared route builder.
func aibridgeProxyHTTPHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return aiGatewayProxyRoutes(api, AIBridgeProxyPath, middlewares...)
}

// aiGatewayProxyHTTPHandler returns the /api/v2/ai-gateway/proxy route tree.
// This shares the same route builder as /aibridge/proxy for endpoints that
// existed before the rename.
//
// NOTE: new endpoints must be registered on the enterprise API
// handler under /api/v2/ai-gateway, not in this shared route builder.
func aiGatewayProxyHTTPHandler(api *API, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return aiGatewayProxyRoutes(api, AIGatewayProxyPath, middlewares...)
}

// aiGatewayProxyRoutes builds the route tree for AI Gateway Proxy endpoints.
func aiGatewayProxyRoutes(api *API, stripPrefix string, middlewares ...func(http.Handler) http.Handler) func(r chi.Router) {
	return func(r chi.Router) {
		r.Use(api.RequireFeatureMW(codersdk.FeatureAIBridge))
		r.Use(middlewares...)

		r.HandleFunc("/*", func(rw http.ResponseWriter, r *http.Request) {
			// Check if the proxy is enabled.
			if !api.DeploymentValues.AI.BridgeProxyConfig.Enabled.Value() {
				httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
					Message: "AI Gateway Proxy is not enabled.",
				})
				return
			}

			// Check if the handler is registered.
			if api.aibridgeproxydHandler == nil {
				httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
					Message: "AI Gateway Proxy handler not mounted.",
				})
				return
			}

			// Strip the prefix and relay to the aibridgeproxyd handler.
			http.StripPrefix(stripPrefix, api.aibridgeproxydHandler).ServeHTTP(rw, r)
		})
	}
}
