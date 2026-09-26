package agentproc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spf13/afero"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const (
	// maxWaitDuration is the maximum time a blocking
	// process output request can wait, regardless of
	// what the client requests.
	maxWaitDuration = 5 * time.Minute
)

// API exposes process-related operations through the agent.
type API struct {
	logger    slog.Logger
	manager   *manager
	pathStore *agentgit.PathStore
	// toolCalls runs the start route at most once per tool call. Nil
	// leaves tool call headers unhandled.
	toolCalls *agenttoolcall.Store
}

// Option configures an API.
type Option func(*apiOptions)

type apiOptions struct {
	clock     quartz.Clock
	toolCalls *agenttoolcall.Store
}

// WithClock sets the clock used for process timestamps and run age.
func WithClock(clock quartz.Clock) Option {
	return func(o *apiOptions) {
		o.clock = clock
	}
}

// WithToolCallStore runs the start route through store.Middleware, so a
// start with tool call headers runs at most once and its process ID is the
// tool call UUID. The store's records also keep exited tool call
// processes from being reaped while they are current.
func WithToolCallStore(store *agenttoolcall.Store) Option {
	return func(o *apiOptions) {
		o.toolCalls = store
	}
}

// NewAPI creates a new process API handler.
func NewAPI(logger slog.Logger, execer agentexec.Execer, fs afero.Fs, pathStore *agentgit.PathStore, envInfo usershell.EnvInfoer, updateEnv func(current []string) (updated []string, err error), workingDir func() string, opts ...Option) *API {
	options := apiOptions{clock: quartz.NewReal()}
	for _, opt := range opts {
		opt(&options)
	}
	return &API{
		logger:    logger,
		manager:   newManager(logger, execer, fs, envInfo, updateEnv, workingDir, options.clock, options.toolCalls),
		pathStore: pathStore,
		toolCalls: options.toolCalls,
	}
}

// Close shuts down the process manager, killing all running
// processes.
func (api *API) Close() error {
	return api.manager.Close()
}

// Routes returns the HTTP handler for process-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	if api.toolCalls != nil {
		r.With(api.toolCalls.Middleware).Post("/start", api.handleStartProcess)
	} else {
		r.Post("/start", api.handleStartProcess)
	}
	r.Get("/list", api.handleListProcesses)
	r.Get("/{id}/output", api.handleProcessOutput)
	r.Post("/{id}/signal", api.handleSignalProcess)
	return r
}

// handleStartProcess starts a new process.
func (api *API) handleStartProcess(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req workspacesdk.StartProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	if req.Command == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Command is required.",
		})
		return
	}

	var chatID string
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		chatID = chatContext.ID.String()
	}

	// Behind the tool call middleware, the process ID is the tool call
	// UUID, so chatd can address the process without this response.
	id := uuid.New().String()
	var key *agenttoolcall.Key
	if toolCall, ok := agenttoolcall.FromContext(ctx); ok {
		id = toolCall.UUID.String()
		key = &toolCall.Key
	}

	proc, err := api.manager.start(req, chatID, id, key)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start process.",
			Detail:  err.Error(),
		})
		return
	}

	// Notify git watchers after the process finishes so that
	// file changes made by the command are visible in the scan.
	// If a workdir is provided, track it as a path as well.
	if api.pathStore != nil {
		if chatContext, ok := agentchat.FromContext(ctx); ok {
			allIDs := append([]uuid.UUID{chatContext.ID}, chatContext.AncestorIDs...)
			go func() {
				<-proc.done
				if req.WorkDir != "" {
					api.pathStore.AddPaths(allIDs, []string{req.WorkDir})
				} else {
					api.pathStore.Notify(allIDs)
				}
			}()
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.StartProcessResponse{
		ID:      proc.id,
		Started: true,
	})
}

// handleListProcesses lists all tracked processes.
func (api *API) handleListProcesses(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var chatID string
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		chatID = chatContext.ID.String()
	}

	infos := api.manager.list(chatID)

	// Sort by running state (running first), then by started_at
	// descending so the most recent processes appear first.
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Running != infos[j].Running {
			return infos[i].Running
		}
		return infos[i].StartedAt > infos[j].StartedAt
	})

	// Cap the response to avoid bloating LLM context.
	const maxListProcesses = 10
	if len(infos) > maxListProcesses {
		infos = infos[:maxListProcesses]
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.ListProcessesResponse{
		Processes: infos,
	})
}

