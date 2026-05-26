package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) shouldBlockNonBrowserConnections(rw http.ResponseWriter) bool {
	if api.Entitlements.Enabled(codersdk.FeatureBrowserOnly) {
		httpapi.Write(context.Background(), rw, http.StatusConflict, codersdk.Response{
			Message: "Non-browser connections are disabled for your deployment.",
		})
		return true
	}
	return false
}

// @Summary Get workspace external agent credentials
// @ID get-workspace-external-agent-credentials
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param agent path string true "Agent name"
// @Success 200 {object} codersdk.ExternalAgentCredentials
// @Router /api/v2/workspaces/{workspace}/external-agent/{agent}/credentials [get]
func (api *API) workspaceExternalAgentCredentials(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	agentName := chi.URLParam(r, "agent")

	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get latest workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	if !build.HasExternalAgent.Bool {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Workspace does not have an external agent.",
		})
		return
	}

	agents, err := api.Database.GetWorkspaceAgentsByWorkspaceAndBuildNumber(ctx, database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
		WorkspaceID: workspace.ID,
		BuildNumber: build.BuildNumber,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get workspace agents.",
			Detail:  err.Error(),
		})
		return
	}

	var agent *database.WorkspaceAgent
	for i := range agents {
		if agents[i].Name == agentName {
			agent = &agents[i]
			break
		}
	}
	if agent == nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("External agent '%s' not found in workspace.", agentName),
		})
		return
	}

	if agent.AuthInstanceID.Valid {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "External agent is authenticated with an instance ID.",
		})
		return
	}

	initScriptURL := fmt.Sprintf("%s/api/v2/init-script/%s/%s", api.AccessURL.String(), agent.OperatingSystem, agent.Architecture)
	command := fmt.Sprintf("curl -fsSL %q | CODER_AGENT_TOKEN=%q sh", initScriptURL, agent.AuthToken.String())
	if agent.OperatingSystem == "windows" {
		command = fmt.Sprintf("$env:CODER_AGENT_TOKEN=%q; iwr -useb %q | iex", agent.AuthToken.String(), initScriptURL)
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ExternalAgentCredentials{
		AgentToken: agent.AuthToken.String(),
		Command:    command,
	})
}

// @Summary Get external agent tokens for multiple workspaces
// @ID get-external-agent-tokens-by-workspace-ids
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.ExternalAgentTokensByWorkspaceIDsRequest true "Workspace IDs"
// @Success 200 {object} codersdk.ExternalAgentTokensByWorkspaceIDsResponse
// @Router /api/v2/workspaces/external-agent/tokens [post]
func (api *API) workspaceExternalAgentTokensByWorkspaceIDs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.ExternalAgentTokensByWorkspaceIDsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if len(req.WorkspaceIDs) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "workspace_ids must not be empty.",
		})
		return
	}

	rows, err := api.Database.GetExternalAgentTokensByWorkspaceIDs(ctx, req.WorkspaceIDs)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get external agent tokens.",
			Detail:  err.Error(),
		})
		return
	}

	agents := make([]codersdk.ExternalAgentTokensByWorkspaceIDsRow, 0, len(rows))
	for _, row := range rows {
		agents = append(agents, codersdk.ExternalAgentTokensByWorkspaceIDsRow{
			WorkspaceID: row.WorkspaceID,
			AgentID:     row.AgentID,
			AgentName:   row.AgentName,
			AgentToken:  row.AgentToken.String(),
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ExternalAgentTokensByWorkspaceIDsResponse{
		Agents: agents,
	})
}
