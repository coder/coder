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
// @Param request body codersdk.UpdatePortSharingLevelRequest true "Update port sharing level request"
// @Success 200
// @Router /workspaces/{workspace}/port-sharing [post]
func (api *API) postWorkspacePortShareLevel(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	var req codersdk.UpdateWorkspaceAgentPortSharingLevelRequest
	if !httpapi.Read(ctx, rw, r, &req) {
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

		if (req.ShareLevel == codersdk.Shar)

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
