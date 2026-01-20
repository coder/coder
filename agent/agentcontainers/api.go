package agentcontainers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentcontainers/ignore"
	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

const (
	defaultUpdateInterval   = 10 * time.Second
	defaultOperationTimeout = 15 * time.Second

	// Destination path inside the container, we store it in a fixed location
	// under /.coder-agent/coder to avoid conflicts and avoid being shadowed
	// by tmpfs or other mounts. This assumes the container root filesystem is
	// read-write, which seems sensible for devcontainers.
	coderPathInsideContainer = "/.coder-agent/coder"

	maxAgentNameLength     = 64
	maxAttemptsToNameAgent = 5
)

// API is responsible for container-related operations in the agent.
// It provides methods to list and manage containers.
type API struct {
	ctx                         context.Context
	cancel                      context.CancelFunc
	watcherDone                 chan struct{}
	updaterDone                 chan struct{}
	discoverDone                chan struct{}
	updateTrigger               chan chan error // Channel to trigger manual refresh.
	updateInterval              time.Duration   // Interval for periodic container updates.
	logger                      slog.Logger
	watcher                     watcher.Watcher
	fs                          afero.Fs
	execer                      agentexec.Execer
	commandEnv                  CommandEnv
	ccli                        ContainerCLI
	containerLabelIncludeFilter map[string]string // Labels to filter containers by.
	dccli                       DevcontainerCLI
	clock                       quartz.Clock
	scriptLogger                func(logSourceID uuid.UUID) ScriptLogger
	subAgentClient              atomic.Pointer[SubAgentClient]
	subAgentURL                 string
	subAgentEnv                 []string

	projectDiscovery   bool // If we should perform project discovery or not.
	discoveryAutostart bool // If we should autostart discovered projects.

	ownerName      string
	workspaceName  string
	parentAgent    string
	agentDirectory string

	mu                       sync.RWMutex  // Protects the following fields.
	initDone                 bool          // Whether Init has been called.
	initialUpdateDone        chan struct{} // Closed after first updateContainers call in updaterLoop.
	updateChans              []chan struct{}
	closed                   bool
	containers               codersdk.WorkspaceAgentListContainersResponse  // Output from the last list operation.
	containersErr            error                                          // Error from the last list operation.
	devcontainerNames        map[string]bool                                // By devcontainer name.
	knownDevcontainers       map[string]codersdk.WorkspaceAgentDevcontainer // By workspace folder.
	devcontainerLogSourceIDs map[string]uuid.UUID                           // By workspace folder.
	configFileModifiedTimes  map[string]time.Time                           // By config file path.
	recreateSuccessTimes     map[string]time.Time                           // By workspace folder.
	recreateErrorTimes       map[string]time.Time                           // By workspace folder.
	injectedSubAgentProcs    map[string]subAgentProcess                     // By workspace folder.
	usingWorkspaceFolderName map[string]bool                                // By workspace folder.
	ignoredDevcontainers     map[string]bool                                // By workspace folder. Tracks three states (true, false and not checked).
	asyncWg                  sync.WaitGroup
}

type subAgentProcess struct {
	agent       SubAgent
	containerID string
	ctx         context.Context
	stop        context.CancelFunc
}

// Option is a functional option for API.
type Option func(*API)

// WithClock sets the quartz.Clock implementation to use.
// This is primarily used for testing to control time.
func WithClock(clock quartz.Clock) Option {
	return func(api *API) {
		api.clock = clock
	}
}

// WithExecer sets the agentexec.Execer implementation to use.
func WithExecer(execer agentexec.Execer) Option {
	return func(api *API) {
		api.execer = execer
	}
}

// WithCommandEnv sets the CommandEnv implementation to use.
func WithCommandEnv(ce CommandEnv) Option {
	return func(api *API) {
		api.commandEnv = func(ei usershell.EnvInfoer, preEnv []string) (string, string, []string, error) {
			shell, dir, env, err := ce(ei, preEnv)
			if err != nil {
				return shell, dir, env, err
			}
			env = slices.DeleteFunc(env, func(s string) bool {
				// Ensure we filter out environment variables that come
				// from the parent agent and are incorrect or not
				// relevant for the devcontainer.
				return strings.HasPrefix(s, "CODER_WORKSPACE_AGENT_NAME=") ||
					strings.HasPrefix(s, "CODER_WORKSPACE_AGENT_URL=") ||
					strings.HasPrefix(s, "CODER_AGENT_TOKEN=") ||
					strings.HasPrefix(s, "CODER_AGENT_AUTH=") ||
					strings.HasPrefix(s, "CODER_AGENT_DEVCONTAINERS_ENABLE=") ||
					strings.HasPrefix(s, "CODER_AGENT_DEVCONTAINERS_PROJECT_DISCOVERY_ENABLE=") ||
					strings.HasPrefix(s, "CODER_AGENT_DEVCONTAINERS_DISCOVERY_AUTOSTART_ENABLE=")
			})
			return shell, dir, env, nil
		}
	}
}

// WithContainerCLI sets the agentcontainers.ContainerCLI implementation
// to use. The default implementation uses the Docker CLI.
func WithContainerCLI(ccli ContainerCLI) Option {
	return func(api *API) {
		api.ccli = ccli
	}
}

// WithContainerLabelIncludeFilter sets a label filter for containers.
// This option can be given multiple times to filter by multiple labels.
// The behavior is such that only containers matching all of the provided
// labels will be included.
func WithContainerLabelIncludeFilter(label, value string) Option {
	return func(api *API) {
		api.containerLabelIncludeFilter[label] = value
	}
}

// WithDevcontainerCLI sets the DevcontainerCLI implementation to use.
// This can be used in tests to modify @devcontainer/cli behavior.
func WithDevcontainerCLI(dccli DevcontainerCLI) Option {
	return func(api *API) {
		api.dccli = dccli
	}
}

// WithSubAgentClient sets the SubAgentClient implementation to use.
// This is used to list, create, and delete devcontainer agents.
func WithSubAgentClient(client SubAgentClient) Option {
	return func(api *API) {
		api.subAgentClient.Store(&client)
	}
}

// WithSubAgentURL sets the agent URL for the sub-agent for
// communicating with the control plane.
func WithSubAgentURL(url string) Option {
	return func(api *API) {
		api.subAgentURL = url
	}
}

// WithSubAgentEnv sets the environment variables for the sub-agent.
func WithSubAgentEnv(env ...string) Option {
	return func(api *API) {
		api.subAgentEnv = env
	}
}

// WithManifestInfo sets the owner name, and workspace name
// for the sub-agent.
func WithManifestInfo(owner, workspace, parentAgent, agentDirectory string) Option {
	return func(api *API) {
		api.ownerName = owner
		api.workspaceName = workspace
		api.parentAgent = parentAgent
		api.agentDirectory = agentDirectory
	}
}

// WithDevcontainers sets the known devcontainers for the API. This
// allows the API to be aware of devcontainers defined in the workspace
// agent manifest.
func WithDevcontainers(devcontainers []codersdk.WorkspaceAgentDevcontainer, scripts []codersdk.WorkspaceAgentScript) Option {
	return func(api *API) {
		if len(devcontainers) == 0 {
			return
		}
		api.knownDevcontainers = make(map[string]codersdk.WorkspaceAgentDevcontainer, len(devcontainers))
		api.devcontainerNames = make(map[string]bool, len(devcontainers))
		api.devcontainerLogSourceIDs = make(map[string]uuid.UUID)
		for _, dc := range devcontainers {
			if dc.Status == "" {
				dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStarting
			}
			logger := api.logger.With(
				slog.F("devcontainer_id", dc.ID),
				slog.F("devcontainer_name", dc.Name),
				slog.F("workspace_folder", dc.WorkspaceFolder),
				slog.F("config_path", dc.ConfigPath),
			)

			// Devcontainers have a name originating from Terraform, but
			// we need to ensure that the name is unique. We will use
			// the workspace folder name to generate a unique agent name,
			// and if that fails, we will fall back to the devcontainers
			// original name.
			name, usingWorkspaceFolder := api.makeAgentName(dc.WorkspaceFolder, dc.Name)
			if name != dc.Name {
				logger = logger.With(slog.F("devcontainer_name", name))
				logger.Debug(api.ctx, "updating devcontainer name", slog.F("devcontainer_old_name", dc.Name))
				dc.Name = name
				api.usingWorkspaceFolderName[dc.WorkspaceFolder] = usingWorkspaceFolder
			}

			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			api.devcontainerNames[dc.Name] = true
			for _, script := range scripts {
				// The devcontainer scripts match the devcontainer ID for
				// identification.
				if script.ID == dc.ID {
					api.devcontainerLogSourceIDs[dc.WorkspaceFolder] = script.LogSourceID
					break
				}
			}
			if api.devcontainerLogSourceIDs[dc.WorkspaceFolder] == uuid.Nil {
				logger.Error(api.ctx, "devcontainer log source ID not found for devcontainer")
			}
		}
	}
}

// WithWatcher sets the file watcher implementation to use. By default a
// noop watcher is used. This can be used in tests to modify the watcher
// behavior or to use an actual file watcher (e.g. fsnotify).
func WithWatcher(w watcher.Watcher) Option {
	return func(api *API) {
		api.watcher = w
	}
}

// WithFileSystem sets the file system used for discovering projects.
func WithFileSystem(fileSystem afero.Fs) Option {
	return func(api *API) {
		api.fs = fileSystem
	}
}

