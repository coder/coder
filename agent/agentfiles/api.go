package agentfiles

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agenthooks"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API exposes file-related operations performed through the agent.
type API struct {
	logger            slog.Logger
	filesystem        afero.Fs
	pathStore         *agentgit.PathStore
	envInfo           usershell.EnvInfoer
	bundleFilesLimits workspacesdk.BundleFilesLimits
	// preToolHook, when set, runs workspace hooks before read_file,
	// write_file, and edit_files do their work. PROTOTYPE (CODAGT-1083).
	preToolHook agenthooks.PreToolHook
}

// Option configures the API.
type Option func(*API)

// WithPreToolHook installs the workspace hook runner.
func WithPreToolHook(hook agenthooks.PreToolHook) Option {
	return func(api *API) {
		api.preToolHook = hook
	}
}

// WithBundleFilesLimits overrides the bundle files collection limits.
func WithBundleFilesLimits(limits workspacesdk.BundleFilesLimits) Option {
	return func(api *API) {
		api.bundleFilesLimits = limits
	}
}

// WithEnvInfo overrides how the agent user's home directory is resolved.
func WithEnvInfo(envInfo usershell.EnvInfoer) Option {
	return func(api *API) {
		api.envInfo = envInfo
	}
}

func NewAPI(logger slog.Logger, filesystem afero.Fs, pathStore *agentgit.PathStore, opts ...Option) *API {
	api := &API{
		logger:            logger,
		filesystem:        filesystem,
		pathStore:         pathStore,
		envInfo:           usershell.SystemEnvInfo{},
		bundleFilesLimits: defaultBundleFilesLimits,
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
	r.Post("/bundle-files", api.HandleBundleFiles)

	return r
}
