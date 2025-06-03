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
	updateTrigger     chan chan error // Channel to trigger manual refresh.
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
	containers              codersdk.WorkspaceAgentListContainersResponse  // Output from the last list operation.
	containersErr           error                                          // Error from the last list operation.
	devcontainerNames       map[string]bool                                // By devcontainer name.
	knownDevcontainers      map[string]codersdk.WorkspaceAgentDevcontainer // By workspace folder.
	configFileModifiedTimes map[string]time.Time                           // By config file path.
	recreateSuccessTimes    map[string]time.Time                           // By workspace folder.
	recreateErrorTimes      map[string]time.Time                           // By workspace folder.
	recreateWg              sync.WaitGroup

	devcontainerLogSourceIDs map[string]uuid.UUID // By workspace folder.
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
		api.knownDevcontainers = make(map[string]codersdk.WorkspaceAgentDevcontainer, len(devcontainers))
		api.devcontainerNames = make(map[string]bool, len(devcontainers))
		api.devcontainerLogSourceIDs = make(map[string]uuid.UUID)
		for _, dc := range devcontainers {
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
				api.logger.Error(api.ctx, "devcontainer log source ID not found for devcontainer",
					slog.F("devcontainer_id", dc.ID),
					slog.F("devcontainer_name", dc.Name),
					slog.F("workspace_folder", dc.WorkspaceFolder),
					slog.F("config_path", dc.ConfigPath),
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
		updateTrigger:           make(chan chan error),
		updateInterval:          defaultUpdateInterval,
		logger:                  logger,
		clock:                   quartz.NewReal(),
		execer:                  agentexec.DefaultExecer,
		devcontainerNames:       make(map[string]bool),
		knownDevcontainers:      make(map[string]codersdk.WorkspaceAgentDevcontainer),
		configFileModifiedTimes: make(map[string]time.Time),
		recreateSuccessTimes:    make(map[string]time.Time),
		recreateErrorTimes:      make(map[string]time.Time),
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

		now := api.clock.Now("watcherLoop")
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

	// Perform an initial update to populate the container list, this
	// gives us a guarantee that the API has loaded the initial state
	// before returning any responses. This is useful for both tests
	// and anyone looking to interact with the API.
	api.logger.Debug(api.ctx, "performing initial containers update")
	if err := api.updateContainers(api.ctx); err != nil {
		api.logger.Error(api.ctx, "initial containers update failed", slog.Error(err))
	} else {
		api.logger.Debug(api.ctx, "initial containers update complete")
	}
	// Signal that the initial update attempt (successful or not) is done.
	// Other services can wait on this if they need the first data to be available.
	close(api.initialUpdateDone)

	// We utilize a TickerFunc here instead of a regular Ticker so that
	// we can guarantee execution of the updateContainers method after
	// advancing the clock.
	ticker := api.clock.TickerFunc(api.ctx, api.updateInterval, func() error {
		done := make(chan error, 1)
		defer close(done)

		select {
		case <-api.ctx.Done():
			return api.ctx.Err()
		case api.updateTrigger <- done:
			err := <-done
			if err != nil {
				api.logger.Error(api.ctx, "updater loop ticker failed", slog.Error(err))
			}
		default:
			api.logger.Debug(api.ctx, "updater loop ticker skipped, update in progress")
		}

		return nil // Always nil to keep the ticker going.
	}, "updaterLoop")
	defer func() {
		if err := ticker.Wait("updaterLoop"); err != nil && !errors.Is(err, context.Canceled) {
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
	// Reset the container links in known devcontainers to detect if
	// they still exist.
	for _, dc := range api.knownDevcontainers {
		dc.Container = nil
		api.knownDevcontainers[dc.WorkspaceFolder] = dc
	}

	// Check if the container is running and update the known devcontainers.
	for i := range updated.Containers {
		container := &updated.Containers[i] // Grab a reference to the container to allow mutating it.
		container.DevcontainerStatus = ""   // Reset the status for the container (updated later).
		container.DevcontainerDirty = false // Reset dirty state for the container (updated later).

		workspaceFolder := container.Labels[DevcontainerLocalFolderLabel]
		configFile := container.Labels[DevcontainerConfigFileLabel]

		if workspaceFolder == "" {
			continue
		}

		if dc, ok := api.knownDevcontainers[workspaceFolder]; ok {
			// If no config path is set, this devcontainer was defined
			// in Terraform without the optional config file. Assume the
			// first container with the workspace folder label is the
			// one we want to use.
			if dc.ConfigPath == "" && configFile != "" {
				dc.ConfigPath = configFile
				if err := api.watcher.Add(configFile); err != nil {
					api.logger.Error(ctx, "watch devcontainer config file failed", slog.Error(err), slog.F("file", configFile))
				}
			}

			dc.Container = container
			api.knownDevcontainers[dc.WorkspaceFolder] = dc
			continue
		}

		// NOTE(mafredri): This name impl. may change to accommodate devcontainer agents RFC.
		// If not in our known list, add as a runtime detected entry.
		name := path.Base(workspaceFolder)
		if api.devcontainerNames[name] {
			// Try to find a unique name by appending a number.
			for i := 2; ; i++ {
				newName := fmt.Sprintf("%s-%d", name, i)
				if !api.devcontainerNames[newName] {
					name = newName
					break
				}
			}
		}
		api.devcontainerNames[name] = true
		if configFile != "" {
			if err := api.watcher.Add(configFile); err != nil {
				api.logger.Error(ctx, "watch devcontainer config file failed", slog.Error(err), slog.F("file", configFile))
			}
		}

		api.knownDevcontainers[workspaceFolder] = codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            name,
			WorkspaceFolder: workspaceFolder,
			ConfigPath:      configFile,
			Status:          "",    // Updated later based on container state.
			Dirty:           false, // Updated later based on config file changes.
			Container:       container,
		}
	}

	// Iterate through all known devcontainers and update their status
	// based on the current state of the containers.
	for _, dc := range api.knownDevcontainers {
		switch {
		case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusStarting:
			if dc.Container != nil {
				dc.Container.DevcontainerStatus = dc.Status
				dc.Container.DevcontainerDirty = dc.Dirty
			}
			continue // This state is handled by the recreation routine.

		case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusError && (dc.Container == nil || dc.Container.CreatedAt.Before(api.recreateErrorTimes[dc.WorkspaceFolder])):
			if dc.Container != nil {
				dc.Container.DevcontainerStatus = dc.Status
				dc.Container.DevcontainerDirty = dc.Dirty
			}
			continue // The devcontainer needs to be recreated.

		case dc.Container != nil:
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
			if dc.Container.Running {
				dc.Status = codersdk.WorkspaceAgentDevcontainerStatusRunning
			}
			dc.Container.DevcontainerStatus = dc.Status

			dc.Dirty = false
			if lastModified, hasModTime := api.configFileModifiedTimes[dc.ConfigPath]; hasModTime && dc.Container.CreatedAt.Before(lastModified) {
				dc.Dirty = true
			}
			dc.Container.DevcontainerDirty = dc.Dirty

		case dc.Container == nil:
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
			dc.Dirty = false
		}

		delete(api.recreateErrorTimes, dc.WorkspaceFolder)
		api.knownDevcontainers[dc.WorkspaceFolder] = dc
	}

	api.containers = updated
	api.containersErr = nil
}

// refreshContainers triggers an immediate update of the container list
// and waits for it to complete.
func (api *API) refreshContainers(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = xerrors.Errorf("refresh containers failed: %w", err)
		}
	}()

	done := make(chan error, 1)
	select {
	case <-api.ctx.Done():
		return xerrors.Errorf("API closed: %w", api.ctx.Err())
	case <-ctx.Done():
		return ctx.Err()
	case api.updateTrigger <- done:
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

	containerIdx := slices.IndexFunc(containers.Containers, func(c codersdk.WorkspaceAgentContainer) bool { return c.Match(containerID) })
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

	api.mu.Lock()

	dc, ok := api.knownDevcontainers[workspaceFolder]
	switch {
	case !ok:
		api.mu.Unlock()

		// This case should not happen if the container is a valid devcontainer.
		api.logger.Error(ctx, "devcontainer not found for workspace folder", slog.F("workspace_folder", workspaceFolder))
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Devcontainer not found.",
			Detail:  fmt.Sprintf("Could not find devcontainer for workspace folder: %q", workspaceFolder),
		})
		return
	case dc.Status == codersdk.WorkspaceAgentDevcontainerStatusStarting:
		api.mu.Unlock()

		httpapi.Write(ctx, w, http.StatusConflict, codersdk.Response{
			Message: "Devcontainer recreation already in progress",
			Detail:  fmt.Sprintf("Recreation for workspace folder %q is already underway.", dc.WorkspaceFolder),
		})
		return
	}

	// Update the status so that we don't try to recreate the
	// devcontainer multiple times in parallel.
	dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStarting
	if dc.Container != nil {
		dc.Container.DevcontainerStatus = dc.Status
	}
	api.knownDevcontainers[dc.WorkspaceFolder] = dc
	api.recreateWg.Add(1)
	go api.recreateDevcontainer(dc, configPath)

	api.mu.Unlock()

	httpapi.Write(ctx, w, http.StatusAccepted, codersdk.Response{
		Message: "Devcontainer recreation initiated",
		Detail:  fmt.Sprintf("Recreation process for workspace folder %q has started.", dc.WorkspaceFolder),
	})
}

