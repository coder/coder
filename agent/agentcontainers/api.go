package agentcontainers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
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
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/quartz"
)

const (
	defaultUpdateInterval = 10 * time.Second
	listContainersTimeout = 15 * time.Second
)

// API is responsible for container-related operations in the agent.
// It provides methods to list and manage containers.
type API struct {
	ctx               context.Context
	cancel            context.CancelFunc
	watcherDone       chan struct{}
	updaterDone       chan struct{}
	initialUpdateDone chan struct{}   // Closed after first update in updaterLoop.
	refreshTrigger    chan chan error // Channel to trigger manual refresh.
	updateInterval    time.Duration   // Interval for periodic container updates.
	logger            slog.Logger
	watcher           watcher.Watcher
	execer            agentexec.Execer
	cl                Lister
	dccli             DevcontainerCLI
	clock             quartz.Clock
	scriptLogger      func(logSourceID uuid.UUID) ScriptLogger

	mu                      sync.RWMutex
	closed                  bool
	containers              codersdk.WorkspaceAgentListContainersResponse // Output from the last list operation.
	containersErr           error                                         // Error from the last list operation.
	devcontainerNames       map[string]struct{}
	knownDevcontainers      []codersdk.WorkspaceAgentDevcontainer
	configFileModifiedTimes map[string]time.Time

	devcontainerLogSourceIDs map[string]uuid.UUID // Track devcontainer log source IDs.
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
func WithDevcontainers(devcontainers []codersdk.WorkspaceAgentDevcontainer, scripts []codersdk.WorkspaceAgentScript) Option {
	return func(api *API) {
		if len(devcontainers) == 0 {
			return
		}
		api.knownDevcontainers = slices.Clone(devcontainers)
		api.devcontainerNames = make(map[string]struct{}, len(devcontainers))
		api.devcontainerLogSourceIDs = make(map[string]uuid.UUID)
		for _, devcontainer := range devcontainers {
			api.devcontainerNames[devcontainer.Name] = struct{}{}
			for _, script := range scripts {
				// The devcontainer scripts match the devcontainer ID for
				// identification.
				if script.ID == devcontainer.ID {
					api.devcontainerLogSourceIDs[devcontainer.WorkspaceFolder] = script.LogSourceID
					break
				}
			}
			if api.devcontainerLogSourceIDs[devcontainer.WorkspaceFolder] == uuid.Nil {
				api.logger.Error(api.ctx, "devcontainer log source ID not found for devcontainer",
					slog.F("devcontainer", devcontainer.Name),
					slog.F("workspace_folder", devcontainer.WorkspaceFolder),
					slog.F("config_path", devcontainer.ConfigPath),
				)
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
		ctx:                     ctx,
		cancel:                  cancel,
		watcherDone:             make(chan struct{}),
		updaterDone:             make(chan struct{}),
		initialUpdateDone:       make(chan struct{}),
		refreshTrigger:          make(chan chan error),
		updateInterval:          defaultUpdateInterval,
		logger:                  logger,
		clock:                   quartz.NewReal(),
		execer:                  agentexec.DefaultExecer,
		devcontainerNames:       make(map[string]struct{}),
		knownDevcontainers:      []codersdk.WorkspaceAgentDevcontainer{},
		configFileModifiedTimes: make(map[string]time.Time),
		scriptLogger:            func(uuid.UUID) ScriptLogger { return noopScriptLogger{} },
	}
	// The ctx and logger must be set before applying options to avoid
	// nil pointer dereference.
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

	go api.watcherLoop()
	go api.updaterLoop()

	return api
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

// updaterLoop is responsible for periodically updating the container
// list and handling manual refresh requests.
func (api *API) updaterLoop() {
	defer close(api.updaterDone)
	defer api.logger.Debug(api.ctx, "updater loop stopped")
	api.logger.Debug(api.ctx, "updater loop started")

	// Ensure that only once instance of the updateContainers is running
	// at a time. This is a workaround since quartz.Ticker does not
	// allow us to know if the routine has completed.
	sema := make(chan struct{}, 1)
	sema <- struct{}{}

	// Ensure only one updateContainers is running at a time, others are
	// queued.
	doUpdate := func() error {
		select {
		case <-api.ctx.Done():
			return api.ctx.Err()
		case <-sema:
		}
		defer func() { sema <- struct{}{} }()

		return api.updateContainers(api.ctx)
	}

	api.logger.Debug(api.ctx, "performing initial containers update")
	if err := doUpdate(); err != nil {
		api.logger.Error(api.ctx, "initial containers update failed", slog.Error(err))
	} else {
		api.logger.Debug(api.ctx, "initial containers update complete")
	}
	// Signal that the initial update attempt (successful or not) is done.
	// Other services can wait on this if they need the first data to be available.
	close(api.initialUpdateDone)

	// Use a ticker func to ensure that doUpdate has run to completion
	// when advancing time.
	waiter := api.clock.TickerFunc(api.ctx, api.updateInterval, func() error {
		err := doUpdate()
		if err != nil {
			api.logger.Error(api.ctx, "periodic containers update failed", slog.Error(err))
		}
		return nil // Always nil, keep going.
	})
	defer func() {
		if err := waiter.Wait(); err != nil {
			api.logger.Error(api.ctx, "updater loop ticker failed", slog.Error(err))
		}
	}()

	for {
		select {
		case <-api.ctx.Done():
			api.logger.Debug(api.ctx, "updater loop context canceled")
			return
		case ch := <-api.refreshTrigger:
			api.logger.Debug(api.ctx, "manual containers update triggered")
			err := doUpdate()
			if err != nil {
				api.logger.Error(api.ctx, "manual containers update failed", slog.Error(err))
			}
			ch <- err
			close(ch)
		}
	}
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
				// Initial update is done, we can start processing
				// requests.
			}
			next.ServeHTTP(rw, r)
		})
	}

	// For now, all endpoints require the initial update to be done.
	// If we want to allow some endpoints to be available before
	// the initial update, we can enable this per-route.
	r.Use(ensureInitialUpdateDoneMW)

	r.Get("/", api.handleList)
	r.Route("/devcontainers", func(r chi.Router) {
		r.Get("/", api.handleDevcontainersList)
		r.Post("/container/{container}/recreate", api.handleDevcontainerRecreate)
	})

	return r
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
	listCtx, listCancel := context.WithTimeout(ctx, listContainersTimeout)
	defer listCancel()

	updated, err := api.cl.List(listCtx)
	if err != nil {
		// If the context was canceled, we hold off on clearing the
		// containers cache. This is to avoid clearing the cache if
		// the update was canceled due to a timeout. Hopefully this
		// will clear up on the next update.
		if !errors.Is(err, context.Canceled) {
			api.mu.Lock()
			api.containers = codersdk.WorkspaceAgentListContainersResponse{}
			api.containersErr = err
			api.mu.Unlock()
		}

		return xerrors.Errorf("list containers failed: %w", err)
	}

	api.mu.Lock()
	defer api.mu.Unlock()

	api.processUpdatedContainersLocked(ctx, updated)

	api.logger.Debug(ctx, "containers updated successfully", slog.F("container_count", len(api.containers.Containers)), slog.F("warning_count", len(api.containers.Warnings)), slog.F("devcontainer_count", len(api.knownDevcontainers)))

	return nil
}

