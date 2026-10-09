package agentacp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// API serves the agent-local ACP harness catalog.
type API struct{ manager *Manager }

// NewAPI constructs the agent-local HTTP API.
func NewAPI(m *Manager) *API { return &API{manager: m} }

// Routes returns handlers mounted under /api/v0/acp.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/harnesses", func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(r.Context(), w, http.StatusOK, api.manager.Catalog())
	})
	return r
}
