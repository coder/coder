package coderd

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceartifacts"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) registerWorkspaceExecutionControlRoutes(r chi.Router) {
	r.Post("/workspace-executions/{session}/{action}", api.controlWorkspaceExecutionSession)
}

func writeWorkspaceExecutionControl(rw http.ResponseWriter, r *http.Request, session database.WorkspaceExecutionSession, err error) {
	if err != nil {
		switch {
		case errors.Is(err, workspaceexec.ErrSessionChanged), errors.Is(err, workspaceexec.ErrCleanupCommitted), errors.Is(err, workspaceexec.ErrAdmissionClosed), errors.Is(err, workspaceexec.ErrNotRetryable):
			httpapi.Write(r.Context(), rw, http.StatusConflict, codersdk.Response{Message: err.Error()})
		case errors.Is(err, workspaceexec.ErrInvalidRenewal), errors.Is(err, workspaceexec.ErrInvalidArtifactExpiry):
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: err.Error()})
		case httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
		default:
			httpapi.InternalServerError(rw, err)
		}
		return
	}
	result := codersdk.WorkspaceExecutionControlReceipt{ID: session.ID, State: session.State, Revision: session.Revision, Retained: session.Retained, LeaseExpiresAt: session.LeaseExpiresAt, Error: session.Error}
	expiry, err := workspaceartifacts.EffectiveExpiry(session)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if expiry.Valid {
		result.EffectiveArtifactExpiresAt = &expiry.Time
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, result)
}

func validWorkspaceExecutionRevision(rw http.ResponseWriter, r *http.Request, revision int64) bool {
	if revision > 0 {
		return true
	}
	httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "expected_revision must be explicitly positive."})
	return false
}

// @Summary Renew retain or retry a workspace execution session
// @ID renew-retain-or-retry-a-workspace-execution-session
// @Security CoderSessionToken
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Param action path string true "Lifecycle action" Enums(renew,retain,retry)
// @Param request body codersdk.WorkspaceExecutionControlRequest true "Observed revision and action-specific expiration"
// @Success 200 {object} codersdk.WorkspaceExecutionControlReceipt
// @Failure 400 {object} codersdk.Response
// @Failure 409 {object} codersdk.Response
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/{action} [post]
func (api *API) controlWorkspaceExecutionSession(rw http.ResponseWriter, r *http.Request) {
	session, ok := api.workspaceExecutionSession(rw, r, policy.ActionUpdate)
	if !ok {
		return
	}
	var req codersdk.WorkspaceExecutionControlRequest
	if !httpapi.Read(r.Context(), rw, r, &req) || !validWorkspaceExecutionRevision(rw, r, req.ExpectedRevision) {
		return
	}
	var result database.WorkspaceExecutionSession
	var err error
	switch chi.URLParam(r, "action") {
	case "renew":
		result, err = workspaceexec.Renew(r.Context(), api.Database, session.ID, req.ExpectedRevision, req.LeaseExpiresAt, api.Clock.Now())
	case "retain":
		result, err = workspaceexec.Retain(r.Context(), api.Database, session.ID, req.ExpectedRevision, api.Clock.Now())
	case "retry":
		result, err = workspaceexec.RetryWithArtifactExpiry(r.Context(), api.Database, session.ID, req.ExpectedRevision, req.ArtifactExpiresAt, api.Clock.Now())
	default:
		httpapi.ResourceNotFound(rw)
		return
	}
	writeWorkspaceExecutionControl(rw, r, result, err)
}
