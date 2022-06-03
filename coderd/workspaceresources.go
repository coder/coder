package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) workspaceResource(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspaceResource := httpmw.WorkspaceResourceParam(r)
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(rw, r, rbac.ActionRead, workspace) {
		return
	}

	job, err := api.Database.GetProvisionerJobByID(r.Context(), workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message:  "Internal error fetching provisioner job",
			Internal: err.Error(),
		})
		return
	}
	if !job.CompletedAt.Valid {
		httpapi.Write(rw, http.StatusPreconditionFailed, httpapi.Response{
			Message: "Job hasn't completed!",
		})
		return
	}
	agents, err := api.Database.GetWorkspaceAgentsByResourceIDs(r.Context(), []uuid.UUID{workspaceResource.ID})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message:  "Internal error fetching provisioner job agents",
			Internal: err.Error(),
		})
		return
	}
	apiAgents := make([]codersdk.WorkspaceAgent, 0)
	for _, agent := range agents {
		convertedAgent, err := convertWorkspaceAgent(agent, api.AgentConnectionUpdateFrequency)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message:  "Internal error reading workspace agent",
				Internal: err.Error(),
			})
			return
		}
		apiAgents = append(apiAgents, convertedAgent)
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspaceResource(workspaceResource, apiAgents))
}