// WithProjectDiscovery sets if the API should attempt to discover
// projects on the filesystem.
func WithProjectDiscovery(projectDiscovery bool) Option {
	return func(api *API) {
		api.projectDiscovery = projectDiscovery
	}
}

// WithDiscoveryAutostart sets if the API should attempt to autostart
// projects that have been discovered
func WithDiscoveryAutostart(discoveryAutostart bool) Option {
	return func(api *API) {
		api.discoveryAutostart = discoveryAutostart
	}
}

// ScriptLogger is an interface for sending devcontainer logs to the
// controlplane.
type ScriptLogger interface {
	Send(ctx context.Context, log ...agentsdk.Log) error
	Flush(ctx context.Context) error
}

// noopScriptLogger is a no-op implementation of the ScriptLogger
// interface.
type noopScriptLogger struct{}

func (noopScriptLogger) Send(context.Context, ...agentsdk.Log) error { return nil }
func (noopScriptLogger) Flush(context.Context) error                 { return nil }

// WithScriptLogger sets the script logger provider for devcontainer operations.
func WithScriptLogger(scriptLogger func(logSourceID uuid.UUID) ScriptLogger) Option {
	return func(api *API) {
		api.scriptLogger = scriptLogger
	}
}

// NewAPI returns a new API with the given options applied.
func NewAPI(logger slog.Logger, options ...Option) *API {
	ctx, cancel := context.WithCancel(context.Background())
	api := &API{
		ctx:                         ctx,
		cancel:                      cancel,
		initialUpdateDone:           make(chan struct{}),
		updateTrigger:               make(chan chan error),
		updateInterval:              defaultUpdateInterval,
		logger:                      logger,
		clock:                       quartz.NewReal(),
		execer:                      agentexec.DefaultExecer,
		containerLabelIncludeFilter: make(map[string]string),
		devcontainerNames:           make(map[string]bool),
		knownDevcontainers:          make(map[string]codersdk.WorkspaceAgentDevcontainer),
		configFileModifiedTimes:     make(map[string]time.Time),
		ignoredDevcontainers:        make(map[string]bool),
		recreateSuccessTimes:        make(map[string]time.Time),
		recreateErrorTimes:          make(map[string]time.Time),
		scriptLogger:                func(uuid.UUID) ScriptLogger { return noopScriptLogger{} },
		injectedSubAgentProcs:       make(map[string]subAgentProcess),
		usingWorkspaceFolderName:    make(map[string]bool),
	}
	// The ctx and logger must be set before applying options to avoid
	// nil pointer dereference.
	for _, opt := range options {
		opt(api)
	}
	if api.commandEnv != nil {
		api.execer = newCommandEnvExecer(
			api.logger,
			api.commandEnv,
			api.execer,
		)
	}
	if api.ccli == nil {
		api.ccli = NewDockerCLI(api.execer)
	}
	if api.dccli == nil {
		api.dccli = NewDevcontainerCLI(logger.Named("devcontainer-cli"), api.execer)
	}
	if api.watcher == nil {
		var err error
		api.watcher, err = watcher.NewFSNotify()
		if err != nil {
			logger.Error(ctx, "create file watcher service failed", slog.Error(err))
			api.watcher = watcher.NewNoop()
		}
	}
	if api.fs == nil {
		api.fs = afero.NewOsFs()
	}
	if api.subAgentClient.Load() == nil {
		var c SubAgentClient = noopSubAgentClient{}
		api.subAgentClient.Store(&c)
	}

	return api
}

// Init applies a final set of options to the API and marks
// initialization as done. This method can only be called once.
func (api *API) Init(opts ...Option) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.closed || api.initDone {
		return
	}
	api.initDone = true

	for _, opt := range opts {
		opt(api)
	}
}

// Start starts the API by initializing the watcher and updater loops.
// This method calls Init, if it is desired to apply options after
// the API has been created, it should be done by calling Init before
// Start. This method must only be called once.
func (api *API) Start() {
	api.Init()

	api.mu.Lock()
	defer api.mu.Unlock()
	if api.closed {
		return
	}

	if api.projectDiscovery && api.agentDirectory != "" {
		api.discoverDone = make(chan struct{})

		go api.discover()
	}

	api.watcherDone = make(chan struct{})
	api.updaterDone = make(chan struct{})

	go api.watcherLoop()
	go api.updaterLoop()
}

func (api *API) discover() {
	defer close(api.discoverDone)
	defer api.logger.Debug(api.ctx, "project discovery finished")
	api.logger.Debug(api.ctx, "project discovery started")

	if err := api.discoverDevcontainerProjects(); err != nil {
		api.logger.Error(api.ctx, "discovering dev container projects", slog.Error(err))
	}

	if err := api.RefreshContainers(api.ctx); err != nil {
		api.logger.Error(api.ctx, "refreshing containers after discovery", slog.Error(err))
	}
}

