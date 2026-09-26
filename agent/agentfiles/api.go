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
	"github.com/coder/quartz"
)

// API exposes file-related operations performed through the agent.
type API struct {
	logger            slog.Logger
	filesystem        afero.Fs
	pathStore         *agentgit.PathStore
	envInfo           usershell.EnvInfoer
	bundleFilesLimits workspacesdk.BundleFilesLimits
	toolCallChats     *agenttoolcall.Chats
	// toolCalls decides whether an edit or write with tool call headers
	// acts, and records its response for repeated requests and cancels.
	// Keys are unique per tool call, so the edit and write routes never
	// read each other's records in practice.
	toolCalls *agenttoolcall.Records[fileResult]
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

// WithToolCallChats sets the per-chat tool call state the file records
// share with the agent's other tool call records. Without it the API
// keeps its own, with the agent start measured by the real clock.
func WithToolCallChats(chats *agenttoolcall.Chats) Option {
	return func(api *API) {
		api.toolCallChats = chats
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
	if api.toolCallChats == nil {
		api.toolCallChats = agenttoolcall.NewChats(quartz.NewReal())
	}
	api.toolCalls = agenttoolcall.NewRecords[fileResult](api.toolCallChats)
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
	r.Post("/write-file/{id}/cancel", api.handleCancelToolCall)
	r.Post("/upload-chat-file", api.HandleUploadChatFile)
	r.Post("/edit-files", api.HandleEditFiles)
	r.Post("/edit-files/{id}/cancel", api.handleCancelToolCall)
	r.Post("/bundle-files", api.HandleBundleFiles)

	return r
}
