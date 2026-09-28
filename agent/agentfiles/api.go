package agentfiles

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agenttoolcall"
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
	// toolCallStore runs the edit and write routes at most once per tool
	// call. Nil serves them without tool call handling.
	toolCallStore *agenttoolcall.Store
}

// Option configures the API.
type Option func(*API)

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

// WithToolCallStore runs the edit and write routes through
// store.Middleware, so a request with tool call headers changes files at
// most once and a repeated request gets the recorded response.
func WithToolCallStore(store *agenttoolcall.Store) Option {
	return func(api *API) {
		api.toolCallStore = store
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
	// changes serves the routes that change files.
	var changes chi.Router = r
	if api.toolCallStore != nil {
		changes = r.With(api.toolCallStore.Middleware)
	}

	r.Post("/list-directory", api.HandleLS)
	r.Get("/resolve-path", api.HandleResolvePath)
	r.Get("/read-file", api.HandleReadFile)
	r.Get("/read-file-lines", api.HandleReadFileLines)
	changes.Post("/write-file", api.HandleWriteFile)
	r.Post("/upload-chat-file", api.HandleUploadChatFile)
	changes.Post("/edit-files", api.HandleEditFiles)
	r.Post("/bundle-files", api.HandleBundleFiles)

	return r
}
