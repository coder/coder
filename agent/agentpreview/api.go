package agentpreview

import (
	"errors"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API publishes workspace files as previews and serves them.
type API struct {
	logger slog.Logger

	mu        sync.RWMutex
	closed    bool
	idsByPath map[string]string
	previews  map[string]*preview
}

// NewAPI creates a preview API handler.
func NewAPI(logger slog.Logger) *API {
	return &API{
		logger:    logger,
		idsByPath: make(map[string]string),
		previews:  make(map[string]*preview),
	}
}

// Close unpublishes every preview. Later requests to serve a preview receive
// 404 responses, and later publish requests fail.
func (api *API) Close() error {
	api.mu.Lock()
	defer api.mu.Unlock()

	api.closed = true

	var errs []error
	for _, p := range api.previews {
		errs = append(errs, p.root.Close())
	}
	clear(api.previews)

	return errors.Join(errs...)
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

	id, canonical, err := api.publish(req.Path)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Could not open preview.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.OpenPreviewResponse{ID: id, Path: canonical})
}
