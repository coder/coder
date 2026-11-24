package agentpreview

import (
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
	// beforeOpen runs with the served path up to an element after the
	// element passes its Lstat checks and before it is opened, so a test can
	// swap the element in that window.
	beforeOpen func(prefix string)
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

	published, status, err := api.publish(req.Path)
	if err != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: "Could not open preview.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, status, published)
}