// recreateDevcontainer should run in its own goroutine and is responsible for
// recreating a devcontainer based on the provided devcontainer configuration.
// It updates the devcontainer status and logs the process. The configPath is
// passed as a parameter for the odd chance that the container being recreated
// has a different config file than the one stored in the devcontainer state.
// The devcontainer state must be set to starting and the recreateWg must be
// incremented before calling this function.
func (api *API) recreateDevcontainer(dc codersdk.WorkspaceAgentDevcontainer, configPath string) {
	defer api.recreateWg.Done()

	var (
		err    error
		ctx    = api.ctx
		logger = api.logger.With(
			slog.F("devcontainer_id", dc.ID),
			slog.F("devcontainer_name", dc.Name),
			slog.F("workspace_folder", dc.WorkspaceFolder),
			slog.F("config_path", configPath),
		)
	)

	if dc.ConfigPath != configPath {
		logger.Warn(ctx, "devcontainer config path mismatch",
			slog.F("config_path_param", configPath),
		)
	}

	// Send logs via agent logging facilities.
	logSourceID := api.devcontainerLogSourceIDs[dc.WorkspaceFolder]
	if logSourceID == uuid.Nil {
		// Fallback to the external log source ID if not found.
		logSourceID = agentsdk.ExternalLogSourceID
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

	_, err = api.dccli.Up(ctx, dc.WorkspaceFolder, configPath, WithOutput(infoW, errW), WithRemoveExistingContainer())
	if err != nil {
		// No need to log if the API is closing (context canceled), as this
		// is expected behavior when the API is shutting down.
		if !errors.Is(err, context.Canceled) {
			logger.Error(ctx, "devcontainer recreation failed", slog.Error(err))
		}

		api.mu.Lock()
		dc = api.knownDevcontainers[dc.WorkspaceFolder]
		dc.Status = codersdk.WorkspaceAgentDevcontainerStatusError
		if dc.Container != nil {
			dc.Container.DevcontainerStatus = dc.Status
		}
		api.knownDevcontainers[dc.WorkspaceFolder] = dc
		api.recreateErrorTimes[dc.WorkspaceFolder] = api.clock.Now("recreate", "errorTimes")
		api.mu.Unlock()
		return
	}

	logger.Info(ctx, "devcontainer recreated successfully")

	api.mu.Lock()
	dc = api.knownDevcontainers[dc.WorkspaceFolder]
	// Update the devcontainer status to Running or Stopped based on the
	// current state of the container, changing the status to !starting
	// allows the update routine to update the devcontainer status, but
	// to minimize the time between API consistency, we guess the status
	// based on the container state.
	dc.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
	if dc.Container != nil {
		if dc.Container.Running {
			dc.Status = codersdk.WorkspaceAgentDevcontainerStatusRunning
		}
		dc.Container.DevcontainerStatus = dc.Status
	}
	dc.Dirty = false
	api.recreateSuccessTimes[dc.WorkspaceFolder] = api.clock.Now("recreate", "successTimes")
	api.knownDevcontainers[dc.WorkspaceFolder] = dc
	api.mu.Unlock()

	// Ensure an immediate refresh to accurately reflect the
	// devcontainer state after recreation.
	if err := api.refreshContainers(ctx); err != nil {
		logger.Error(ctx, "failed to trigger immediate refresh after devcontainer recreation", slog.Error(err))
	}
}

// handleDevcontainersList handles the HTTP request to list known devcontainers.
func (api *API) handleDevcontainersList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.mu.RLock()
	err := api.containersErr
	devcontainers := make([]codersdk.WorkspaceAgentDevcontainer, 0, len(api.knownDevcontainers))
	for _, dc := range api.knownDevcontainers {
		devcontainers = append(devcontainers, dc)
	}
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
		if dc.Container != nil && !dc.Container.DevcontainerDirty {
			logger.Info(api.ctx, "marking devcontainer container as dirty")
			dc.Container.DevcontainerDirty = true
		}

		api.knownDevcontainers[dc.WorkspaceFolder] = dc
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
	api.cancel()    // Interrupt all routines.
	api.mu.Unlock() // Release lock before waiting for goroutines.

	// Close the watcher to ensure its loop finishes.
	err := api.watcher.Close()

	// Wait for loops to finish.
	<-api.watcherDone
	<-api.updaterDone

	// Wait for all devcontainer recreation tasks to complete.
	api.recreateWg.Wait()

	api.logger.Debug(api.ctx, "closed API")
	return err
}
