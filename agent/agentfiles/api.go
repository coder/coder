package agentfiles

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"

	"cdr.dev/slog/v3"
)

// API exposes file-related operations performed through the agent.
type API struct {
	logger     slog.Logger
	filesystem afero.Fs
}

func NewAPI(logger slog.Logger, filesystem afero.Fs) *API {
	api := &API{
		logger:     logger,
		filesystem: filesystem,
	}
	return api
}

// Routes returns the HTTP handler for file-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()

	r.Post("/list-directory", api.HandleLS)
	r.Get("/read-file", api.HandleReadFile)
	r.Post("/write-file", api.HandleWriteFile)
	r.Post("/edit-files", api.HandleEditFiles)

	return r
}