func (api *API) discoverDevcontainerProjects() error {
	isGitProject, err := afero.DirExists(api.fs, filepath.Join(api.agentDirectory, ".git"))
	if err != nil {
		return xerrors.Errorf(".git dir exists: %w", err)
	}

	// If the agent directory is a git project, we'll search
	// the project for any `.devcontainer/devcontainer.json`
	// files.
	if isGitProject {
		return api.discoverDevcontainersInProject(api.agentDirectory)
	}

	// The agent directory is _not_ a git project, so we'll
	// search the top level of the agent directory for any
	// git projects, and search those.
	entries, err := afero.ReadDir(api.fs, api.agentDirectory)
	if err != nil {
		return xerrors.Errorf("read agent directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		isGitProject, err = afero.DirExists(api.fs, filepath.Join(api.agentDirectory, entry.Name(), ".git"))
		if err != nil {
			return xerrors.Errorf(".git dir exists: %w", err)
		}

		// If this directory is a git project, we'll search
		// it for any `.devcontainer/devcontainer.json` files.
		if isGitProject {
			if err := api.discoverDevcontainersInProject(filepath.Join(api.agentDirectory, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func (api *API) discoverDevcontainersInProject(projectPath string) error {
	logger := api.logger.
		Named("project-discovery").
		With(slog.F("project_path", projectPath))

	globalPatterns, err := ignore.LoadGlobalPatterns(api.fs)
	if err != nil {
		return xerrors.Errorf("read global git ignore patterns: %w", err)
	}

	patterns, err := ignore.ReadPatterns(api.ctx, logger, api.fs, projectPath)
	if err != nil {
		return xerrors.Errorf("read git ignore patterns: %w", err)
	}

	matcher := gitignore.NewMatcher(append(globalPatterns, patterns...))

	devcontainerConfigPaths := []string{
		"/.devcontainer/devcontainer.json",
		"/.devcontainer.json",
	}

	return afero.Walk(api.fs, projectPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			logger.Error(api.ctx, "encountered error while walking for dev container projects",
				slog.F("path", path),
				slog.Error(err))
			return nil
		}

		pathParts := ignore.FilePathToParts(path)

		// We know that a directory entry cannot be a `devcontainer.json` file, so we
		// always skip processing directories. If the directory happens to be ignored
		// by git then we'll make sure to ignore all of the children of that directory.
		if info.IsDir() {
			if matcher.Match(pathParts, true) {
				return fs.SkipDir
			}

			return nil
		}

		if matcher.Match(pathParts, false) {
			return nil
		}

		for _, relativeConfigPath := range devcontainerConfigPaths {
			if !strings.HasSuffix(path, relativeConfigPath) {
				continue
			}

			workspaceFolder := strings.TrimSuffix(path, relativeConfigPath)

			logger := logger.With(slog.F("workspace_folder", workspaceFolder))
			logger.Debug(api.ctx, "discovered dev container project")

			api.mu.Lock()
			if _, found := api.knownDevcontainers[workspaceFolder]; !found {
				logger.Debug(api.ctx, "adding dev container project")

				dc := codersdk.WorkspaceAgentDevcontainer{
					ID:              uuid.New(),
					Name:            "", // Updated later based on container state.
					WorkspaceFolder: workspaceFolder,
					ConfigPath:      path,
					Status:          codersdk.WorkspaceAgentDevcontainerStatusStopped,
					Dirty:           false, // Updated later based on config file changes.
					Container:       nil,
				}

				if api.discoveryAutostart {
					config, err := api.dccli.ReadConfig(api.ctx, workspaceFolder, path, []string{})
					if err != nil {
						logger.Error(api.ctx, "read project configuration", slog.Error(err))
					} else if config.Configuration.Customizations.Coder.AutoStart {
						dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStarting
					}
				}

				api.knownDevcontainers[workspaceFolder] = dc
				api.broadcastUpdatesLocked()

				if dc.Status == codersdk.WorkspaceAgentDevcontainerStatusStarting {
					api.asyncWg.Add(1)
					go func() {
						defer api.asyncWg.Done()

						_ = api.CreateDevcontainer(dc.WorkspaceFolder, dc.ConfigPath)
					}()
				}
			}
			api.mu.Unlock()
		}

		return nil
	})
}

func (api *API) watcherLoop() {
	defer close(api.watcherDone)
	defer api.logger.Debug(api.ctx, "watcher loop stopped")
	api.logger.Debug(api.ctx, "watcher loop started")

	for {
		event, err := api.watcher.Next(api.ctx)
		if err != nil {
			if errors.Is(err, watcher.ErrClosed) {
				api.logger.Debug(api.ctx, "watcher closed")
				return
			}
			if api.ctx.Err() != nil {
				api.logger.Debug(api.ctx, "api context canceled")
				return
			}
			api.logger.Error(api.ctx, "watcher error waiting for next event", slog.Error(err))
			continue
		}
		if event == nil {
			continue
		}

		now := api.clock.Now("agentcontainers", "watcherLoop")
		switch {
		case event.Has(fsnotify.Create | fsnotify.Write):
			api.logger.Debug(api.ctx, "devcontainer config file changed", slog.F("file", event.Name))
			api.markDevcontainerDirty(event.Name, now)
		case event.Has(fsnotify.Remove):
			api.logger.Debug(api.ctx, "devcontainer config file removed", slog.F("file", event.Name))
			api.markDevcontainerDirty(event.Name, now)
		case event.Has(fsnotify.Rename):
			api.logger.Debug(api.ctx, "devcontainer config file renamed", slog.F("file", event.Name))
			api.markDevcontainerDirty(event.Name, now)
		default:
			api.logger.Debug(api.ctx, "devcontainer config file event ignored", slog.F("file", event.Name), slog.F("event", event))
		}
	}
}

// updaterLoop is responsible for periodically updating the container
// list and handling manual refresh requests.
func (api *API) updaterLoop() {
	defer close(api.updaterDone)
	defer api.logger.Debug(api.ctx, "updater loop stopped")
	api.logger.Debug(api.ctx, "updater loop started")

	// Make sure we clean up any subagents not tracked by this process
	// before starting the update loop and creating new ones.
	api.logger.Debug(api.ctx, "cleaning up subagents")
	if err := api.cleanupSubAgents(api.ctx); err != nil {
		api.logger.Error(api.ctx, "cleanup subagents failed", slog.Error(err))
	} else {
		api.logger.Debug(api.ctx, "cleanup subagents complete")
	}

	// Perform an initial update to populate the container list, this
	// gives us a guarantee that the API has loaded the initial state
	// before returning any responses. This is useful for both tests
	// and anyone looking to interact with the API.
	api.logger.Debug(api.ctx, "performing initial containers update")
	if err := api.updateContainers(api.ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			api.logger.Warn(api.ctx, "initial containers update canceled", slog.Error(err))
		} else {
			api.logger.Error(api.ctx, "initial containers update failed", slog.Error(err))
		}
	} else {
		api.logger.Debug(api.ctx, "initial containers update complete")
	}
	close(api.initialUpdateDone)

	// We utilize a TickerFunc here instead of a regular Ticker so that
	// we can guarantee execution of the updateContainers method after
	// advancing the clock.
	var prevErr error
	ticker := api.clock.TickerFunc(api.ctx, api.updateInterval, func() error {
		done := make(chan error, 1)
		var sent bool
		defer func() {
			if !sent {
				close(done)
			}
		}()
		select {
		case <-api.ctx.Done():
			return api.ctx.Err()
		case api.updateTrigger <- done:
			sent = true
			err := <-done
			if err != nil {
				if errors.Is(err, context.Canceled) {
					api.logger.Warn(api.ctx, "updater loop ticker canceled", slog.Error(err))
					return nil
				}
				// Avoid excessive logging of the same error.
				if prevErr == nil || prevErr.Error() != err.Error() {
					api.logger.Error(api.ctx, "updater loop ticker failed", slog.Error(err))
				}
				prevErr = err
			} else {
				prevErr = nil
			}
		}

		return nil // Always nil to keep the ticker going.
	}, "agentcontainers", "updaterLoop")
	defer func() {
		if err := ticker.Wait("agentcontainers", "updaterLoop"); err != nil && !errors.Is(err, context.Canceled) {
			api.logger.Error(api.ctx, "updater loop ticker failed", slog.Error(err))
		}
	}()

	for {
		select {
		case <-api.ctx.Done():
			return
		case done := <-api.updateTrigger:
			// Note that although we pass api.ctx here, updateContainers
			// has an internal timeout to prevent long blocking calls.
			done <- api.updateContainers(api.ctx)
			close(done)
		}
	}
}

// UpdateSubAgentClient updates the `SubAgentClient` for the API.
func (api *API) UpdateSubAgentClient(client SubAgentClient) {
	api.subAgentClient.Store(&client)
}

// Routes returns the HTTP handler for container-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()

	ensureInitialUpdateDoneMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			select {
			case <-api.ctx.Done():
				httpapi.Write(r.Context(), rw, http.StatusServiceUnavailable, codersdk.Response{
					Message: "API closed",
					Detail:  "The API is closed and cannot process requests.",
				})
				return
			case <-r.Context().Done():
				return
			case <-api.initialUpdateDone:
				// Initial update is done, we can start processing requests.
			}
			next.ServeHTTP(rw, r)
		})
	}

	// For now, all endpoints require the initial update to be done.
	// If we want to allow some endpoints to be available before
	// the initial update, we can enable this per-route.
	r.Use(ensureInitialUpdateDoneMW)

	r.Get("/", api.handleList)
	r.Get("/watch", api.watchContainers)
	// TODO(mafredri): Simplify this route as the previous /devcontainers
	// /-route was dropped. We can drop the /devcontainers prefix here too.
	r.Route("/devcontainers/{devcontainer}", func(r chi.Router) {
		r.Post("/recreate", api.handleDevcontainerRecreate)
		r.Delete("/", api.handleDevcontainerDelete)
	})

	return r
}

// broadcastUpdatesLocked sends the current state to any listening clients.
// This method assumes that api.mu is held.
func (api *API) broadcastUpdatesLocked() {
	// Broadcast state changes to WebSocket listeners.
	for _, ch := range api.updateChans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (api *API) watchContainers(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// We want `NoContextTakeover` compression to balance improving
		// bandwidth cost/latency with minimal memory usage overhead.
		CompressionMode: websocket.CompressionNoContextTakeover,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upgrade connection to websocket.",
			Detail:  err.Error(),
		})
		return
	}

	// Here we close the websocket for reading, so that the websocket library will handle pings and
	// close frames.
	_ = conn.CloseRead(context.Background())

	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close()

	go httpapi.Heartbeat(ctx, conn)

	updateCh := make(chan struct{}, 1)

	api.mu.Lock()
	api.updateChans = append(api.updateChans, updateCh)
	api.mu.Unlock()

	defer func() {
		api.mu.Lock()
		api.updateChans = slices.DeleteFunc(api.updateChans, func(ch chan struct{}) bool {
			return ch == updateCh
		})
		close(updateCh)
		api.mu.Unlock()
	}()

	encoder := json.NewEncoder(wsNetConn)

	ct, err := api.getContainers()
	if err != nil {
		api.logger.Error(ctx, "unable to get containers", slog.Error(err))
		return
	}

	if err := encoder.Encode(ct); err != nil {
		api.logger.Error(ctx, "encode container list", slog.Error(err))
		return
	}

	for {
		select {
		case <-api.ctx.Done():
			return

		case <-ctx.Done():
			return

		case <-updateCh:
			ct, err := api.getContainers()
			if err != nil {
				api.logger.Error(ctx, "unable to get containers", slog.Error(err))
				continue
			}

			if err := encoder.Encode(ct); err != nil {
				api.logger.Error(ctx, "encode container list", slog.Error(err))
				return
			}
		}
	}
}

// handleList handles the HTTP request to list containers.
func (api *API) handleList(rw http.ResponseWriter, r *http.Request) {
	ct, err := api.getContainers()
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not get containers",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, ct)
}

// updateContainers fetches the latest container list, processes it, and
// updates the cache. It performs locking for updating shared API state.
func (api *API) updateContainers(ctx context.Context) error {
	listCtx, listCancel := context.WithTimeout(ctx, defaultOperationTimeout)
	defer listCancel()

	updated, err := api.ccli.List(listCtx)
	if err != nil {
		// If the context was canceled, we hold off on clearing the
		// containers cache. This is to avoid clearing the cache if
		// the update was canceled due to a timeout. Hopefully this
		// will clear up on the next update.
		if !errors.Is(err, context.Canceled) {
			api.mu.Lock()
			api.containersErr = err
			api.mu.Unlock()
		}

		return xerrors.Errorf("list containers failed: %w", err)
	}
	// Clone to avoid test flakes due to data manipulation.
	updated.Containers = slices.Clone(updated.Containers)

	api.mu.Lock()
	defer api.mu.Unlock()

	var previouslyKnownDevcontainers map[string]codersdk.WorkspaceAgentDevcontainer
	if len(api.updateChans) > 0 {
		previouslyKnownDevcontainers = maps.Clone(api.knownDevcontainers)
	}

	api.processUpdatedContainersLocked(ctx, updated)

	if len(api.updateChans) > 0 {
		statesAreEqual := maps.EqualFunc(
			previouslyKnownDevcontainers,
			api.knownDevcontainers,
			func(dc1, dc2 codersdk.WorkspaceAgentDevcontainer) bool {
				return dc1.Equals(dc2)
			})

		if !statesAreEqual {
			api.broadcastUpdatesLocked()
		}
	}

	api.logger.Debug(ctx, "containers updated successfully", slog.F("container_count", len(api.containers.Containers)), slog.F("warning_count", len(api.containers.Warnings)), slog.F("devcontainer_count", len(api.knownDevcontainers)))

	return nil
}

