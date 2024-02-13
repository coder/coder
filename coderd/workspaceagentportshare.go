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

// @Summary Upsert workspace agent port share
// @ID upsert-workspace-agent-port-share
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags PortSharing
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.UpsertWorkspaceAgentPortShareRequest true "Upsert port sharing level request"
// @Success 200 {object} codersdk.WorkspaceAgentPortShare
// @Router /workspaces/{workspace}/port-share [post]
func (api *API) postWorkspaceAgentPortShare(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	portSharer := *api.PortSharer.Load()
	var req codersdk.UpsertWorkspaceAgentPortShareRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if !req.ShareLevel.ValidPortShareLevel() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Port sharing level not allowed.",
		})
		return
	}

	template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	err = portSharer.AuthorizedPortSharingLevel(template, req.ShareLevel)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: err.Error(),
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

	psl, err := api.Database.UpsertWorkspaceAgentPortShare(ctx, database.UpsertWorkspaceAgentPortShareParams{
		WorkspaceID: workspace.ID,
		AgentName:   req.AgentName,
		Port:        req.Port,
		ShareLevel:  database.AppSharingLevel(req.ShareLevel),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertPortShare(psl))
}

// @Summary Get workspace agent port shares
// @ID get-workspace-agent-port-shares
// @Security CoderSessionToken
// @Produce json
// @Tags PortSharing
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceAgentPortShares
// @Router /workspaces/{workspace}/port-share [get]
func (api *API) workspaceAgentPortShares(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)

	shares, err := api.Database.ListWorkspaceAgentPortShares(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentPortShares{
		Shares: convertPortShares(shares),
	})
}

// @Summary Get workspace agent port shares
// @ID get-workspace-agent-port-shares
// @Security CoderSessionToken
// @Accept json
// @Tags PortSharing
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.DeleteWorkspaceAgentPortShareRequest true "Delete port sharing level request"
// @Success 200
// @Router /workspaces/{workspace}/port-share [delete]
func (api *API) deleteWorkspaceAgentPortShare(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	var req codersdk.DeleteWorkspaceAgentPortShareRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	_, err := api.Database.GetWorkspaceAgentPortShare(ctx, database.GetWorkspaceAgentPortShareParams{
		WorkspaceID: workspace.ID,
		AgentName:   req.AgentName,
		Port:        req.Port,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Port share not found.",
			})
			return
		}

		httpapi.InternalServerError(rw, err)
		return
	}

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
}

func convertPortShares(shares []database.WorkspaceAgentPortShare) []codersdk.WorkspaceAgentPortShare {
	var converted []codersdk.WorkspaceAgentPortShare
	for _, share := range shares {
		converted = append(converted, convertPortShare(share))
	}
	return converted
}

func convertPortShare(share database.WorkspaceAgentPortShare) codersdk.WorkspaceAgentPortShare {
	return codersdk.WorkspaceAgentPortShare{
		WorkspaceID: share.WorkspaceID,
		AgentName:   share.AgentName,
		Port:        share.Port,
		ShareLevel:  codersdk.WorkspaceAgentPortShareLevel(share.ShareLevel),
	}
}
