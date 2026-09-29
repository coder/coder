package coderd

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) registerWorkspaceCommandRoutes(r chi.Router) {
	r.Post("/workspace-executions/{session}/commands", api.startWorkspaceCommand)
	r.Get("/workspace-executions/{session}/commands/{execution}", api.workspaceCommand)
	r.Post("/workspace-executions/{session}/commands/{execution}/cancel", api.cancelWorkspaceCommand)
}

func (api *API) commandAgentAuthorized(r *http.Request, session database.WorkspaceExecutionSession, agentID uuid.UUID) (bool, error) {
	workspace, err := api.Database.GetWorkspaceByAgentID(r.Context(), agentID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return false, nil
		}
		return false, xerrors.Errorf("read command workspace: %w", err)
	}
	return session.WorkspaceID.Valid && workspace.ID == session.WorkspaceID.UUID &&
		workspace.OwnerID == session.WorkspaceOwnerID.UUID && workspace.OrganizationID == session.OrganizationID &&
		!workspace.Deleted && api.Authorize(r, policy.ActionSSH, workspace), nil
}

// @Summary Start a durable workspace command
// @ID start-a-durable-workspace-command
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Accept json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Param request body codersdk.StartWorkspaceCommandRequest true "Command intent"
// @Success 200 {object} codersdk.WorkspaceCommand
// @Failure 403 {object} codersdk.Response
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/commands [post]
func (api *API) startWorkspaceCommand(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	session, ok := api.workspaceExecutionSession(rw, r, policy.ActionSSH)
	if !ok {
		return
	}
	var req codersdk.StartWorkspaceCommandRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	if req.RequestID == uuid.Nil || req.AgentID == uuid.Nil || req.Command == "" {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "request_id, agent_id, and command are required."})
		return
	}
	input := workspaceexec.StartInput{SessionID: session.ID, ActorID: httpmw.APIKey(r).UserID, RequestID: req.RequestID, AgentID: req.AgentID, Command: req.Command, WorkDir: req.WorkDir, Env: req.Env}
	// Recovery must work without the original workspace or agent connection.
	_, err := api.Database.GetWorkspaceExecutionReceiptByRequest(r.Context(), database.GetWorkspaceExecutionReceiptByRequestParams{SessionID: session.ID, ActorID: input.ActorID, RequestID: req.RequestID})
	var receipt database.WorkspaceExecutionReceipt
	if err == nil {
		receipt, err = workspaceexec.Start(r.Context(), api.Database, nil, input, api.Clock.Now())
	} else if xerrors.Is(err, sql.ErrNoRows) {
		authorized, authErr := api.commandAgentAuthorized(r, session, req.AgentID)
		if authErr != nil {
			httpapi.InternalServerError(rw, authErr)
			return
		}
		if !authorized {
			httpapi.ResourceNotFound(rw)
			return
		}
		if api.Entitlements.Enabled(codersdk.FeatureBrowserOnly) {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{Message: "Non-browser agent connections are disabled by deployment policy."})
			return
		}
		conn, release, dialErr := api.agentProvider.AgentConn(r.Context(), req.AgentID)
		if dialErr != nil {
			httpapi.Write(r.Context(), rw, http.StatusServiceUnavailable, codersdk.Response{Message: "Agent unavailable; command was not admitted.", Detail: dialErr.Error()})
			return
		}
		defer release()
		receipt, err = workspaceexec.Start(r.Context(), api.Database, conn, input, api.Clock.Now())
	}
	if err != nil {
		status := http.StatusInternalServerError
		if xerrors.Is(err, workspaceexec.ErrRequestConflict) || xerrors.Is(err, workspaceexec.ErrAdmissionClosed) || xerrors.Is(err, sql.ErrNoRows) {
			status = http.StatusConflict
		}
		if xerrors.Is(err, workspaceexec.ErrUnsupportedAgent) {
			status = http.StatusPreconditionFailed
		}
		httpapi.Write(r.Context(), rw, status, codersdk.Response{Message: "Command admission failed.", Detail: err.Error()})
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, convertWorkspaceCommand(receipt))
}

func commandWait(rw http.ResponseWriter, r *http.Request) (int64, bool) {
	wait := int64(0)
	var err error
	if raw := r.URL.Query().Get("wait_ms"); raw != "" {
		wait, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil || wait < 0 || wait > 30000 {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "wait_ms must be between 0 and 30000."})
		return 0, false
	}
	return wait, true
}

