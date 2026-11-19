package agentpreview

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API publishes workspace files as previews and serves them.
type API struct {
	mu        sync.RWMutex
	idsByPath map[string]string
	previews  map[string]preview
}

// NewAPI creates a preview API handler.
func NewAPI() *API {
	return &API{
		idsByPath: make(map[string]string),
		previews:  make(map[string]preview),
	}
}

// Routes returns the handler that publishes previews and serves them under
// /{id}/.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/", api.handlePublish)
	r.Get("/{id}/*", api.handleServe)
	r.Head("/{id}/*", api.handleServe)
	return r
}

func (api *API) handlePublish(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req workspacesdk.OpenPreviewRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	id, path, err := api.publish(req.Path)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Could not open preview.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.OpenPreviewResponse{ID: id, Path: path})
}
