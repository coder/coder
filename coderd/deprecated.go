package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// @Summary Removed: Get parameters by template version
// @ID removed-get-parameters-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200
// @Router /templateversions/{templateversion}/parameters [get]
func templateVersionParametersDeprecated(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, []struct{}{})
}

// @Summary Removed: Get schema by template version
// @ID removed-get-schema-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200
// @Router /templateversions/{templateversion}/schema [get]
func templateVersionSchemaDeprecated(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, []struct{}{})
}

// @Summary Removed: Patch workspace agent logs
// @ID removed-patch-workspace-agent-logs
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PatchLogs true "logs"
// @Success 200 {object} codersdk.Response
// @Router /workspaceagents/me/startup-logs [patch]
func (api *API) patchWorkspaceAgentLogsDeprecated(rw http.ResponseWriter, r *http.Request) {
	api.patchWorkspaceAgentLogs(rw, r)
}

// @Summary Removed: Get logs by workspace agent
// @ID removed-get-logs-by-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Param before query int false "Before log id"
// @Param after query int false "After log id"
// @Param follow query bool false "Follow log stream"
// @Param no_compression query bool false "Disable compression for WebSocket connection"
// @Success 200 {array} codersdk.WorkspaceAgentLog
// @Router /workspaceagents/{workspaceagent}/startup-logs [get]
func (api *API) workspaceAgentLogsDeprecated(rw http.ResponseWriter, r *http.Request) {
	api.workspaceAgentLogs(rw, r)
}

// @Summary Removed: Get workspace agent git auth
// @ID removed-get-workspace-agent-git-auth
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param match query string true "Match"
// @Param id query string true "Provider ID"
// @Param listen query bool false "Wait for a new token to be issued"
// @Success 200 {object} agentsdk.ExternalAuthResponse
// @Router /workspaceagents/me/gitauth [get]
func (api *API) workspaceAgentsGitAuth(rw http.ResponseWriter, r *http.Request) {
	api.workspaceAgentsExternalAuth(rw, r)
}

// @Summary Removed: Submit workspace agent metadata
// @ID removed-submit-workspace-agent-metadata
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param request body agentsdk.PostMetadataRequestDeprecated true "Workspace agent metadata request"
// @Param key path string true "metadata key" format(string)
// @Success 204 "Success"
// @Router /workspaceagents/me/metadata/{key} [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentPostMetadataDeprecated(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req agentsdk.PostMetadataRequestDeprecated
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	workspaceAgent := httpmw.WorkspaceAgent(r)

	key := chi.URLParam(r, "key")

	err := api.workspaceAgentUpdateMetadata(ctx, workspaceAgent, agentsdk.PostMetadataRequest{
		Metadata: []agentsdk.Metadata{
			{
				Key:                          key,
				WorkspaceAgentMetadataResult: req,
			},
		},
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to handle metadata request", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}
