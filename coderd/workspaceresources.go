package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
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
			Message: fmt.Sprintf("get provisioner job: %s", err),
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
			Message: fmt.Sprintf("get provisioner job agents: %s", err),
		})
		return
	}
	agentIDs := make([]uuid.UUID, 0)
	for _, agent := range agents {
		agentIDs = append(agentIDs, agent.ID)
	}
	apps, err := api.Database.GetWorkspaceAppsByAgentIDs(r.Context(), agentIDs)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace apps: %s", err),
		})
		return
	}
	apiAgents := make([]codersdk.WorkspaceAgent, 0)
	for _, agent := range agents {
		dbApps := make([]database.WorkspaceApp, 0)
		for _, app := range apps {
			if app.AgentID == agent.ID {
				dbApps = append(dbApps, app)
			}
		}

		convertedAgent, err := convertWorkspaceAgent(agent, convertApps(dbApps), api.AgentConnectionUpdateFrequency)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("convert provisioner job agent: %s", err),
			})
			return
		}
		apiAgents = append(apiAgents, convertedAgent)
	}

	httpapi.Write(rw, http.StatusOK, convertWorkspaceResource(workspaceResource, apiAgents))
}
