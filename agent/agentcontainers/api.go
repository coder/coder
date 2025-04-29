package agentcontainers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultGetContainersCacheDuration = 10 * time.Second
	dockerCreatedAtTimeFormat         = "2006-01-02 15:04:05 -0700 MST"
	getContainersTimeout              = 5 * time.Second
)

// API is responsible for container-related operations in the agent.
// It provides methods to list and manage containers.
type API struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	logger  slog.Logger
	watcher watcher.Watcher

	cacheDuration time.Duration
	execer        agentexec.Execer
	cl            Lister
	dccli         DevcontainerCLI
	clock         quartz.Clock

	// lockCh protects the below fields. We use a channel instead of a
	// mutex so we can handle cancellation properly.
	lockCh                  chan struct{}
	containers              codersdk.WorkspaceAgentListContainersResponse
	mtime                   time.Time
	devcontainerNames       map[string]struct{}                   // Track devcontainer names to avoid duplicates.
	knownDevcontainers      []codersdk.WorkspaceAgentDevcontainer // Track predefined and runtime-detected devcontainers.
	configFileModifiedTimes map[string]time.Time                  // Track when config files were last modified.
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

// WithLister sets the agentcontainers.Lister implementation to use.
// The default implementation uses the Docker CLI to list containers.
func WithLister(cl Lister) Option {
	return func(api *API) {
		api.cl = cl
	}
}

// WithDevcontainerCLI sets the DevcontainerCLI implementation to use.
// This can be used in tests to modify @devcontainer/cli behavior.
func WithDevcontainerCLI(dccli DevcontainerCLI) Option {
	return func(api *API) {
		api.dccli = dccli
	}
}

