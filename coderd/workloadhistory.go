package coderd

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get workload host statistics (experimental)
// @ID get-workload-host-statistics-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param agent path string true "Workspace agent ID" format(uuid)
// @Success 200 {object} codersdk.WorkloadHostStats
// @Router /api/v2/organizations/{organization}/workload-hosts/{agent}/stats [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP GET handlers write responses rather than return values.
func (api *API) getWorkloadHostStats(rw http.ResponseWriter, r *http.Request) {
	host := api.authorizedWorkloadHostReport(rw, r)
	if host == nil {
		return
	}
	api.writeWorkloadHostStats(rw, r, host)
}

// @Summary Get workload host command history (experimental)
// @ID get-workload-host-command-history-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param agent path string true "Workspace agent ID" format(uuid)
// @Param offset query integer false "History offset"
// @Param limit query integer false "Page size, from 1 to 10 (default 10)"
// @Success 200 {object} codersdk.WorkloadHostHistory
// @Router /api/v2/organizations/{organization}/workload-hosts/{agent}/history [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP GET handlers write responses rather than return values.
func (api *API) getWorkloadHostHistory(rw http.ResponseWriter, r *http.Request) {
	host := api.authorizedWorkloadHostReport(rw, r)
	if host == nil {
		return
	}
	offset, limit := 0, 10
	var err error
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || offset < 0 {
		workloadError(rw, r, http.StatusBadRequest, "Offset must be a nonnegative history index.")
		return
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 10 {
		workloadError(rw, r, http.StatusBadRequest, "Limit must be between 1 and 10.")
		return
	}
	var history codersdk.WorkloadHostHistory
	path := "/history?offset=" + strconv.Itoa(offset) + "&limit=" + strconv.Itoa(limit)
	if _, err := api.workloadHostRequest(r.Context(), host, http.MethodGet, path, nil, &history); err != nil {
		workloadError(rw, r, http.StatusBadGateway, "Workload host history is unavailable.")
		return
	}
	for i := range history.Items {
		entry := &history.Items[i]
		// A supervisor may report only this host's organization and template.
		// Command history remains subject to the caller's workspace token scope.
		if entry.Request.OrganizationID != host.organizationID || entry.Request.TemplateID != host.templateID ||
			entry.Workload.OrganizationID != host.organizationID || entry.Workload.TemplateID != host.templateID ||
			entry.Workload.ID != entry.Request.ID || entry.Workload.OwnerID != entry.Request.OwnerID {
			workloadError(rw, r, http.StatusBadGateway, "Workload host returned invalid history.")
			return
		}
		if !api.Authorize(r, policy.ActionRead, rbac.ResourceWorkspace.InOrg(host.organizationID).WithOwner(entry.Request.OwnerID.String())) {
			httpapi.ResourceNotFound(rw)
			return
		}
		entry.Workload.AgentID = host.agentID
		// Like output responses, history must not expose raw supervisor errors.
		if entry.Workload.Failure != "" {
			entry.Workload.Failure = "Workload supervisor reported an execution or cleanup failure."
		}
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, history)
}

func (api *API) authorizedWorkloadHostReport(rw http.ResponseWriter, r *http.Request) *workloadHost {
	org := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionUpdate, org) {
		httpapi.ResourceNotFound(rw)
		return nil
	}
	id, err := uuid.Parse(chi.URLParam(r, "agent"))
	reg := &api.workloads
	reg.mu.Lock()
	host := reg.hosts[id]
	reg.mu.Unlock()
	if err != nil || host == nil || host.organizationID != org.ID {
		httpapi.ResourceNotFound(rw)
		return nil
	}
	//nolint:gocritic // Organization administration permits narrow host discovery, not bypassing workspace scopes.
	workspace, err := api.Database.GetWorkspaceByAgentID(dbauthz.AsSystemRestricted(r.Context()), id)
	if err != nil || workspace.OrganizationID != org.ID || workspace.Deleted || !api.Authorize(r, policy.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return nil
	}
	if api.workloadBrowserOnly(rw) {
		return nil
	}
	return host
}

// @Summary Get this agent's workload statistics (experimental)
// @ID get-this-agents-workload-statistics-experimental
// @Security CoderSessionToken
// @Tags Agents
// @Produce json
// @Success 200 {object} codersdk.WorkloadHostStats
// @Router /api/v2/workspaceagents/me/workload-stats [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP GET handlers write responses rather than return values.
func (api *API) getWorkspaceAgentWorkloadStats(rw http.ResponseWriter, r *http.Request) {
	host := api.workloadStatsHost(httpmw.WorkspaceAgent(r), httpmw.LatestBuild(r))
	if host == nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	api.writeWorkloadHostStats(rw, r, host)
}

func (api *API) workloadStatsHost(agent database.WorkspaceAgent, build database.WorkspaceBuild) *workloadHost {
	reg := &api.workloads
	reg.mu.Lock()
	defer reg.mu.Unlock()
	host := reg.hosts[agent.ID]
	if host == nil || host.buildID != build.ID {
		return nil
	}
	return host
}

func (api *API) writeWorkloadHostStats(rw http.ResponseWriter, r *http.Request, host *workloadHost) {
	var stats codersdk.WorkloadHostStats
	if _, err := api.workloadHostRequest(r.Context(), host, http.MethodGet, "/stats", nil, &stats); err != nil {
		workloadError(rw, r, http.StatusBadGateway, "Workload host statistics are unavailable.")
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, stats)
}
