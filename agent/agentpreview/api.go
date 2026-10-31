package agentpreview

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// maxOpenRequestBytes bounds the JSON body of a publication request, which
// only carries a path.
const maxOpenRequestBytes = 16 << 10

// API publishes workspace files as static previews. Routes handles
// publication requests on the agent API; ServeHTTP serves the previews.
type API struct {
	// mu guards closed and previews.
	mu       sync.Mutex
	closed   bool
	previews map[string]*preview
}

// NewAPI creates a preview API handler.
func NewAPI() *API {
	return &API{previews: make(map[string]*preview)}
}

// Close releases all preview roots. Later requests receive 404 responses.
func (api *API) Close() error {
	api.mu.Lock()
	defer api.mu.Unlock()

	api.closed = true

	var errs []error
	for _, p := range api.previews {
		// Wait for in-flight openFile traversals; files they returned stay
		// readable. closed stops a concurrent republish from swapping in a
		// new root.
		p.rootMu.Lock()
		p.closed = true
		errs = append(errs, p.root.Close())
		p.rootMu.Unlock()
	}
	clear(api.previews)

	return errors.Join(errs...)
}

// Routes returns the HTTP handler for preview publication requests.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/", api.handleOpen)
	return r
}

func (api *API) handleOpen(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req workspacesdk.OpenPreviewRequest
	body := http.MaxBytesReader(rw, r.Body, maxOpenRequestBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	id, canonical, err := api.open(req.Path)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Could not open preview.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.OpenPreviewResponse{ID: id, Path: canonical})
}