// WithDevcontainers sets the known devcontainers for the API. This
// allows the API to be aware of devcontainers defined in the workspace
// agent manifest.
func WithDevcontainers(devcontainers []codersdk.WorkspaceAgentDevcontainer) Option {
	return func(api *API) {
		if len(devcontainers) > 0 {
			api.knownDevcontainers = slices.Clone(devcontainers)
			api.devcontainerNames = make(map[string]struct{}, len(devcontainers))
			for _, devcontainer := range devcontainers {
				api.devcontainerNames[devcontainer.Name] = struct{}{}
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

// NewAPI returns a new API with the given options applied.
func NewAPI(logger slog.Logger, options ...Option) *API {
	ctx, cancel := context.WithCancel(context.Background())
	api := &API{
		ctx:                     ctx,
		cancel:                  cancel,
		done:                    make(chan struct{}),
		logger:                  logger,
		clock:                   quartz.NewReal(),
		execer:                  agentexec.DefaultExecer,
		cacheDuration:           defaultGetContainersCacheDuration,
		lockCh:                  make(chan struct{}, 1),
		devcontainerNames:       make(map[string]struct{}),
		knownDevcontainers:      []codersdk.WorkspaceAgentDevcontainer{},
		configFileModifiedTimes: make(map[string]time.Time),
	}
	for _, opt := range options {
		opt(api)
	}
	if api.cl == nil {
		api.cl = NewDocker(api.execer)
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

	go api.loop()

	return api
}

// Ready signals the API that we are ready to begin watching for file
// changes. This is used to prime the cache with the current list of
// containers and to start watching the devcontainer config files for
// changes. It should be called after the agent ready.
func (api *API) Ready() {
	// Prime the cache with the current list of containers.
	_, _ = api.cl.List(api.ctx)

	// Make sure we watch the devcontainer config files for changes.
	for _, devcontainer := range api.knownDevcontainers {
		if devcontainer.ConfigPath == "" {
			continue
		}

		if err := api.watcher.Add(devcontainer.ConfigPath); err != nil {
			api.logger.Error(api.ctx, "watch devcontainer config file failed", slog.Error(err), slog.F("file", devcontainer.ConfigPath))
		}
	}
}

func (api *API) loop() {
	defer close(api.done)

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

		now := api.clock.Now()
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

// Routes returns the HTTP handler for container-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()

	r.Get("/", api.handleList)
	r.Get("/devcontainers", api.handleListDevcontainers)
	r.Post("/{id}/recreate", api.handleRecreate)

	return r
}

// handleList handles the HTTP request to list containers.
func (api *API) handleList(rw http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
		// Client went away.
		return
	default:
		ct, err := api.getContainers(r.Context())
		if err != nil {
			if errors.Is(err, context.Canceled) {
				httpapi.Write(r.Context(), rw, http.StatusRequestTimeout, codersdk.Response{
					Message: "Could not get containers.",
					Detail:  "Took too long to list containers.",
				})
				return
			}
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Could not get containers.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(r.Context(), rw, http.StatusOK, ct)
	}
}

func copyListContainersResponse(resp codersdk.WorkspaceAgentListContainersResponse) codersdk.WorkspaceAgentListContainersResponse {
	return codersdk.WorkspaceAgentListContainersResponse{
		Containers: slices.Clone(resp.Containers),
		Warnings:   slices.Clone(resp.Warnings),
	}
}

func (api *API) getContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	select {
	case <-api.ctx.Done():
		return codersdk.WorkspaceAgentListContainersResponse{}, api.ctx.Err()
	case <-ctx.Done():
		return codersdk.WorkspaceAgentListContainersResponse{}, ctx.Err()
	case api.lockCh <- struct{}{}:
		defer func() { <-api.lockCh }()
	}

	now := api.clock.Now()
	if now.Sub(api.mtime) < api.cacheDuration {
		return copyListContainersResponse(api.containers), nil
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, getContainersTimeout)
	defer timeoutCancel()
	updated, err := api.cl.List(timeoutCtx)
	if err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("get containers: %w", err)
	}
	api.containers = updated
	api.mtime = now

	dirtyStates := make(map[string]bool)
	// Reset all known devcontainers to not running.
	for i := range api.knownDevcontainers {
		api.knownDevcontainers[i].Running = false
		api.knownDevcontainers[i].Container = nil

		// Preserve the dirty state and store in map for lookup.
		dirtyStates[api.knownDevcontainers[i].WorkspaceFolder] = api.knownDevcontainers[i].Dirty
	}

	// Check if the container is running and update the known devcontainers.
	for _, container := range updated.Containers {
		workspaceFolder := container.Labels[DevcontainerLocalFolderLabel]
		configFile := container.Labels[DevcontainerConfigFileLabel]

		if workspaceFolder == "" {
			continue
		}

		// Check if this is already in our known list.
		if knownIndex := slices.IndexFunc(api.knownDevcontainers, func(dc codersdk.WorkspaceAgentDevcontainer) bool {
			return dc.WorkspaceFolder == workspaceFolder
		}); knownIndex != -1 {
			// Update existing entry with runtime information.
			if configFile != "" && api.knownDevcontainers[knownIndex].ConfigPath == "" {
				api.knownDevcontainers[knownIndex].ConfigPath = configFile
				if err := api.watcher.Add(configFile); err != nil {
					api.logger.Error(ctx, "watch devcontainer config file failed", slog.Error(err), slog.F("file", configFile))
				}
			}
			api.knownDevcontainers[knownIndex].Running = container.Running
			api.knownDevcontainers[knownIndex].Container = &container

			// Check if this container was created after the config
			// file was modified.
			if configFile != "" && api.knownDevcontainers[knownIndex].Dirty {
				lastModified, hasModTime := api.configFileModifiedTimes[configFile]
				if hasModTime && container.CreatedAt.After(lastModified) {
					api.logger.Info(ctx, "clearing dirty flag for container created after config modification",
						slog.F("container", container.ID),
						slog.F("created_at", container.CreatedAt),
						slog.F("config_modified_at", lastModified),
						slog.F("file", configFile),
					)
					api.knownDevcontainers[knownIndex].Dirty = false
				}
			}
			continue
		}

		// NOTE(mafredri): This name impl. may change to accommodate devcontainer agents RFC.
		// If not in our known list, add as a runtime detected entry.
		name := path.Base(workspaceFolder)
		if _, ok := api.devcontainerNames[name]; ok {
			// Try to find a unique name by appending a number.
			for i := 2; ; i++ {
				newName := fmt.Sprintf("%s-%d", name, i)
				if _, ok := api.devcontainerNames[newName]; !ok {
					name = newName
					break
				}
			}
		}
		api.devcontainerNames[name] = struct{}{}
		if configFile != "" {
			if err := api.watcher.Add(configFile); err != nil {
				api.logger.Error(ctx, "watch devcontainer config file failed", slog.Error(err), slog.F("file", configFile))
			}
		}

		dirty := dirtyStates[workspaceFolder]
		if dirty {
			lastModified, hasModTime := api.configFileModifiedTimes[configFile]
			if hasModTime && container.CreatedAt.After(lastModified) {
				api.logger.Info(ctx, "new container created after config modification, not marking as dirty",
					slog.F("container", container.ID),
					slog.F("created_at", container.CreatedAt),
					slog.F("config_modified_at", lastModified),
					slog.F("file", configFile),
				)
				dirty = false
			}
		}

		api.knownDevcontainers = append(api.knownDevcontainers, codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            name,
			WorkspaceFolder: workspaceFolder,
			ConfigPath:      configFile,
			Running:         container.Running,
			Dirty:           dirty,
			Container:       &container,
		})
	}

	return copyListContainersResponse(api.containers), nil
}

// handleRecreate handles the HTTP request to recreate a container.
func (api *API) handleRecreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing container ID or name",
			Detail:  "Container ID or name is required to recreate a devcontainer.",
		})
		return
	}

	containers, err := api.getContainers(ctx)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not list containers",
			Detail:  err.Error(),
		})
		return
	}

	containerIdx := slices.IndexFunc(containers.Containers, func(c codersdk.WorkspaceAgentContainer) bool {
		return c.Match(id)
	})
	if containerIdx == -1 {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
			Message: "Container not found",
			Detail:  "Container ID or name not found in the list of containers.",
		})
		return
	}

	container := containers.Containers[containerIdx]
	workspaceFolder := container.Labels[DevcontainerLocalFolderLabel]
	configPath := container.Labels[DevcontainerConfigFileLabel]

	// Workspace folder is required to recreate a container, we don't verify
	// the config path here because it's optional.
	if workspaceFolder == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing workspace folder label",
			Detail:  "The workspace folder label is required to recreate a devcontainer.",
		})
		return
	}

	_, err = api.dccli.Up(ctx, workspaceFolder, configPath, WithRemoveExistingContainer())
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not recreate devcontainer",
			Detail:  err.Error(),
		})
		return
	}

	// TODO(mafredri): Temporarily handle clearing the dirty state after
	// recreation, later on this should be handled by a "container watcher".
	select {
	case <-api.ctx.Done():
		return
	case <-ctx.Done():
		return
	case api.lockCh <- struct{}{}:
		defer func() { <-api.lockCh }()
	}
	for i := range api.knownDevcontainers {
		if api.knownDevcontainers[i].WorkspaceFolder == workspaceFolder {
			if api.knownDevcontainers[i].Dirty {
				api.logger.Info(ctx, "clearing dirty flag after recreation",
					slog.F("workspace_folder", workspaceFolder),
					slog.F("name", api.knownDevcontainers[i].Name),
				)
				api.knownDevcontainers[i].Dirty = false
			}
			break
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListDevcontainers handles the HTTP request to list known devcontainers.
func (api *API) handleListDevcontainers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Run getContainers to detect the latest devcontainers and their state.
	_, err := api.getContainers(ctx)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not list containers",
			Detail:  err.Error(),
		})
		return
	}

	select {
	case <-api.ctx.Done():
		return
	case <-ctx.Done():
		return
	case api.lockCh <- struct{}{}:
	}
	devcontainers := slices.Clone(api.knownDevcontainers)
	<-api.lockCh

	slices.SortFunc(devcontainers, func(a, b codersdk.WorkspaceAgentDevcontainer) int {
		if cmp := strings.Compare(a.WorkspaceFolder, b.WorkspaceFolder); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ConfigPath, b.ConfigPath)
	})

	response := codersdk.WorkspaceAgentDevcontainersResponse{
		Devcontainers: devcontainers,
	}

	httpapi.Write(ctx, w, http.StatusOK, response)
}

