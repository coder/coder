package agentfiles

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentgit"
)

// API exposes file-related operations performed through the agent.
type API struct {
	logger     slog.Logger
	filesystem afero.Fs
	pathStore  *agentgit.PathStore
}

func NewAPI(logger slog.Logger, filesystem afero.Fs, pathStore *agentgit.PathStore) *API {
	api := &API{
		logger:     logger,
		filesystem: filesystem,
		pathStore:  pathStore,
	}
	return api
}

// Routes returns the HTTP handler for file-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()

	r.Post("/list-directory", api.HandleLS)
	r.Get("/resolve-path", api.HandleResolvePath)
	r.Get("/read-file", api.HandleReadFile)
	r.Get("/read-file-lines", api.HandleReadFileLines)
	r.Post("/write-file", api.HandleWriteFile)
	r.Post("/edit-files", api.HandleEditFiles)

	return r
}