// processUpdatedContainersLocked updates the devcontainer state based
// on the latest list of containers. This method assumes that api.mu is
// held.
func (api *API) processUpdatedContainersLocked(ctx context.Context, updated codersdk.WorkspaceAgentListContainersResponse) {
	dirtyStates := make(map[string]bool)
	// Reset all known devcontainers to not running.
	for i := range api.knownDevcontainers {
		api.knownDevcontainers[i].Running = false
		api.knownDevcontainers[i].Container = nil

		// Preserve the dirty state and store in map for lookup.
		dirtyStates[api.knownDevcontainers[i].WorkspaceFolder] = api.knownDevcontainers[i].Dirty
	}

	// Check if the container is running and update the known devcontainers.
	updatedDevcontainers := make(map[string]bool)
	for i := range updated.Containers {
		container := &updated.Containers[i]
		workspaceFolder := container.Labels[DevcontainerLocalFolderLabel]
		configFile := container.Labels[DevcontainerConfigFileLabel]

		if workspaceFolder == "" {
			continue
		}

		if lastModified, hasModTime := api.configFileModifiedTimes[configFile]; !hasModTime || container.CreatedAt.Before(lastModified) {
			api.logger.Debug(ctx, "container created before config modification, setting dirty state from devcontainer",
				slog.F("container", container.ID),
				slog.F("created_at", container.CreatedAt),
				slog.F("config_modified_at", lastModified),
				slog.F("file", configFile),
				slog.F("workspace_folder", workspaceFolder),
				slog.F("dirty", dirtyStates[workspaceFolder]),
			)
			container.DevcontainerDirty = dirtyStates[workspaceFolder]
		}

		// Check if this is already in our known list.
		if knownIndex := slices.IndexFunc(api.knownDevcontainers, func(dc codersdk.WorkspaceAgentDevcontainer) bool {
			return dc.WorkspaceFolder == workspaceFolder
		}); knownIndex != -1 {
			// Update existing entry with runtime information.
			dc := &api.knownDevcontainers[knownIndex]
			if configFile != "" && dc.ConfigPath == "" {
				dc.ConfigPath = configFile
				if err := api.watcher.Add(configFile); err != nil {
					api.logger.Error(ctx, "watch devcontainer config file failed", slog.Error(err), slog.F("file", configFile))
				}
			}
			dc.Running = container.Running
			dc.Container = container
			dc.Dirty = container.DevcontainerDirty
			updatedDevcontainers[workspaceFolder] = true
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

		api.knownDevcontainers = append(api.knownDevcontainers, codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            name,
			WorkspaceFolder: workspaceFolder,
			ConfigPath:      configFile,
			Running:         container.Running,
			Dirty:           container.DevcontainerDirty,
			Container:       container,
		})
		updatedDevcontainers[workspaceFolder] = true
	}

	for i := range api.knownDevcontainers {
		if _, ok := updatedDevcontainers[api.knownDevcontainers[i].WorkspaceFolder]; ok {
			continue
		}

		dc := &api.knownDevcontainers[i]

		if !dc.Running && !dc.Dirty && dc.Container == nil {
			// Already marked as not running, skip.
			continue
		}

		api.logger.Debug(ctx, "devcontainer is not running anymore, marking as not running",
			slog.F("workspace_folder", dc.WorkspaceFolder),
			slog.F("config_path", dc.ConfigPath),
			slog.F("name", dc.Name),
		)
		dc.Running = false
		dc.Dirty = false
		dc.Container = nil
	}

	api.containers = updated
	api.containersErr = nil
}

