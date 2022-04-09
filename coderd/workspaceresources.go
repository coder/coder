package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/google/uuid"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *api) workspaceResource(rw http.ResponseWriter, r *http.Request) {
	workspaceBuild := httpmw.WorkspaceBuildParam(r)
	workspaceResource := httpmw.WorkspaceResourceParam(r)
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
	apiAgents := make([]codersdk.WorkspaceAgent, 0)
	for _, agent := range agents {
		convertedAgent, err := convertWorkspaceAgent(agent, api.AgentConnectionUpdateFrequency)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("convert provisioner job agent: %s", err),
			})
			return
		}
		apiAgents = append(apiAgents, convertedAgent)
	}

	render.Status(r, http.StatusOK)
	render.JSON(rw, r, convertWorkspaceResource(workspaceResource, apiAgents))
}