// processUpdatedContainersLocked updates the devcontainer state based
// on the latest list of containers. This method assumes that api.mu is
// held.
func (api *API) processUpdatedContainersLocked(ctx context.Context, updated codersdk.WorkspaceAgentListContainersResponse) {
	dcFields := func(dc codersdk.WorkspaceAgentDevcontainer) []slog.Field {
		f := []slog.Field{
			slog.F("devcontainer_id", dc.ID),
			slog.F("devcontainer_name", dc.Name),
			slog.F("workspace_folder", dc.WorkspaceFolder),
			slog.F("config_path", dc.ConfigPath),
		}
		if dc.Container != nil {
			f = append(f, slog.F("container_id", dc.Container.ID))
			f = append(f, slog.F("container_name", dc.Container.FriendlyName))
		}
		return f
	}

	// Reset the container links in known devcontainers to detect if
	// they still exist.
	for _, dc := range api.knownDevcontainers {
		dc.Container = nil
		api.knownDevcontainers[dc.WorkspaceFolder] = dc
	}

	// Check if the container is running and update the known devcontainers.
	for i := range updated.Containers {
		container := &updated.Containers[i] // Grab a reference to the container to allow mutating it.

		workspaceFolder := container.Labels[DevcontainerLocalFolderLabel]
		configFile := container.Labels[DevcontainerConfigFileLabel]

		if workspaceFolder == "" {
			continue
		}

		logger := api.logger.With(
			slog.F("container_id", updated.Containers[i].ID),
			slog.F("container_name", updated.Containers[i].FriendlyName),
			slog.F("workspace_folder", workspaceFolder),
			slog.F("config_file", configFile),
		)

		// If we haven't set any include filters, we should explicitly ignore test devcontainers.
		if len(api.containerLabelIncludeFilter) == 0 && container.Labels[DevcontainerIsTestRunLabel] == "true" {
			continue
		}

		// Filter out devcontainer tests, unless explicitly set in include filters.
		if len(api.containerLabelIncludeFilter) > 0 {
			includeContainer := true
			for label, value := range api.containerLabelIncludeFilter {
				v, found := container.Labels[label]

				includeContainer = includeContainer && (found && v == value)
			}
			// Verbose debug logging is fine here since typically filters
			// are only used in development or testing environments.
			if !includeContainer {
				logger.Debug(ctx, "container does not match include filter, ignoring devcontainer", slog.F("container_labels", container.Labels), slog.F("include_filter", api.containerLabelIncludeFilter))
				continue
			}
			logger.Debug(ctx, "container matches include filter, processing devcontainer", slog.F("container_labels", container.Labels), slog.F("include_filter", api.containerLabelIncludeFilter))
		}

		if dc, ok := api.knownDevcontainers[workspaceFolder]; ok {
			// If no config path is set, this devcontainer was defined
			// in Terraform without the optional config file. Assume the
			// first container with the workspace folder label is the
			// one we want to use.
			if dc.ConfigPath == "" && configFile != "" {
				dc.ConfigPath = configFile
				if err := api.watcher.Add(configFile); err != nil {
					logger.With(dcFields(dc)...).Error(ctx, "watch devcontainer config file failed", slog.Error(err))
				}
			}

			dc.Container = container
			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			continue
		}

		dc := codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            "", // Updated later based on container state.
			WorkspaceFolder: workspaceFolder,
			ConfigPath:      configFile,
			Status:          "",    // Updated later based on container state.
			Dirty:           false, // Updated later based on config file changes.
			Container:       container,
		}

		if configFile != "" {
			if err := api.watcher.Add(configFile); err != nil {
				logger.With(dcFields(dc)...).Error(ctx, "watch devcontainer config file failed", slog.Error(err))
			}
		}

		api.knownDevcontainers[workspaceFolder] = dc
	}

	// Iterate through all known devcontainers and update their status
	// based on the current state of the containers.
	for _, dc := range api.knownDevcontainers {
		logger := api.logger.With(dcFields(dc)...)

		if dc.Container != nil {
			if !api.devcontainerNames[dc.Name] {
				// If the devcontainer name wasn't set via terraform, we
				// will attempt to create an agent name based on the workspace
				// folder's name. If it is not possible to generate a valid
				// agent name based off of the folder name (i.e. no valid characters),
				// we will instead fall back to using the container's friendly name.
				dc.Name, api.usingWorkspaceFolderName[dc.WorkspaceFolder] = api.makeAgentName(dc.WorkspaceFolder, dc.Container.FriendlyName)
			}
		}

		switch {
		case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusStarting:
			continue // This state is handled by the recreation routine.

		case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusStopping:
			continue // This state is handled by the stopping routine.

		case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusDeleting:
			continue // This state is handled by the delete routine.

		case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusError && (dc.Container == nil || dc.Container.CreatedAt.Before(api.recreateErrorTimes[dc.WorkspaceFolder])):
			continue // The devcontainer needs to be recreated.

		case dc.Container != nil:
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
			if dc.Container.Running {
				dc.Status = codersdk.WorkspaceAgentDevcontainerStatusRunning
			}

			dc.Dirty = false
			if lastModified, hasModTime := api.configFileModifiedTimes[dc.ConfigPath]; hasModTime && dc.Container.CreatedAt.Before(lastModified) {
				dc.Dirty = true
			}

			if dc.Status == codersdk.WorkspaceAgentDevcontainerStatusRunning {
				err := api.maybeInjectSubAgentIntoContainerLocked(ctx, dc)
				if err != nil {
					logger.Error(ctx, "inject subagent into container failed", slog.Error(err))
					dc.Error = err.Error()
				} else {
					// TODO(mafredri): Preserve the error from devcontainer
					// up if it was a lifecycle script error. Currently
					// this results in a brief flicker for the user if
					// injection is fast, as the error is shown then erased.
					dc.Error = ""
				}
			}

		case dc.Container == nil:
			if !api.devcontainerNames[dc.Name] {
				dc.Name = ""
			}
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
			dc.Dirty = false
		}

		delete(api.recreateErrorTimes, dc.WorkspaceFolder)
		api.knownDevcontainers[dc.WorkspaceFolder] = dc
	}

	api.containers = updated
	api.containersErr = nil
}

var consecutiveHyphenRegex = regexp.MustCompile("-+")

// `safeAgentName` returns a safe agent name derived from a folder name,
// falling back to the container’s friendly name if needed. The second
// return value will be `true` if it succeeded and `false` if it had
// to fallback to the friendly name.
func safeAgentName(name string, friendlyName string) (string, bool) {
	// Keep only ASCII letters and digits, replacing everything
	// else with a hyphen.
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			_, _ = sb.WriteRune(r)
		} else {
			_, _ = sb.WriteRune('-')
		}
	}

	// Remove any consecutive hyphens, and then trim any leading
	// and trailing hyphens.
	name = consecutiveHyphenRegex.ReplaceAllString(sb.String(), "-")
	name = strings.Trim(name, "-")

	// Ensure the name of the agent doesn't exceed the maximum agent
	// name length.
	name = name[:min(len(name), maxAgentNameLength)]

	if provisioner.AgentNameRegex.Match([]byte(name)) {
		return name, true
	}

	return safeFriendlyName(friendlyName), false
}

// safeFriendlyName returns a API safe version of the container's
// friendly name.
//
// See provisioner/regexes.go for the regex used to validate
// the friendly name on the API side.
func safeFriendlyName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")

	return name
}

// expandedAgentName creates an agent name by including parent directories
// from the workspace folder path to avoid name collisions. Like `safeAgentName`,
// the second returned value will be true if using the workspace folder name,
// and false if it fell back to the friendly name.
func expandedAgentName(workspaceFolder string, friendlyName string, depth int) (string, bool) {
	var parts []string
	for part := range strings.SplitSeq(filepath.ToSlash(workspaceFolder), "/") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return safeFriendlyName(friendlyName), false
	}

	components := parts[max(0, len(parts)-depth-1):]
	expanded := strings.Join(components, "-")

	return safeAgentName(expanded, friendlyName)
}

// makeAgentName attempts to create an agent name. It will first attempt to create an
// agent name based off of the workspace folder, and will eventually fallback to a
// friendly name. Like `safeAgentName`, the second returned value will be true if the
// agent name utilizes the workspace folder, and false if it falls back to the
// friendly name.
func (api *API) makeAgentName(workspaceFolder string, friendlyName string) (string, bool) {
	for attempt := 0; attempt <= maxAttemptsToNameAgent; attempt++ {
		agentName, usingWorkspaceFolder := expandedAgentName(workspaceFolder, friendlyName, attempt)
		if !usingWorkspaceFolder {
			return agentName, false
		}

		if !api.devcontainerNames[agentName] {
			return agentName, true
		}
	}

	return safeFriendlyName(friendlyName), false
}

// RefreshContainers triggers an immediate update of the container list
// and waits for it to complete.
func (api *API) RefreshContainers(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = xerrors.Errorf("refresh containers failed: %w", err)
		}
	}()

	done := make(chan error, 1)
	var sent bool
	defer func() {
		if !sent {
			close(done)
		}
	}()
	select {
	case <-api.ctx.Done():
		return xerrors.Errorf("API closed: %w", api.ctx.Err())
	case <-ctx.Done():
		return ctx.Err()
	case api.updateTrigger <- done:
		sent = true
		select {
		case <-api.ctx.Done():
			return xerrors.Errorf("API closed: %w", api.ctx.Err())
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			return err
		}
	}
}