// @Summary Read workspace command output and status
// @ID read-workspace-command-output-and-status
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Param execution path string true "Command ID" format(uuid)
// @Param wait_ms query int false "Bounded observation wait, 0 to 30000 milliseconds"
// @Success 200 {object} codersdk.WorkspaceCommand
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/commands/{execution} [get]
func (api *API) workspaceCommand(rw http.ResponseWriter, r *http.Request) {
	api.observeWorkspaceCommand(rw, r)
}

// @Summary Cancel a workspace command with acknowledgment
// @ID cancel-a-workspace-command-with-acknowledgment
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Param execution path string true "Command ID" format(uuid)
// @Param wait_ms query int false "Bounded acknowledgment wait, 0 to 30000 milliseconds"
// @Success 200 {object} codersdk.WorkspaceCommand
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/commands/{execution}/cancel [post]
func (api *API) cancelWorkspaceCommand(rw http.ResponseWriter, r *http.Request) {
	api.observeWorkspaceCommand(rw, r)
}

func (api *API) observeWorkspaceCommand(rw http.ResponseWriter, r *http.Request) {
	ctx, finish := context.WithTimeout(r.Context(), 35*time.Second)
	defer finish()
	r = r.WithContext(ctx)
	session, ok := api.workspaceExecutionSession(rw, r, policy.ActionSSH)
	if !ok {
		return
	}
	id, ok := httpmw.ParseUUIDParam(rw, r, "execution")
	if !ok {
		return
	}
	wait, ok := commandWait(rw, r)
	if !ok {
		return
	}
	receipt, err := api.Database.GetWorkspaceExecutionReceiptByID(r.Context(), id)
	if err != nil && !httpapi.Is404Error(err) {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{Message: "Read command receipt.", Detail: err.Error()})
		return
	}
	if err != nil || receipt.SessionID != session.ID || receipt.OrganizationID != session.OrganizationID {
		httpapi.ResourceNotFound(rw)
		return
	}
	result := convertWorkspaceCommand(receipt)
	if api.Entitlements.Enabled(codersdk.FeatureBrowserOnly) {
		result.OutputUnavailable = true
		result.Error = "Non-browser agent connections are disabled by deployment policy."
		httpapi.Write(r.Context(), rw, http.StatusOK, result)
		return
	}
	authorized, authErr := api.commandAgentAuthorized(r, session, receipt.AgentID)
	if authErr != nil {
		result.OutputUnavailable = true
		result.Error = authErr.Error()
		httpapi.Write(r.Context(), rw, http.StatusOK, result)
		return
	}
	if !authorized {
		result.OutputUnavailable = true
		result.Error = "Original agent is unavailable or workspace ownership changed."
		httpapi.Write(r.Context(), rw, http.StatusOK, result)
		return
	}
	conn, release, err := api.agentProvider.AgentConn(r.Context(), receipt.AgentID)
	if err != nil {
		result.OutputUnavailable = true
		result.Error = err.Error()
		httpapi.Write(r.Context(), rw, http.StatusOK, result)
		return
	}
	defer release()
	if r.Method == http.MethodPost {
		receipt, err = workspaceexec.Cancel(r.Context(), api.Database, conn, receipt, wait, api.Clock.Now())
		result = convertWorkspaceCommand(receipt)
	} else {
		updated, output, observeErr := workspaceexec.Observe(r.Context(), api.Database, conn, receipt, wait, api.Clock.Now())
		result = convertWorkspaceCommand(updated)
		err = observeErr
		result.OutputUnavailable = err != nil
		if err == nil {
			result.Output = &codersdk.WorkspaceCommandOutput{Text: output.Output}
			if output.Truncated != nil {
				t := output.Truncated
				result.Output.Truncated = &codersdk.WorkspaceCommandTruncation{OriginalBytes: t.OriginalBytes, RetainedBytes: t.RetainedBytes, OmittedBytes: t.OmittedBytes, Strategy: t.Strategy}
			}
		}
	}
	if err != nil {
		result.Error = err.Error()
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, result)
}

func convertWorkspaceCommand(row database.WorkspaceExecutionReceipt) codersdk.WorkspaceCommand {
	result := codersdk.WorkspaceCommand{ID: row.ID, SessionID: row.SessionID, RequestID: row.RequestID, WorkspaceID: row.WorkspaceID, AgentID: row.AgentID, AgentInstanceID: row.AgentInstanceID, ProcessID: row.ProcessID, State: row.State, Error: row.Error}
	if row.ExitCode.Valid {
		result.ExitCode = &row.ExitCode.Int32
	}
	if row.Deadline.Valid {
		result.Deadline = &row.Deadline.Time
	}
	return result
}
