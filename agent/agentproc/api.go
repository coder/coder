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
	"github.com/coder/coder/v2/agent/agentrunonce"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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
}

// NewAPI creates a new process API handler.
func NewAPI(logger slog.Logger, execer agentexec.Execer, fs afero.Fs, pathStore *agentgit.PathStore, envInfo usershell.EnvInfoer, updateEnv func(current []string) (updated []string, err error), workingDir func() string) *API {
	return &API{
		logger:    logger,
		manager:   newManager(logger, execer, fs, envInfo, updateEnv, workingDir),
		pathStore: pathStore,
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
	r.Post("/start", api.handleStartProcess)
	r.Get("/list", api.handleListProcesses)
	r.Get("/{id}/output", api.handleProcessOutput)
	r.Post("/{id}/signal", api.handleSignalProcess)
	r.Post("/signal-by-idempotency-key", api.handleSignalProcessByIdempotencyKey)
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

	proc, attached, err := api.manager.start(ctx, req, chatID)
	if err != nil {
		if errors.Is(err, agentrunonce.ErrInputMismatch) {
			httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ProcessConflictError{
				Code: workspacesdk.ProcessConflictInputMismatch,
				Response: codersdk.Response{
					Message: "Idempotency key was already used to start a process with different parameters.",
					Detail:  err.Error(),
				},
			})
			return
		}
		if errors.Is(err, agentrunonce.ErrPublicationPending) {
			// The concurrent start may still publish a process under
			// this key; 409 keeps the dispatch recoverable.
			httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ProcessConflictError{
				Code: workspacesdk.ProcessConflictStartPending,
				Response: codersdk.Response{
					Message: "Timed out waiting for the concurrent start that holds this idempotency key.",
					Detail:  err.Error(),
				},
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start process.",
			Detail:  err.Error(),
		})
		return
	}

	// Notify git watchers after the process finishes so that
	// file changes made by the command are visible in the scan.
	// If a workdir is provided, track it as a path as well.
	// An attached process already has a watcher from the request
	// that started it.
	if api.pathStore != nil && !attached {
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
		ID:             proc.id,
		Started:        !attached,
		IdempotencyKey: req.IdempotencyKey,
		Attached:       attached,
		StartedAt:      proc.info().StartedAt,
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

// handleSignalProcessByIdempotencyKey sends a signal to the process
// started under an idempotency key. It exists so a caller whose start
// response was lost can still stop the command it asked for: the key
// identifies the process even when its ID never arrived. The key
// travels in the body rather than the path so no key value needs URL
// escaping.
func (api *API) handleSignalProcessByIdempotencyKey(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var chatID string
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		chatID = chatContext.ID.String()
	}

	var req workspacesdk.SignalProcessByIdempotencyKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}
	if req.IdempotencyKey == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Idempotency key is required.",
		})
		return
	}
	if !validateSignal(ctx, rw, req.Signal) {
		return
	}

	if err := api.manager.signalByKey(ctx, chatID, req.IdempotencyKey, req.Signal); err != nil {
		switch {
		case errors.Is(err, errProcessNotFound):
			httpapi.Write(ctx, rw, http.StatusNotFound, workspacesdk.ProcessKeyNotFoundError{
				Code: workspacesdk.ProcessKeyNotFoundCode,
				Response: codersdk.Response{
					Message: "No process was started with this idempotency key.",
				},
			})
		case errors.Is(err, errProcessStartPending):
			httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ProcessConflictError{
				Code: workspacesdk.ProcessConflictStartPending,
				Response: codersdk.Response{
					Message: "The start holding this idempotency key has not spawned its process yet.",
				},
			})
		case errors.Is(err, errProcessNotRunning):
			httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ProcessConflictError{
				Code: workspacesdk.ProcessConflictNotRunning,
				Response: codersdk.Response{
					Message: "The process started with this idempotency key is not running.",
				},
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
		Message: fmt.Sprintf("Signal %q sent.", req.Signal),
	})
}

// validateSignal writes its own error response; false means the caller
// must stop.
func validateSignal(ctx context.Context, rw http.ResponseWriter, signal string) bool {
	if signal == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Signal is required.",
		})
		return false
	}
	if signal != "kill" && signal != "terminate" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(
				"Unsupported signal %q. Use \"kill\" or \"terminate\".",
				signal,
			),
		})
		return false
	}
	return true
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
	if !validateSignal(ctx, rw, req.Signal) {
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