func (api *API) getContainers() (codersdk.WorkspaceAgentListContainersResponse, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	if api.containersErr != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, api.containersErr
	}

	var devcontainers []codersdk.WorkspaceAgentDevcontainer
	if len(api.knownDevcontainers) > 0 {
		devcontainers = make([]codersdk.WorkspaceAgentDevcontainer, 0, len(api.knownDevcontainers))
		for _, dc := range api.knownDevcontainers {
			if api.ignoredDevcontainers[dc.WorkspaceFolder] {
				continue
			}

			// Include the agent if it's running (we're iterating over
			// copies, so mutating is fine).
			if proc := api.injectedSubAgentProcs[dc.WorkspaceFolder]; proc.agent.ID != uuid.Nil {
				dc.Agent = &codersdk.WorkspaceAgentDevcontainerAgent{
					ID:        proc.agent.ID,
					Name:      proc.agent.Name,
					Directory: proc.agent.Directory,
				}
			}

			devcontainers = append(devcontainers, dc)
		}
		slices.SortFunc(devcontainers, func(a, b codersdk.WorkspaceAgentDevcontainer) int {
			return strings.Compare(a.WorkspaceFolder, b.WorkspaceFolder)
		})
	}

	return codersdk.WorkspaceAgentListContainersResponse{
		Devcontainers: devcontainers,
		Containers:    slices.Clone(api.containers.Containers),
		Warnings:      slices.Clone(api.containers.Warnings),
	}, nil
}

// devcontainerByIDLocked attempts to find a devcontainer by its ID.
// This method assumes that api.mu is held.
func (api *API) devcontainerByIDLocked(devcontainerID string) (codersdk.WorkspaceAgentDevcontainer, error) {
	for _, knownDC := range api.knownDevcontainers {
		if knownDC.ID.String() == devcontainerID {
			return knownDC, nil
		}
	}

	return codersdk.WorkspaceAgentDevcontainer{}, httperror.NewResponseError(http.StatusNotFound, codersdk.Response{
		Message: "Devcontainer not found.",
		Detail:  fmt.Sprintf("Could not find devcontainer with ID: %q", devcontainerID),
	})
}

func (api *API) handleDevcontainerDelete(w http.ResponseWriter, r *http.Request) {
	var (
		ctx            = r.Context()
		devcontainerID = chi.URLParam(r, "devcontainer")
	)

	if devcontainerID == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing devcontainer ID",
			Detail:  "Devcontainer ID is required to delete a devcontainer.",
		})
		return
	}

	api.mu.Lock()

	dc, err := api.devcontainerByIDLocked(devcontainerID)
	if err != nil {
		api.mu.Unlock()
		httperror.WriteResponseError(ctx, w, err)
		return
	}

	// NOTE(DanielleMaywood):
	// We currently do not support canceling the startup of a dev container.
	if dc.Status.Transitioning() {
		api.mu.Unlock()

		httpapi.Write(ctx, w, http.StatusConflict, codersdk.Response{
			Message: "Unable to delete transitioning devcontainer",
			Detail:  fmt.Sprintf("Devcontainer %q is currently %s and cannot be deleted.", dc.Name, dc.Status),
		})
		return
	}

	var (
		containerID string
		subAgentID  uuid.UUID
	)
	if dc.Container != nil {
		containerID = dc.Container.ID
	}
	if proc, hasSubAgent := api.injectedSubAgentProcs[dc.WorkspaceFolder]; hasSubAgent && proc.agent.ID != uuid.Nil {
		subAgentID = proc.agent.ID
		proc.stop()
	}

	dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopping
	dc.Error = ""
	api.knownDevcontainers[dc.WorkspaceFolder] = dc
	api.broadcastUpdatesLocked()
	api.mu.Unlock()

	// Stop and remove the container if it exists.
	if containerID != "" {
		if err := api.ccli.Stop(ctx, containerID); err != nil {
			api.logger.Error(ctx, "unable to stop container", slog.Error(err))

			api.mu.Lock()
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusError
			dc.Error = err.Error()
			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			api.broadcastUpdatesLocked()
			api.mu.Unlock()

			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "An error occurred stopping the container",
				Detail:  err.Error(),
			})
			return
		}
	}

	api.mu.Lock()
	dc.Status = codersdk.WorkspaceAgentDevcontainerStatusDeleting
	dc.Error = ""
	api.knownDevcontainers[dc.WorkspaceFolder] = dc
	api.broadcastUpdatesLocked()
	api.mu.Unlock()

	if containerID != "" {
		if err := api.ccli.Remove(ctx, containerID); err != nil {
			api.logger.Error(ctx, "unable to remove container", slog.Error(err))

			api.mu.Lock()
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusError
			dc.Error = err.Error()
			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			api.broadcastUpdatesLocked()
			api.mu.Unlock()

			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "An error occurred removing the container",
				Detail:  err.Error(),
			})
			return
		}
	}

	// Delete the subagent if it exists.
	if subAgentID != uuid.Nil {
		client := *api.subAgentClient.Load()
		if err := client.Delete(ctx, subAgentID); err != nil {
			api.logger.Error(ctx, "unable to delete agent", slog.Error(err))

			api.mu.Lock()
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusError
			dc.Error = err.Error()
			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			api.broadcastUpdatesLocked()
			api.mu.Unlock()

			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "An error occurred deleting the agent",
				Detail:  err.Error(),
			})
			return
		}
	}

	api.mu.Lock()
	delete(api.devcontainerNames, dc.Name)
	delete(api.knownDevcontainers, dc.WorkspaceFolder)
	delete(api.devcontainerLogSourceIDs, dc.WorkspaceFolder)
	delete(api.recreateSuccessTimes, dc.WorkspaceFolder)
	delete(api.recreateErrorTimes, dc.WorkspaceFolder)
	delete(api.usingWorkspaceFolderName, dc.WorkspaceFolder)
	delete(api.injectedSubAgentProcs, dc.WorkspaceFolder)
	api.broadcastUpdatesLocked()
	api.mu.Unlock()

	httpapi.Write(ctx, w, http.StatusNoContent, nil)
}

// handleDevcontainerRecreate handles the HTTP request to recreate a
// devcontainer by referencing the container.
func (api *API) handleDevcontainerRecreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	devcontainerID := chi.URLParam(r, "devcontainer")

	if devcontainerID == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing devcontainer ID",
			Detail:  "Devcontainer ID is required to recreate a devcontainer.",
		})
		return
	}

	api.mu.Lock()

	dc, err := api.devcontainerByIDLocked(devcontainerID)
	if err != nil {
		api.mu.Unlock()
		httperror.WriteResponseError(ctx, w, err)
		return
	}
	if dc.SubagentID.Valid {
		api.mu.Unlock()
		httpapi.Write(ctx, w, http.StatusForbidden, codersdk.Response{
			Message: "Cannot rebuild Terraform-defined devcontainer",
			Detail:  fmt.Sprintf("Devcontainer %q has resources defined in Terraform and cannot be rebuilt from the UI. Update the workspace template to modify this devcontainer.", dc.Name),
		})
		return
	}
	if dc.Status.Transitioning() {
		api.mu.Unlock()

		httpapi.Write(ctx, w, http.StatusConflict, codersdk.Response{
			Message: "Unable to recreate transitioning devcontainer",
			Detail:  fmt.Sprintf("Devcontainer %q is currently %s and cannot be restarted.", dc.Name, dc.Status),
		})
		return
	}

	// Update the status so that we don't try to recreate the
	// devcontainer multiple times in parallel.
	dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStarting
	dc.Container = nil
	dc.Error = ""
	api.knownDevcontainers[dc.WorkspaceFolder] = dc
	api.broadcastUpdatesLocked()

	go func() {
		_ = api.CreateDevcontainer(dc.WorkspaceFolder, dc.ConfigPath, WithRemoveExistingContainer())
	}()

	api.mu.Unlock()

	httpapi.Write(ctx, w, http.StatusAccepted, codersdk.Response{
		Message: "Devcontainer recreation initiated",
		Detail:  fmt.Sprintf("Recreation process for devcontainer %q has started.", dc.Name),
	})
}

