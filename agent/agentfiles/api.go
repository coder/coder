package agentfiles

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/filefinder"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API exposes file-related operations performed through the agent.
type API struct {
	logger            slog.Logger
	filesystem        afero.Fs
	pathStore         *agentgit.PathStore
	envInfo           usershell.EnvInfoer
	workingDir        func() string
	bundleFilesLimits workspacesdk.BundleFilesLimits
	execer            agentexec.Execer

	ignorerOnce sync.Once
	ignorer     *filefinder.Ignorer

	finderMu      sync.Mutex
	finder        *filefinder.Engine
	finderCtx     context.Context
	finderCancel  context.CancelFunc
	finderRoots   []string
	finderPending map[string]*pendingRoot
	finderPins    map[string]int
	closed        bool
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

// WithExecer sets how git is run to classify gitignored paths. The
// default is agentexec.DefaultExecer.
func WithExecer(execer agentexec.Execer) Option {
	return func(api *API) {
		api.execer = execer
	}
}

// WithWorkingDir sets the agent's configured working directory, used
// as the default directory for path forms. It may return "".
func WithWorkingDir(workingDir func() string) Option {
	return func(api *API) {
		api.workingDir = workingDir
	}
}

func NewAPI(logger slog.Logger, filesystem afero.Fs, pathStore *agentgit.PathStore, opts ...Option) *API {
	api := &API{
		logger:            logger,
		filesystem:        filesystem,
		pathStore:         pathStore,
		envInfo:           usershell.SystemEnvInfo{},
		execer:            agentexec.DefaultExecer,
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
	r.Post("/upload-chat-file", api.HandleUploadChatFile)
	r.Post("/edit-files", api.HandleEditFiles)
	r.Post("/bundle-files", api.HandleBundleFiles)
	r.Get("/find-files", api.HandleFindFiles)

	return r
}

// Close releases the file finder index, if one was created, and
// cancels in-progress root walks.
func (api *API) Close() error {
	api.finderMu.Lock()
	api.closed = true
	engine, cancel := api.finder, api.finderCancel
	api.finder = nil
	api.finderRoots = nil
	api.finderMu.Unlock()
	if engine == nil {
		return nil
	}
	cancel()
	return engine.Close()
}

// getIgnorer returns the gitignore rules shared by list-directory and
// find-files, created on first use. Global excludes are read from the
// agent user's home directory; they are skipped if it is unknown.
func (api *API) getIgnorer() *filefinder.Ignorer {
	api.ignorerOnce.Do(func() {
		home, _ := api.envInfo.HomeDir()
		api.ignorer = filefinder.NewIgnorer(api.filesystem, home, filefinder.WithGit(api.execer, "git"))
	})
	return api.ignorer
}

// defaultDir returns the agent's configured working directory when it
// exists, otherwise the user's home directory.
func (api *API) defaultDir() (string, error) {
	var configured string
	if api.workingDir != nil {
		configured = api.workingDir()
	}
	dir, err := usershell.ResolveWorkingDirectory(api.filesystem, api.envInfo, configured)
	if err != nil {
		return "", xerrors.Errorf("resolve default directory: %w", err)
	}
	return filepath.Clean(dir), nil
}

// resolvePathForm converts a user-facing path into an absolute, cleaned
// path. "" is the default directory, "~" and "~/x" are relative to the
// home directory, absolute paths are cleaned, and other paths are
// relative to the default directory. Environment variables are not
// expanded.
func (api *API) resolvePathForm(path string) (string, error) {
	switch {
	case path == "":
		return api.defaultDir()
	case path == "~" || strings.HasPrefix(path, "~/"):
		home, err := api.envInfo.HomeDir()
		if err != nil {
			return "", xerrors.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
	case filepath.IsAbs(path):
		return filepath.Clean(path), nil
	default:
		dir, err := api.defaultDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, path), nil
	}
}
