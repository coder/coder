package coderd

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Export declared workspace execution results
// @ID export-declared-workspace-execution-results
// @Security CoderSessionToken
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Param request body codersdk.ExportWorkspaceExecutionRequest true "Observed open session revision"
// @Success 200 {array} codersdk.WorkspaceExecutionArtifact
// @Failure 409 {object} codersdk.Response
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/export [post]
func (api *API) exportWorkspaceExecution(rw http.ResponseWriter, r *http.Request) {
	session, ok := api.workspaceExecutionSession(rw, r, policy.ActionUpdate)
	if !ok {
		return
	}
	if _, ok := api.workspaceExecutionSession(rw, r, policy.ActionSSH); !ok {
		return
	}
	if api.Entitlements.Enabled(codersdk.FeatureBrowserOnly) {
		httpapi.Forbidden(rw)
		return
	}
	if session.State != "preserved" && session.State != "completed" {
		workspace, err := api.Database.GetWorkspaceByID(r.Context(), session.WorkspaceID.UUID)
		if err != nil {
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
			} else {
				httpapi.InternalServerError(rw, err)
			}
			return
		}
		if !api.Authorize(r, policy.ActionSSH, workspace) {
			httpapi.ResourceNotFound(rw)
			return
		}
	}
	var request codersdk.ExportWorkspaceExecutionRequest
	if !httpapi.Read(r.Context(), rw, r, &request) || !validWorkspaceExecutionRevision(rw, r, request.ExpectedRevision) {
		return
	}
	// Already admitted preservation survives a disappearing HTTP client.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
	defer cancel()
	artifacts, err := api.workspaceExecutionController.Export(ctx, session.ID, request.ExpectedRevision)
	if err != nil {
		if errors.Is(err, workspaceexec.ErrSessionChanged) || errors.Is(err, workspaceexec.ErrAdmissionClosed) || errors.Is(err, workspaceexec.ErrCleanupCommitted) {
			httpapi.Write(r.Context(), rw, http.StatusConflict, codersdk.Response{Message: err.Error()})
		} else {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{Message: "Result preservation failed; automatic cleanup is blocked.", Detail: err.Error()})
		}
		return
	}
	result := make([]codersdk.WorkspaceExecutionArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		result = append(result, convertWorkspaceExecutionArtifact(artifact))
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, result)
}