// createDevcontainer should run in its own goroutine and is responsible for
// recreating a devcontainer based on the provided devcontainer configuration.
// It updates the devcontainer status and logs the process. The configPath is
// passed as a parameter for the odd chance that the container being recreated
// has a different config file than the one stored in the devcontainer state.
// The devcontainer state must be set to starting and the asyncWg must be
// incremented before calling this function.
func (api *API) CreateDevcontainer(workspaceFolder, configPath string, opts ...DevcontainerCLIUpOptions) error {
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return nil
	}

	dc, found := api.knownDevcontainers[workspaceFolder]
	if !found {
		api.mu.Unlock()
		return xerrors.Errorf("devcontainer not found")
	}

	var (
		ctx    = api.ctx
		logger = api.logger.With(
			slog.F("devcontainer_id", dc.ID),
			slog.F("devcontainer_name", dc.Name),
			slog.F("workspace_folder", dc.WorkspaceFolder),
			slog.F("config_path", dc.ConfigPath),
		)
	)

	// Send logs via agent logging facilities.
	logSourceID := api.devcontainerLogSourceIDs[dc.WorkspaceFolder]
	if logSourceID == uuid.Nil {
		api.logger.Debug(api.ctx, "devcontainer log source ID not found, falling back to external log source ID")
		logSourceID = agentsdk.ExternalLogSourceID
	}

	api.asyncWg.Add(1)
	defer api.asyncWg.Done()
	api.mu.Unlock()

	if dc.ConfigPath != configPath {
		logger.Warn(ctx, "devcontainer config path mismatch",
			slog.F("config_path_param", configPath),
		)
	}

	scriptLogger := api.scriptLogger(logSourceID)
	defer func() {
		flushCtx, cancel := context.WithTimeout(api.ctx, 5*time.Second)
		defer cancel()
		if err := scriptLogger.Flush(flushCtx); err != nil {
			logger.Error(flushCtx, "flush devcontainer logs failed during recreation", slog.Error(err))
		}
	}()
	infoW := agentsdk.LogsWriter(ctx, scriptLogger.Send, logSourceID, codersdk.LogLevelInfo)
	defer infoW.Close()
	errW := agentsdk.LogsWriter(ctx, scriptLogger.Send, logSourceID, codersdk.LogLevelError)
	defer errW.Close()

	logger.Debug(ctx, "starting devcontainer recreation")

	upOptions := []DevcontainerCLIUpOptions{WithUpOutput(infoW, errW)}
	upOptions = append(upOptions, opts...)

	containerID, upErr := api.dccli.Up(ctx, dc.WorkspaceFolder, configPath, upOptions...)
	if upErr != nil {
		// No need to log if the API is closing (context canceled), as this
		// is expected behavior when the API is shutting down.
		if !errors.Is(upErr, context.Canceled) {
			logger.Error(ctx, "devcontainer creation failed", slog.Error(upErr))
		}

		// If we don't have a container ID, the error is fatal, so we
		// should mark the devcontainer as errored and return.
		if containerID == "" {
			api.mu.Lock()
			dc = api.knownDevcontainers[dc.WorkspaceFolder]
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusError
			dc.Error = upErr.Error()
			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			api.recreateErrorTimes[dc.WorkspaceFolder] = api.clock.Now("agentcontainers", "recreate", "errorTimes")
			api.broadcastUpdatesLocked()
			api.mu.Unlock()

			return xerrors.Errorf("start devcontainer: %w", upErr)
		}

		// If we have a container ID, it means the container was created
		// but a lifecycle script (e.g. postCreateCommand) failed. In this
		// case, we still want to refresh containers to pick up the new
		// container, inject the agent, and allow the user to debug the
		// issue. We store the error to surface it to the user.
		logger.Warn(ctx, "devcontainer created with errors (e.g. lifecycle script failure), container is available",
			slog.F("container_id", containerID),
		)
	} else {
		logger.Info(ctx, "devcontainer created successfully")
	}

	api.mu.Lock()
	dc = api.knownDevcontainers[dc.WorkspaceFolder]
	// Update the devcontainer status to Running or Stopped based on the
	// current state of the container, changing the status to !starting
	// allows the update routine to update the devcontainer status, but
	// to minimize the time between API consistency, we guess the status
	// based on the container state.
	dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
	if dc.Container != nil && dc.Container.Running {
		dc.Status = codersdk.WorkspaceAgentDevcontainerStatusRunning
	}
	dc.Dirty = false
	if upErr != nil {
		// If there was a lifecycle script error but we have a container ID,
		// the container is running so we should set the status to Running.
		dc.Status = codersdk.WorkspaceAgentDevcontainerStatusRunning
		dc.Error = upErr.Error()
	} else {
		dc.Error = ""
	}
	api.recreateSuccessTimes[dc.WorkspaceFolder] = api.clock.Now("agentcontainers", "recreate", "successTimes")
	api.knownDevcontainers[dc.WorkspaceFolder] = dc
	api.broadcastUpdatesLocked()
	api.mu.Unlock()

	// Ensure an immediate refresh to accurately reflect the
	// devcontainer state after recreation.
	if err := api.RefreshContainers(ctx); err != nil {
		logger.Error(ctx, "failed to trigger immediate refresh after devcontainer creation", slog.Error(err))
		return xerrors.Errorf("refresh containers: %w", err)
	}

	return nil
}

// markDevcontainerDirty finds the devcontainer with the given config file path
// and marks it as dirty. It acquires the lock before modifying the state.
func (api *API) markDevcontainerDirty(configPath string, modifiedAt time.Time) {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Record the timestamp of when this configuration file was modified.
	api.configFileModifiedTimes[configPath] = modifiedAt

	for _, dc := range api.knownDevcontainers {
		if dc.ConfigPath != configPath {
			continue
		}

		logger := api.logger.With(
			slog.F("devcontainer_id", dc.ID),
			slog.F("devcontainer_name", dc.Name),
			slog.F("workspace_folder", dc.WorkspaceFolder),
			slog.F("file", configPath),
			slog.F("modified_at", modifiedAt),
		)

		// TODO(mafredri): Simplistic mark for now, we should check if the
		// container is running and if the config file was modified after
		// the container was created.
		if !dc.Dirty {
			logger.Info(api.ctx, "marking devcontainer as dirty")
			dc.Dirty = true
		}
		if _, ok := api.ignoredDevcontainers[dc.WorkspaceFolder]; ok {
			logger.Debug(api.ctx, "clearing devcontainer ignored state")
			delete(api.ignoredDevcontainers, dc.WorkspaceFolder) // Allow re-reading config.
		}

		api.knownDevcontainers[dc.WorkspaceFolder] = dc
	}

	api.broadcastUpdatesLocked()
}

// cleanupSubAgents removes subagents that are no longer managed by
// this agent. This is usually only run at startup to ensure a clean
// slate. This method has an internal timeout to prevent blocking
// indefinitely if something goes wrong with the subagent deletion.
func (api *API) cleanupSubAgents(ctx context.Context) error {
	client := *api.subAgentClient.Load()
	agents, err := client.List(ctx)
	if err != nil {
		return xerrors.Errorf("list agents: %w", err)
	}
	if len(agents) == 0 {
		return nil
	}

	api.mu.Lock()
	defer api.mu.Unlock()

	// Collect all subagent IDs that should be kept:
	// 1. Subagents currently tracked by injectedSubAgentProcs
	// 2. Subagents referenced by known devcontainers from the manifest
	keep := make(map[uuid.UUID]bool, len(api.injectedSubAgentProcs)+len(api.knownDevcontainers))
	for _, proc := range api.injectedSubAgentProcs {
		keep[proc.agent.ID] = true
	}
	for _, dc := range api.knownDevcontainers {
		if dc.SubagentID.Valid {
			keep[dc.SubagentID.UUID] = true
		}
	}

	ctx, cancel := context.WithTimeout(ctx, defaultOperationTimeout)
	defer cancel()

	var errs []error
	for _, agent := range agents {
		if keep[agent.ID] {
			continue
		}
		client := *api.subAgentClient.Load()
		err := client.Delete(ctx, agent.ID)
		if err != nil {
			api.logger.Error(ctx, "failed to delete agent",
				slog.Error(err),
				slog.F("agent_id", agent.ID),
				slog.F("agent_name", agent.Name),
			)
			errs = append(errs, xerrors.Errorf("delete agent %s (%s): %w", agent.Name, agent.ID, err))
		}
	}

	return errors.Join(errs...)
}