// handleProcessOutput returns the output of a process.
func (api *API) handleProcessOutput(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := api.logger.With(agentchat.Fields(ctx)...)

	id := chi.URLParam(r, "id")
	proc, ok := api.manager.get(id)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Process %q not found.", id),
		})
		return
	}

	// Enforce chat ID isolation. If the request carries
	// a chat context, only allow access to processes
	// belonging to that chat.
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		if proc.chatID != "" && proc.chatID != chatContext.ID.String() {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("Process %q not found.", id),
			})
			return
		}
	}

	// Check for blocking mode via query params.
	waitStr := r.URL.Query().Get("wait")
	wantWait := waitStr == "true"

	if wantWait {
		// Extend the write deadline so the HTTP server's
		// WriteTimeout does not kill the connection while
		// we block.
		rc := http.NewResponseController(rw)
		// Add headroom beyond the wait timeout so there's time to
		// write the response after the blocking wait completes.
		if err := rc.SetWriteDeadline(time.Now().Add(maxWaitDuration + 30*time.Second)); err != nil {
			logger.Error(ctx, "extend write deadline for blocking process output",
				slog.Error(err),
			)
		}

		// Cap the wait at maxWaitDuration regardless of
		// client-supplied timeout.
		waitCtx, waitCancel := context.WithTimeout(ctx, maxWaitDuration)
		defer waitCancel()

		_ = proc.waitForOutput(waitCtx)
		// Fall through to read snapshot below.
	}

	// Read info before output to avoid a TOCTOU race. The exit
	// goroutine completes all buffer writes (cmd.Wait) before
	// setting running=false, so if info reports the process as
	// exited, the subsequent output read is guaranteed to reflect
	// the final buffer state.
	info := proc.info()
	output, truncated := proc.output()

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.ProcessOutputResponse{
		Output:    output,
		Truncated: truncated,
		Running:   info.Running,
		ExitCode:  info.ExitCode,
		Command:   info.Command,
	})
}

// handleSignalProcess sends a signal to a running process.
func (api *API) handleSignalProcess(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := chi.URLParam(r, "id")

	// Enforce chat ID isolation.
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		proc, procOK := api.manager.get(id)
		if procOK && proc.chatID != "" && proc.chatID != chatContext.ID.String() {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("Process %q not found.", id),
			})
			return
		}
	}

	var req workspacesdk.SignalProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	if req.Signal == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Signal is required.",
		})
		return
	}

	if req.Signal != "kill" && req.Signal != "terminate" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(
				"Unsupported signal %q. Use \"kill\" or \"terminate\".",
				req.Signal,
			),
		})
		return
	}

	if err := api.manager.signal(id, req.Signal); err != nil {
		switch {
		case errors.Is(err, errProcessNotFound):
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("Process %q not found.", id),
			})
		case errors.Is(err, errProcessNotRunning):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf(
					"Process %q is not running.", id,
				),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to signal process.",
				Detail:  err.Error(),
			})
		}
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: fmt.Sprintf(
			"Signal %q sent to process %q.", req.Signal, id,
		),
	})
}

// StopToolCallProcess implements agenttoolcall.ProcessStopper for the
// cancel route. It kills the process if it is running with a run age below
// stopIfRunAgeBelow, waits for it to exit, and returns its state. A process
// at or above that age, such as a foreground command past its execute
// deadline or a background process, keeps running. found is false when no
// process has processID.
func (api *API) StopToolCallProcess(ctx context.Context, processID string, stopIfRunAgeBelow time.Duration) (state workspacesdk.ToolCallProcess, found bool, err error) {
	proc, ok := api.manager.get(processID)
	if !ok {
		return workspacesdk.ToolCallProcess{}, false, nil
	}
	if err := api.manager.stop(ctx, proc, stopIfRunAgeBelow); err != nil {
		return workspacesdk.ToolCallProcess{}, true, err
	}
	return api.manager.toolCallProcessState(proc), true, nil
}
