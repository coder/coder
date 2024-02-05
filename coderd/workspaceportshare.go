package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Update workspace agent port share
// @ID update-workspace-agent-port-share
// @Security CoderSessionToken
// @Accept json
// @Tags PortSharing
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpdateWorkspaceAgentPortShareRequest true "Update port sharing level request"
// @Success 200
// @Router /workspaces/{workspace}/port-share [post]
func (api *API) postWorkspaceAgentPortShare(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	portSharer := *api.PortSharer.Load()
	var req codersdk.UpdateWorkspaceAgentPortShareRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.ShareLevel < 0 || req.ShareLevel > 2 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Port sharing level not allowed. Must be between 0 and 2.",
		})
		return
	}

	if portSharer.CanRestrictSharing() {
		template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		if req.ShareLevel > codersdk.WorkspaceAgentPortShareLevel(template.MaxPortShareLevel) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Port sharing level not allowed. Must not be greater than %d.", template.MaxPortShareLevel),
			})
			return
		}
	}

	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	found := false
	for _, agent := range agents {
		if agent.Name == req.AgentName {
			found = true
			break
		}
	}
	if !found {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Agent not found.",
		})
		return
	}

	psl, err := api.Database.GetWorkspaceAgentPortShare(ctx, database.GetWorkspaceAgentPortShareParams{
		WorkspaceID: workspace.ID,
		AgentName:   req.AgentName,
		Port:        req.Port,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}

		if req.ShareLevel == codersdk.WorkspaceAgentPortShareLevelOwner {
			// If the port is not shared, and the user is trying to set it to owner,
			// we don't need to do anything.
			rw.WriteHeader(http.StatusOK)
			return
		}

		err = api.Database.CreateWorkspaceAgentPortShare(ctx, database.CreateWorkspaceAgentPortShareParams{
			WorkspaceID: workspace.ID,
			AgentName:   req.AgentName,
			Port:        req.Port,
			ShareLevel:  int32(req.ShareLevel),
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		rw.WriteHeader(http.StatusOK)
		return
	}

	if codersdk.WorkspaceAgentPortShareLevel(psl.ShareLevel) == codersdk.WorkspaceAgentPortShareLevelOwner {
		// If the port is shared, and the user is trying to set it to owner,
		// we need to remove the existing share record.
		err = api.Database.DeleteWorkspaceAgentPortShare(ctx, database.DeleteWorkspaceAgentPortShareParams{
			WorkspaceID: workspace.ID,
			AgentName:   req.AgentName,
			Port:        req.Port,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		rw.WriteHeader(http.StatusOK)
		return
	}

	err = api.Database.UpdateWorkspaceAgentPortShare(ctx, database.UpdateWorkspaceAgentPortShareParams{
		WorkspaceID: psl.WorkspaceID,
		AgentName:   psl.AgentName,
		Port:        psl.Port,
		ShareLevel:  int32(req.ShareLevel),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusOK)
}
