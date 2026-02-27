package agentproc

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// StartProcessRequest is the request body for starting a
// new process.
type StartProcessRequest struct {
	Command    string            `json:"command"`
	WorkDir    string            `json:"workdir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Background bool              `json:"background,omitempty"`
}

// StartProcessResponse is returned after a process is started.
type StartProcessResponse struct {
	ID      string `json:"id"`
	Started bool   `json:"started"`
}

// ListProcessesResponse is the response for listing all
// tracked processes.
type ListProcessesResponse struct {
	Processes []ProcessInfo `json:"processes"`
}

// ProcessInfo describes the state of a tracked process.
type ProcessInfo struct {
	ID         string `json:"id"`
	Command    string `json:"command"`
	WorkDir    string `json:"workdir,omitempty"`
	Background bool   `json:"background"`
	Running    bool   `json:"running"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	StartedAt  int64  `json:"started_at_unix"`
	ExitedAt   *int64 `json:"exited_at_unix,omitempty"`
}

// ProcessOutputResponse is returned when fetching process
// output.
type ProcessOutputResponse struct {
	Output    string          `json:"output"`
	Truncated *TruncationInfo `json:"truncated,omitempty"`
	Running   bool            `json:"running"`
	ExitCode  *int            `json:"exit_code,omitempty"`
}

// SignalProcessRequest is the request body for signaling a
// process.
type SignalProcessRequest struct {
	Signal string `json:"signal"` // "kill" or "terminate"
}

// API exposes process-related operations through the agent.
type API struct {
	logger  slog.Logger
	manager *Manager
}

// NewAPI creates a new process API handler.
func NewAPI(logger slog.Logger, execer agentexec.Execer) *API {
	return &API{
		logger:  logger,
		manager: NewManager(logger, execer),
	}
}

// Routes returns the HTTP handler for process-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/start", api.HandleStartProcess)
	r.Get("/list", api.HandleListProcesses)
	r.Get("/{id}/output", api.HandleProcessOutput)
	r.Post("/{id}/signal", api.HandleSignalProcess)
	return r
}

// HandleStartProcess starts a new process.
func (api *API) HandleStartProcess(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req StartProcessRequest
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

	proc, err := api.manager.Start(ctx, req)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start process.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, StartProcessResponse{
		ID:      proc.id,
		Started: true,
	})
}

// HandleListProcesses lists all tracked processes.
func (api *API) HandleListProcesses(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	infos := api.manager.List()
	httpapi.Write(ctx, rw, http.StatusOK, ListProcessesResponse{
		Processes: infos,
	})
}

// HandleProcessOutput returns the output of a process.
func (api *API) HandleProcessOutput(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := chi.URLParam(r, "id")
	proc, ok := api.manager.Get(id)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Process %q not found.", id),
		})
		return
	}

	output, truncated := proc.Output()
	info := proc.Info()

	httpapi.Write(ctx, rw, http.StatusOK, ProcessOutputResponse{
		Output:    output,
		Truncated: truncated,
		Running:   info.Running,
		ExitCode:  info.ExitCode,
	})
}

// HandleSignalProcess sends a signal to a running process.
func (api *API) HandleSignalProcess(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := chi.URLParam(r, "id")

	var req SignalProcessRequest
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

	if err := api.manager.Signal(id, req.Signal); err != nil {
		// Distinguish between not found and other errors.
		_, exists := api.manager.Get(id)
		if !exists {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("Process %q not found.", id),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to signal process.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: fmt.Sprintf(
			"Signal %q sent to process %q.", req.Signal, id,
		),
	})
}