// refreshContainers triggers an immediate update of the container list
// and waits for it to complete.
func (api *API) refreshContainers(ctx context.Context) error {
	done := make(chan error, 1)
	select {
	case <-api.ctx.Done():
		return xerrors.Errorf("API closed, cannot send refresh trigger: %w", api.ctx.Err())
	case <-ctx.Done():
		return ctx.Err()
	case api.refreshTrigger <- done:
		select {
		case <-api.ctx.Done():
			return xerrors.Errorf("API closed, cannot wait for refresh: %w", api.ctx.Err())
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
	return codersdk.WorkspaceAgentListContainersResponse{
		Containers: slices.Clone(api.containers.Containers),
		Warnings:   slices.Clone(api.containers.Warnings),
	}, nil
}

// handleDevcontainerRecreate handles the HTTP request to recreate a
// devcontainer by referencing the container.
func (api *API) handleDevcontainerRecreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	containerID := chi.URLParam(r, "container")

	if containerID == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing container ID or name",
			Detail:  "Container ID or name is required to recreate a devcontainer.",
		})
		return
	}

	containers, err := api.getContainers()
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not list containers",
			Detail:  err.Error(),
		})
		return
	}

	containerIdx := slices.IndexFunc(containers.Containers, func(c codersdk.WorkspaceAgentContainer) bool {
		return c.Match(containerID)
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
			Detail:  "The container is not a devcontainer, the container must have the workspace folder label to support recreation.",
		})
		return
	}

	// Send logs via agent logging facilities.
	logSourceID := api.devcontainerLogSourceIDs[workspaceFolder]
	if logSourceID == uuid.Nil {
		// Fallback to the external log source ID if not found.
		logSourceID = agentsdk.ExternalLogSourceID
	}
	scriptLogger := api.scriptLogger(logSourceID)
	defer func() {
		flushCtx, cancel := context.WithTimeout(api.ctx, 5*time.Second)
		defer cancel()
		if err := scriptLogger.Flush(flushCtx); err != nil {
			api.logger.Error(flushCtx, "flush devcontainer logs failed", slog.Error(err))
		}
	}()
	infoW := agentsdk.LogsWriter(ctx, scriptLogger.Send, logSourceID, codersdk.LogLevelInfo)
	defer infoW.Close()
	errW := agentsdk.LogsWriter(ctx, scriptLogger.Send, logSourceID, codersdk.LogLevelError)
	defer errW.Close()

	_, err = api.dccli.Up(ctx, workspaceFolder, configPath, WithOutput(infoW, errW), WithRemoveExistingContainer())
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not recreate devcontainer",
			Detail:  err.Error(),
		})
		return
	}

	// NOTE(mafredri): This won't be needed once recreation is done async.
	if err := api.refreshContainers(r.Context()); err != nil {
		api.logger.Error(ctx, "failed to trigger immediate refresh after devcontainer recreation", slog.Error(err))
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDevcontainersList handles the HTTP request to list known devcontainers.
func (api *API) handleDevcontainersList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.mu.RLock()
	err := api.containersErr
	devcontainers := slices.Clone(api.knownDevcontainers)
	api.mu.RUnlock()
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not list containers",
			Detail:  err.Error(),
		})
		return
	}

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
	api.mu.Lock()
	defer api.mu.Unlock()

	// Record the timestamp of when this configuration file was modified.
	api.configFileModifiedTimes[configPath] = modifiedAt

	for i := range api.knownDevcontainers {
		dc := &api.knownDevcontainers[i]
		if dc.ConfigPath != configPath {
			continue
		}

		logger := api.logger.With(
			slog.F("file", configPath),
			slog.F("name", dc.Name),
			slog.F("workspace_folder", dc.WorkspaceFolder),
			slog.F("modified_at", modifiedAt),
		)

		// TODO(mafredri): Simplistic mark for now, we should check if the
		// container is running and if the config file was modified after
		// the container was created.
		if !dc.Dirty {
			logger.Info(api.ctx, "marking devcontainer as dirty")
			dc.Dirty = true
		}
		if dc.Container != nil && !dc.Container.DevcontainerDirty {
			logger.Info(api.ctx, "marking devcontainer container as dirty")
			dc.Container.DevcontainerDirty = true
		}
	}
}

func (api *API) Close() error {
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return nil
	}
	api.closed = true

	api.logger.Debug(api.ctx, "closing API")
	defer api.logger.Debug(api.ctx, "closed API")

	api.cancel()
	err := api.watcher.Close()

	api.mu.Unlock()
	<-api.watcherDone
	<-api.updaterDone

	return err
}