// maybeInjectSubAgentIntoContainerLocked injects a subagent into a dev
// container and starts the subagent process. This method assumes that
// api.mu is held. This method is idempotent and will not re-inject the
// subagent if it is already/still running in the container.
//
// This method uses an internal timeout to prevent blocking indefinitely
// if something goes wrong with the injection.
func (api *API) maybeInjectSubAgentIntoContainerLocked(ctx context.Context, dc codersdk.WorkspaceAgentDevcontainer) (err error) {
	if api.ignoredDevcontainers[dc.WorkspaceFolder] {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, defaultOperationTimeout)
	defer cancel()

	container := dc.Container
	if container == nil {
		return xerrors.New("container is nil, cannot inject subagent")
	}

	logger := api.logger.With(
		slog.F("devcontainer_id", dc.ID),
		slog.F("devcontainer_name", dc.Name),
		slog.F("workspace_folder", dc.WorkspaceFolder),
		slog.F("config_path", dc.ConfigPath),
		slog.F("container_id", container.ID),
		slog.F("container_name", container.FriendlyName),
	)

	// Check if subagent already exists for this devcontainer.
	maybeRecreateSubAgent := false
	proc, injected := api.injectedSubAgentProcs[dc.WorkspaceFolder]
	if injected {
		if _, ignoreChecked := api.ignoredDevcontainers[dc.WorkspaceFolder]; !ignoreChecked {
			// If ignore status has not yet been checked, or cleared by
			// modifications to the devcontainer.json, we must read it
			// to determine the current status. This can happen while
			// the  devcontainer subagent is already running or before
			// we've had a chance to inject it.
			//
			// Note, for simplicity, we do not try to optimize to reduce
			// ReadConfig calls here.
			config, err := api.dccli.ReadConfig(ctx, dc.WorkspaceFolder, dc.ConfigPath, nil)
			if err != nil {
				return xerrors.Errorf("read devcontainer config: %w", err)
			}

			dcIgnored := config.Configuration.Customizations.Coder.Ignore
			if dcIgnored {
				proc.stop()
				if proc.agent.ID != uuid.Nil {
					// Unlock while doing the delete operation.
					api.mu.Unlock()
					client := *api.subAgentClient.Load()
					if err := client.Delete(ctx, proc.agent.ID); err != nil {
						api.mu.Lock()
						return xerrors.Errorf("delete subagent: %w", err)
					}
					api.mu.Lock()
				}
				// Reset agent and containerID to force config re-reading if ignore is toggled.
				proc.agent = SubAgent{}
				proc.containerID = ""
				api.injectedSubAgentProcs[dc.WorkspaceFolder] = proc
				api.ignoredDevcontainers[dc.WorkspaceFolder] = dcIgnored
				return nil
			}
		}

		if proc.containerID == container.ID && proc.ctx.Err() == nil {
			// Same container and running, no need to reinject.
			return nil
		}

		if proc.containerID != container.ID {
			// Always recreate the subagent if the container ID changed
			// for now, in the future we can inspect e.g. if coder_apps
			// remain the same and avoid unnecessary recreation.
			logger.Debug(ctx, "container ID changed, injecting subagent into new container",
				slog.F("old_container_id", proc.containerID),
			)
			maybeRecreateSubAgent = proc.agent.ID != uuid.Nil
		}

		// Container ID changed or the subagent process is not running,
		// stop the existing subagent context to replace it.
		proc.stop()
	}
	if proc.agent.OperatingSystem == "" {
		// Set SubAgent defaults.
		proc.agent.OperatingSystem = "linux" // Assuming Linux for devcontainers.
	}

	// Prepare the subAgentProcess to be used when running the subagent.
	// We use api.ctx here to ensure that the process keeps running
	// after this method returns.
	proc.ctx, proc.stop = context.WithCancel(api.ctx)
	api.injectedSubAgentProcs[dc.WorkspaceFolder] = proc

	// This is used to track the goroutine that will run the subagent
	// process inside the container. It will be decremented when the
	// subagent process completes or if an error occurs before we can
	// start the subagent.
	api.asyncWg.Add(1)
	ranSubAgent := false

	// Clean up if injection fails.
	var dcIgnored, setDCIgnored bool
	defer func() {
		if setDCIgnored {
			api.ignoredDevcontainers[dc.WorkspaceFolder] = dcIgnored
		}
		if !ranSubAgent {
			proc.stop()
			if !api.closed {
				// Ensure sure state modifications are reflected.
				api.injectedSubAgentProcs[dc.WorkspaceFolder] = proc
			}
			api.asyncWg.Done()
		}
	}()

	// Unlock the mutex to allow other operations while we
	// inject the subagent into the container.
	api.mu.Unlock()
	defer api.mu.Lock() // Re-lock.

	arch, err := api.ccli.DetectArchitecture(ctx, container.ID)
	if err != nil {
		return xerrors.Errorf("detect architecture: %w", err)
	}

	logger.Info(ctx, "detected container architecture", slog.F("architecture", arch))

	// For now, only support injecting if the architecture matches the host.
	hostArch := runtime.GOARCH

	// TODO(mafredri): Add support for downloading agents for supported architectures.
	if arch != hostArch {
		logger.Warn(ctx, "skipping subagent injection for unsupported architecture",
			slog.F("container_arch", arch),
			slog.F("host_arch", hostArch),
		)
		return nil
	}
	if proc.agent.ID == uuid.Nil {
		proc.agent.Architecture = arch
	}

	subAgentConfig := proc.agent.CloneConfig(dc)
	if proc.agent.ID == uuid.Nil || maybeRecreateSubAgent {
		subAgentConfig.Architecture = arch

		displayAppsMap := map[codersdk.DisplayApp]bool{
			// NOTE(DanielleMaywood):
			// We use the same defaults here as set in terraform-provider-coder.
			// https://github.com/coder/terraform-provider-coder/blob/c1c33f6d556532e75662c0ca373ed8fdea220eb5/provider/agent.go#L38-L51
			codersdk.DisplayAppVSCodeDesktop:  true,
			codersdk.DisplayAppVSCodeInsiders: false,
			codersdk.DisplayAppWebTerminal:    true,
			codersdk.DisplayAppSSH:            true,
			codersdk.DisplayAppPortForward:    true,
		}

		var (
			featureOptionsAsEnvs       []string
			appsWithPossibleDuplicates []SubAgentApp
			workspaceFolder            = DevcontainerDefaultContainerWorkspaceFolder
		)

		if err := func() error {
			var (
				config         DevcontainerConfig
				configOutdated bool
			)

			readConfig := func() (DevcontainerConfig, error) {
				return api.dccli.ReadConfig(ctx, dc.WorkspaceFolder, dc.ConfigPath,
					append(featureOptionsAsEnvs, []string{
						fmt.Sprintf("CODER_WORKSPACE_AGENT_NAME=%s", subAgentConfig.Name),
						fmt.Sprintf("CODER_WORKSPACE_OWNER_NAME=%s", api.ownerName),
						fmt.Sprintf("CODER_WORKSPACE_NAME=%s", api.workspaceName),
						fmt.Sprintf("CODER_WORKSPACE_PARENT_AGENT_NAME=%s", api.parentAgent),
						fmt.Sprintf("CODER_URL=%s", api.subAgentURL),
						fmt.Sprintf("CONTAINER_ID=%s", container.ID),
					}...),
				)
			}

			if config, err = readConfig(); err != nil {
				return err
			}

			// We only allow ignore to be set in the root customization layer to
			// prevent weird interactions with devcontainer features.
			dcIgnored, setDCIgnored = config.Configuration.Customizations.Coder.Ignore, true
			if dcIgnored {
				return nil
			}

			workspaceFolder = config.Workspace.WorkspaceFolder

			featureOptionsAsEnvs = config.MergedConfiguration.Features.OptionsAsEnvs()
			if len(featureOptionsAsEnvs) > 0 {
				configOutdated = true
			}

			// NOTE(DanielleMaywood):
			// We only want to take an agent name specified in the root customization layer.
			// This restricts the ability for a feature to specify the agent name. We may revisit
			// this in the future, but for now we want to restrict this behavior.
			if name := config.Configuration.Customizations.Coder.Name; name != "" {
				// We only want to pick this name if it is a valid name.
				if provisioner.AgentNameRegex.Match([]byte(name)) {
					subAgentConfig.Name = name
					configOutdated = true
					delete(api.usingWorkspaceFolderName, dc.WorkspaceFolder)
				} else {
					logger.Warn(ctx, "invalid name in devcontainer customization, ignoring",
						slog.F("name", name),
						slog.F("regex", provisioner.AgentNameRegex.String()),
					)
				}
			}

			if configOutdated {
				if config, err = readConfig(); err != nil {
					return err
				}
			}

			coderCustomization := config.MergedConfiguration.Customizations.Coder

			for _, customization := range coderCustomization {
				for app, enabled := range customization.DisplayApps {
					if _, ok := displayAppsMap[app]; !ok {
						logger.Warn(ctx, "unknown display app in devcontainer customization, ignoring",
							slog.F("app", app),
							slog.F("enabled", enabled),
						)
						continue
					}
					displayAppsMap[app] = enabled
				}

				appsWithPossibleDuplicates = append(appsWithPossibleDuplicates, customization.Apps...)
			}

			return nil
		}(); err != nil {
			api.logger.Error(ctx, "unable to read devcontainer config", slog.Error(err))
		}

		if dcIgnored {
			proc.stop()
			if proc.agent.ID != uuid.Nil {
				// If we stop the subagent, we also need to delete it.
				client := *api.subAgentClient.Load()
				if err := client.Delete(ctx, proc.agent.ID); err != nil {
					return xerrors.Errorf("delete subagent: %w", err)
				}
			}
			// Reset agent and containerID to force config re-reading if
			// ignore is toggled.
			proc.agent = SubAgent{}
			proc.containerID = ""
			return nil
		}

		displayApps := make([]codersdk.DisplayApp, 0, len(displayAppsMap))
		for app, enabled := range displayAppsMap {
			if enabled {
				displayApps = append(displayApps, app)
			}
		}
		slices.Sort(displayApps)

		appSlugs := make(map[string]struct{})
		apps := make([]SubAgentApp, 0, len(appsWithPossibleDuplicates))

		// We want to deduplicate the apps based on their slugs here.
		// As we want to prioritize later apps, we will walk through this
		// backwards.
		for _, app := range slices.Backward(appsWithPossibleDuplicates) {
			if _, slugAlreadyExists := appSlugs[app.Slug]; slugAlreadyExists {
				continue
			}

			appSlugs[app.Slug] = struct{}{}
			apps = append(apps, app)
		}

		// Apps is currently in reverse order here, so by reversing it we restore
		// it to the original order.
		slices.Reverse(apps)

		subAgentConfig.DisplayApps = displayApps
		subAgentConfig.Apps = apps
		subAgentConfig.Directory = workspaceFolder
	}

	agentBinaryPath, err := os.Executable()
	if err != nil {
		return xerrors.Errorf("get agent binary path: %w", err)
	}
	agentBinaryPath, err = filepath.EvalSymlinks(agentBinaryPath)
	if err != nil {
		return xerrors.Errorf("resolve agent binary path: %w", err)
	}

	// If we scripted this as a `/bin/sh` script, we could reduce these
	// steps to one instruction, speeding up the injection process.
	//
	// Note: We use `path` instead of `filepath` here because we are
	// working with Unix-style paths inside the container.
	if _, err := api.ccli.ExecAs(ctx, container.ID, "root", "mkdir", "-p", path.Dir(coderPathInsideContainer)); err != nil {
		return xerrors.Errorf("create agent directory in container: %w", err)
	}

	if err := api.ccli.Copy(ctx, container.ID, agentBinaryPath, coderPathInsideContainer); err != nil {
		return xerrors.Errorf("copy agent binary: %w", err)
	}

	logger.Info(ctx, "copied agent binary to container")

	// Make sure the agent binary is executable so we can run it (the
	// user doesn't matter since we're making it executable for all).
	if _, err := api.ccli.ExecAs(ctx, container.ID, "root", "chmod", "0755", path.Dir(coderPathInsideContainer), coderPathInsideContainer); err != nil {
		return xerrors.Errorf("set agent binary executable: %w", err)
	}

	// Make sure the agent binary is owned by a valid user so we can run it.
	if _, err := api.ccli.ExecAs(ctx, container.ID, "root", "/bin/sh", "-c", fmt.Sprintf("chown $(id -u):$(id -g) %s", coderPathInsideContainer)); err != nil {
		return xerrors.Errorf("set agent binary ownership: %w", err)
	}

	// Attempt to add CAP_NET_ADMIN to the binary to improve network
	// performance (optional, allow to fail). See `bootstrap_linux.sh`.
	// TODO(mafredri): Disable for now until we can figure out why this
	// causes the following error on some images:
	//
	//	Image: mcr.microsoft.com/devcontainers/base:ubuntu
	// 	Error: /.coder-agent/coder: Operation not permitted
	//
	// if _, err := api.ccli.ExecAs(ctx, container.ID, "root", "setcap", "cap_net_admin+ep", coderPathInsideContainer); err != nil {
	// 	logger.Warn(ctx, "set CAP_NET_ADMIN on agent binary failed", slog.Error(err))
	// }

	// Only delete and recreate subagents that were dynamically created
	// (ID == uuid.Nil). Terraform-defined subagents (subAgentConfig.ID !=
	// uuid.Nil) must not be deleted because they have attached resources
	// managed by terraform.
	deleteSubAgent := subAgentConfig.ID == uuid.Nil && proc.agent.ID != uuid.Nil && maybeRecreateSubAgent && !proc.agent.EqualConfig(subAgentConfig)
	if deleteSubAgent {
		logger.Debug(ctx, "deleting existing subagent for recreation", slog.F("agent_id", proc.agent.ID))
		client := *api.subAgentClient.Load()
		err = client.Delete(ctx, proc.agent.ID)
		if err != nil {
			return xerrors.Errorf("delete existing subagent failed: %w", err)
		}
		proc.agent = SubAgent{} // Clear agent to signal that we need to create a new one.
	}

	if proc.agent.ID == uuid.Nil {
		logger.Debug(ctx, "creating new subagent",
			slog.F("directory", subAgentConfig.Directory),
			slog.F("display_apps", subAgentConfig.DisplayApps),
		)

		// Create new subagent record in the database to receive the auth token.
		// If we get a unique constraint violation, try with expanded names that
		// include parent directories to avoid collisions.
		client := *api.subAgentClient.Load()

		originalName := subAgentConfig.Name

		for attempt := 1; attempt <= maxAttemptsToNameAgent; attempt++ {
			agent, err := client.Create(ctx, subAgentConfig)
			if err == nil {
				proc.agent = agent // Only reassign on success.
				if api.usingWorkspaceFolderName[dc.WorkspaceFolder] {
					api.devcontainerNames[dc.Name] = true
					delete(api.usingWorkspaceFolderName, dc.WorkspaceFolder)
				}

				break
			}
			// NOTE(DanielleMaywood):
			// Ordinarily we'd use `errors.As` here, but it didn't appear to work. Not
			// sure if this is because of the communication protocol? Instead I've opted
			// for a slightly more janky string contains approach.
			//
			// We only care if sub agent creation has failed due to a unique constraint
			// violation on the agent name, as we can _possibly_ rectify this.
			if !strings.Contains(err.Error(), "workspace agent name") {
				return xerrors.Errorf("create subagent failed: %w", err)
			}

			// If there has been a unique constraint violation but the user is *not*
			// using an auto-generated name, then we should error. This is because
			// we do not want to surprise the user with a name they did not ask for.
			if usingFolderName := api.usingWorkspaceFolderName[dc.WorkspaceFolder]; !usingFolderName {
				return xerrors.Errorf("create subagent failed: %w", err)
			}

			if attempt == maxAttemptsToNameAgent {
				return xerrors.Errorf("create subagent failed after %d attempts: %w", attempt, err)
			}

			// We increase how much of the workspace folder is used for generating
			// the agent name. With each iteration there is greater chance of this
			// being successful.
			subAgentConfig.Name, api.usingWorkspaceFolderName[dc.WorkspaceFolder] = expandedAgentName(dc.WorkspaceFolder, dc.Container.FriendlyName, attempt)

			logger.Debug(ctx, "retrying subagent creation with expanded name",
				slog.F("original_name", originalName),
				slog.F("expanded_name", subAgentConfig.Name),
				slog.F("attempt", attempt+1),
			)
		}

		logger.Info(ctx, "created new subagent", slog.F("agent_id", proc.agent.ID))
	} else {
		logger.Debug(ctx, "subagent already exists, skipping recreation",
			slog.F("agent_id", proc.agent.ID),
		)
	}

	api.mu.Lock() // Re-lock to update the agent.
	defer api.mu.Unlock()
	if api.closed {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
		defer deleteCancel()
		client := *api.subAgentClient.Load()
		err := client.Delete(deleteCtx, proc.agent.ID)
		if err != nil {
			return xerrors.Errorf("delete existing subagent failed after API closed: %w", err)
		}
		return nil
	}
	// If we got this far, we should update the container ID to make
	// sure we don't retry. If we update it too soon we may end up
	// using an old subagent if e.g. delete failed previously.
	proc.containerID = container.ID
	api.injectedSubAgentProcs[dc.WorkspaceFolder] = proc

	// Start the subagent in the container in a new goroutine to avoid
	// blocking. Note that we pass the api.ctx to the subagent process
	// so that it isn't affected by the timeout.
	go api.runSubAgentInContainer(api.ctx, logger, dc, proc, coderPathInsideContainer)
	ranSubAgent = true

	return nil
}