// markDevcontainerDirty finds the devcontainer with the given config file path
// and marks it as dirty. It acquires the lock before modifying the state.
func (api *API) markDevcontainerDirty(configPath string, modifiedAt time.Time) {
	select {
	case <-api.ctx.Done():
		return
	case api.lockCh <- struct{}{}:
		defer func() { <-api.lockCh }()
	}

	// Record the timestamp of when this configuration file was modified.
	api.configFileModifiedTimes[configPath] = modifiedAt

	for i := range api.knownDevcontainers {
		if api.knownDevcontainers[i].ConfigPath != configPath {
			continue
		}

		// TODO(mafredri): Simplistic mark for now, we should check if the
		// container is running and if the config file was modified after
		// the container was created.
		if !api.knownDevcontainers[i].Dirty {
			api.logger.Info(api.ctx, "marking devcontainer as dirty",
				slog.F("file", configPath),
				slog.F("name", api.knownDevcontainers[i].Name),
				slog.F("workspace_folder", api.knownDevcontainers[i].WorkspaceFolder),
				slog.F("modified_at", modifiedAt),
			)
			api.knownDevcontainers[i].Dirty = true
		}
	}
}

func (api *API) Close() error {
	api.cancel()
	<-api.done
	err := api.watcher.Close()
	if err != nil {
		return err
	}
	return nil
}
