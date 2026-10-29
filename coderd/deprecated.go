package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Removed: Get parameters by template version
// @ID removed-get-parameters-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200
// @Router /api/v2/templateversions/{templateversion}/parameters [get]
func templateVersionParametersDeprecated(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, []struct{}{})
}

// @Summary Removed: Get schema by template version
// @ID removed-get-schema-by-template-version
// @Security CoderSessionToken
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200
// @Router /api/v2/templateversions/{templateversion}/schema [get]
func templateVersionSchemaDeprecated(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, []struct{}{})
}

// @Summary Removed: Get logs by workspace agent
// @Description Returns the same response as `/api/v2/workspaceagents/{workspaceagent}/logs`, and remains only so older clients keep working.
// @Description New code uses `/api/v2/workspaceagents/{workspaceagent}/logs` instead.
// @ID removed-get-logs-by-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Param before query int false "Not used. The route ignores this parameter."
// @Param after query int false "ID of the last log entry you already have. Returns only entries with a greater ID, and the value must be an integer of `0` or more."
// @Param follow query bool false "When present, whatever its value, upgrades to a WebSocket that sends existing logs and then new logs as they arrive."
// @Param no_compression query bool false "When present, turns off WebSocket compression. It only applies with `follow`."
// @Success 200 {array} codersdk.WorkspaceAgentLog
// @Router /api/v2/workspaceagents/{workspaceagent}/startup-logs [get]
func (api *API) workspaceAgentLogsDeprecated(rw http.ResponseWriter, r *http.Request) {
	api.workspaceAgentLogs(rw, r)
}

// @Summary Removed: Get workspace agent git auth
// @Description Returns the same response as `/api/v2/workspaceagents/me/external-auth`, and remains only so legacy workspace agents keep working.
// @Description New code uses `/api/v2/workspaceagents/me/external-auth` instead.
// @Description Authenticate with the workspace agent token instead of a session token.
// @ID removed-get-workspace-agent-git-auth
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param match query string true "URL or hostname that selects the external auth provider by its configured regex. Pass `match` or `id`, not both."
// @Param id query string true "ID of the external auth provider, as configured on the deployment. Pass `id` or `match`, not both."
// @Param listen query bool false "When present, whatever its value, the request waits until the workspace owner has a valid token instead of returning the login URL right away."
// @Success 200 {object} agentsdk.ExternalAuthResponse
// @Router /api/v2/workspaceagents/me/gitauth [get]
func (api *API) workspaceAgentsGitAuth(rw http.ResponseWriter, r *http.Request) {
	api.workspaceAgentsExternalAuth(rw, r)
}

// @Summary Removed: Get workspace resources for workspace build
// @ID removed-get-workspace-resources-for-workspace-build
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID"
// @Success 200 {array} codersdk.WorkspaceResource
// @Router /api/v2/workspacebuilds/{workspacebuild}/resources [get]
// @Deprecated this endpoint is unused and will be removed in future.
func (api *API) workspaceBuildResourcesDeprecated(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceBuild := httpmw.WorkspaceBuildParam(r)

	job, err := api.Database.GetProvisionerJobByID(ctx, workspaceBuild.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	api.provisionerJobResources(rw, r, job)
}
