package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Update port sharing level
// @ID post-workspace-port-sharing-level
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags PortSharing
// @Param request body codersdk.UpdateWorkspaceAgentPortSharingLevelRequest true "Update port sharing level request"
// @Success 200
// @Router /workspaces/{workspace}/port-sharing [post]
func (api *API) postWorkspacePortShareLevel(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	portSharer := *api.PortSharer.Load()
	var req codersdk.UpdateWorkspaceAgentPortSharingLevelRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	shareLevelAllowed := portSharer.ShareLevelAllowed(workspace.ID, codersdk.WorkspacePortSharingLevel(req.ShareLevel))
	if !shareLevelAllowed {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Port sharing level not allowed.",
		})
		return
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

	psl, err := api.Database.GetWorkspacePortShareLevel(ctx, database.GetWorkspacePortShareLevelParams{
		WorkspaceID: workspace.ID,
		AgentName:   req.AgentName,
		Port:        int32(req.Port),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}

		if req.ShareLevel == int(codersdk.WorkspaceAgentPortSharingLevelOwner) {
			// If the port is not shared, and the user is trying to set it to owner,
			// we don't need to do anything.
			rw.WriteHeader(http.StatusOK)
			return
		}

		err = api.Database.CreateWorkspacePortShareLevel(ctx, database.CreateWorkspacePortShareLevelParams{
			WorkspaceID: workspace.ID,
			AgentName:   req.AgentName,
			Port:        int32(req.Port),
			ShareLevel:  int32(req.ShareLevel),
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		rw.WriteHeader(http.StatusOK)
		return
	}

	if codersdk.WorkspacePortSharingLevel(psl.ShareLevel) == codersdk.WorkspaceAgentPortSharingLevelOwner {
		// If the port is shared, and the user is trying to set it to owner,
		// we need to remove the existing share record.
		err = api.Database.DeleteWorkspacePortShareLevel(ctx, database.DeleteWorkspacePortShareLevelParams{
			WorkspaceID: workspace.ID,
			AgentName:   req.AgentName,
			Port:        int32(req.Port),
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		rw.WriteHeader(http.StatusOK)
		return
	}

	err = api.Database.UpdateWorkspacePortShareLevel(ctx, database.UpdateWorkspacePortShareLevelParams{
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
