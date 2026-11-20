package agentpreview

import (
	"errors"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API publishes workspace files as previews and serves them.
type API struct {
	mu       sync.RWMutex
	previews map[appurl.PreviewID]preview
}

// NewAPI creates a preview API handler.
func NewAPI() *API {
	return &API{previews: make(map[appurl.PreviewID]preview)}
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

	published, err := api.publish(req.Path)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errIDTaken) {
			status = http.StatusConflict
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: "Could not open preview.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, published)
}