// runSubAgentInContainer runs the subagent process inside a dev
// container. The api.asyncWg must be incremented before calling this
// function, and it will be decremented when the subagent process
// completes or if an error occurs.
func (api *API) runSubAgentInContainer(ctx context.Context, logger slog.Logger, dc codersdk.WorkspaceAgentDevcontainer, proc subAgentProcess, agentPath string) {
	container := dc.Container // Must not be nil.
	logger = logger.With(
		slog.F("agent_id", proc.agent.ID),
	)

	defer func() {
		proc.stop()
		logger.Debug(ctx, "agent process cleanup complete")
		api.asyncWg.Done()
	}()

	logger.Info(ctx, "starting subagent in devcontainer")

	env := []string{
		"CODER_AGENT_URL=" + api.subAgentURL,
		"CODER_AGENT_TOKEN=" + proc.agent.AuthToken.String(),
	}
	env = append(env, api.subAgentEnv...)
	err := api.dccli.Exec(proc.ctx, dc.WorkspaceFolder, dc.ConfigPath, agentPath, []string{"agent"},
		WithExecContainerID(container.ID),
		WithRemoteEnv(env...),
	)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(ctx, "subagent process failed", slog.Error(err))
	} else {
		logger.Info(ctx, "subagent process finished")
	}
}

func (api *API) Close() error {
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return nil
	}
	api.logger.Debug(api.ctx, "closing API")
	api.closed = true

	// Stop all running subagent processes and clean up.
	subAgentIDs := make([]uuid.UUID, 0, len(api.injectedSubAgentProcs))
	for workspaceFolder, proc := range api.injectedSubAgentProcs {
		api.logger.Debug(api.ctx, "canceling subagent process",
			slog.F("agent_name", proc.agent.Name),
			slog.F("agent_id", proc.agent.ID),
			slog.F("container_id", proc.containerID),
			slog.F("workspace_folder", workspaceFolder),
		)
		proc.stop()
		if proc.agent.ID != uuid.Nil {
			subAgentIDs = append(subAgentIDs, proc.agent.ID)
		}
	}
	api.injectedSubAgentProcs = make(map[string]subAgentProcess)

	api.cancel()    // Interrupt all routines.
	api.mu.Unlock() // Release lock before waiting for goroutines.

	// Note: We can't use api.ctx here because it's canceled.
	deleteCtx, deleteCancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer deleteCancel()
	client := *api.subAgentClient.Load()
	for _, id := range subAgentIDs {
		err := client.Delete(deleteCtx, id)
		if err != nil {
			api.logger.Error(api.ctx, "delete subagent record during shutdown failed",
				slog.Error(err),
				slog.F("agent_id", id),
			)
		}
	}

	// Close the watcher to ensure its loop finishes.
	err := api.watcher.Close()

	// Wait for loops to finish.
	if api.watcherDone != nil {
		<-api.watcherDone
	}
	if api.updaterDone != nil {
		<-api.updaterDone
	}
	if api.discoverDone != nil {
		<-api.discoverDone
	}

	// Wait for all async tasks to complete.
	api.asyncWg.Wait()

	api.logger.Debug(api.ctx, "closed API")
	return err
}
