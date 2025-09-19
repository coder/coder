package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/x/aibridged"
)

// RegisterInMemoryAIBridgedHTTPHandler mounts [aibridged.Server]'s HTTP router onto
// [API]'s router, so that requests to aibridged will be relayed from Coder's API server
// to the in-memory aibridged.
func (api *API) RegisterInMemoryAIBridgedHTTPHandler(srv *aibridged.Server) {
	if srv == nil {
		panic("aibridged cannot be nil")
	}

	if api.RootHandler == nil {
		panic("api.RootHandler cannot be nil")
	}

	aibridgeEndpoint := "/api/experimental/aibridge"

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentAIBridge))
		r.HandleFunc("/*", http.StripPrefix(aibridgeEndpoint, srv).ServeHTTP)
	})

	api.RootHandler.Mount(aibridgeEndpoint, r)
}
