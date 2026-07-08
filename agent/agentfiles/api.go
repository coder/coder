package agentfiles

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API exposes file-related operations performed through the agent.
type API struct {
	logger         slog.Logger
	filesystem     afero.Fs
	pathStore      *agentgit.PathStore
	envInfo        usershell.EnvInfoer
	zipFilesLimits workspacesdk.ZipFilesLimits
}

// Option configures the API.
type Option func(*API)

// WithZipFilesLimits overrides the zip files collection limits. Intended
// for tests.
func WithZipFilesLimits(limits workspacesdk.ZipFilesLimits) Option {
	return func(api *API) {
		api.zipFilesLimits = limits
	}
}

func NewAPI(logger slog.Logger, filesystem afero.Fs, pathStore *agentgit.PathStore, envInfo usershell.EnvInfoer, opts ...Option) *API {
	api := &API{
		logger:         logger,
		filesystem:     filesystem,
		pathStore:      pathStore,
		envInfo:        envInfo,
		zipFilesLimits: defaultZipFilesLimits,
	}
	for _, opt := range opts {
		opt(api)
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
	r.Post("/zip-files", api.HandleZipFiles)

	return r
}
