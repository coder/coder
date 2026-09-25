package agentproc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

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
}

// Option configures an API.
type Option func(*apiOptions)

type apiOptions struct {
	clock quartz.Clock
}

// WithClock sets the clock used for process timestamps, process age, and
// the agent start that tool call decisions compare against.
func WithClock(clock quartz.Clock) Option {
	return func(o *apiOptions) {
		o.clock = clock
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
		manager:   newManager(logger, execer, fs, envInfo, updateEnv, workingDir, options.clock),
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
	r.Post("/{id}/cancel", api.handleCancelProcess)
	return r
}

// handleStartProcess starts a new process. With tool call headers it
// starts at most one process per tool call, whose ID is the tool call
// UUID, and returns that process to repeated requests.
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

	toolCall, hasToolCall, err := workspacesdk.ToolCallFromHeaders(r.Header)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid tool call headers.",
			Detail:  err.Error(),
		})
		return
	}

	var proc *process
	if hasToolCall {
		chatContext, ok := agentchat.FromContext(ctx)
		if !ok {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Tool call headers require the %s header.", workspacesdk.CoderChatIDHeader),
			})
			return
		}
		key := agenttoolcall.Key{ChatID: chatContext.ID, MessageID: toolCall.MessageID, ToolCallID: toolCall.ID}
		proc, err = api.startToolCall(ctx, req, key, toolCall.Age)
		if writeToolCallError(ctx, rw, err) {
			return
		}
	} else {
		proc, err = api.startProcess(ctx, req, uuid.New().String(), nil)
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start process.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.StartProcessResponse{
		ID:      proc.id,
		Started: true,
		AgeMs:   api.manager.age(proc).Milliseconds(),
	})
}

// startToolCall starts the process for a tool call at most once, with
// the tool call UUID as its ID, and returns the recorded process or
// error to repeated requests.
func (api *API) startToolCall(ctx context.Context, req workspacesdk.StartProcessRequest, key agenttoolcall.Key, age time.Duration) (*process, error) {
	// The input is the request as sent, including the requested WorkDir
	// rather than the resolved one, so a repeated request matches even
	// if the agent's working directory changed in between.
	body, err := json.Marshal(req)
	if err != nil {
		return nil, xerrors.Errorf("encode process request: %w", err)
	}
	id := workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolCallID).String()
	return api.manager.records.Start(ctx, key, age, sha256.Sum256(body), func() (*process, error) {
		return api.startProcess(ctx, req, id, &key)
	})
}

// startProcess spawns a process with the given ID and chat context from
// ctx, and notifies git watchers when it exits.
func (api *API) startProcess(ctx context.Context, req workspacesdk.StartProcessRequest, id string, toolCall *agenttoolcall.Key) (*process, error) {
	var chatID string
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		chatID = chatContext.ID.String()
	}

	proc, err := api.manager.start(req, chatID, id, toolCall)
	if err != nil {
		return nil, err
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

	return proc, nil
}

// writeToolCallError writes the HTTP 409 for an agenttoolcall decision
// error and reports whether err was one.
func writeToolCallError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	code, ok := agenttoolcall.ErrorCode(err)
	if !ok {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ToolCallError{
		Response: codersdk.Response{Message: err.Error()},
		Code:     code,
	})
	return true
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
		AgeMs:     api.manager.age(proc).Milliseconds(),
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

// handleCancelProcess cancels a tool call's process. It kills a running
// process and waits for it to exit, returns an exited process's result
// unchanged, and records a tool call the agent never received as
// canceled so that a start still in transit does nothing.
func (api *API) handleCancelProcess(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	chatContext, ok := agentchat.FromContext(ctx)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Canceling a process requires the %s header.", workspacesdk.CoderChatIDHeader),
		})
		return
	}
	toolCall, ok, err := workspacesdk.ToolCallFromHeaders(r.Header)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid tool call headers.",
			Detail:  err.Error(),
		})
		return
	}
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Canceling a process requires tool call headers.",
		})
		return
	}
	key := agenttoolcall.Key{ChatID: chatContext.ID, MessageID: toolCall.MessageID, ToolCallID: toolCall.ID}
	wantID := workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolCallID).String()
	if id != wantID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Process ID %q is not the ID of the tool call in the headers.", id),
		})
		return
	}

	proc, started, err := api.manager.records.Cancel(ctx, key, toolCall.Age)
	// Cancel reports an aborted wait for a pending start on the same path
	// as a recorded start error. The client is gone, and answering
	// started=false for a process that is starting would be wrong.
	if err != nil && ctx.Err() != nil {
		return
	}
	if writeToolCallError(ctx, rw, err) {
		return
	}
	// Any other error is the recorded start error: no process started.
	if !started || err != nil {
		httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelProcessResponse{})
		return
	}

	killed, err := proc.killAndWait(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to cancel process.",
			Detail:  err.Error(),
		})
		return
	}
	// The process has exited, so its output is final.
	info := proc.info()
	output, truncated := proc.output()
	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelProcessResponse{
		Started:   true,
		Canceled:  killed,
		Output:    output,
		Truncated: truncated,
		ExitCode:  info.ExitCode,
		AgeMs:     api.manager.age(proc).Milliseconds(),
	})
}
