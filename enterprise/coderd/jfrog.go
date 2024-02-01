package coderd

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// Post workspace agent results for a JFrog XRay scan.
//
// @Summary Post JFrog XRay scan by workspace agent ID.
// @ID post-jfrog-xray-scan-by-workspace-agent-id
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.JFrogXrayScan true "Post JFrog XRay scan request"
// @Success 200 {object} codersdk.Response
// @Router /integrations/jfrog/xray-scan [post]
func (api *API) postJFrogXrayScan(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.JFrogXrayScan
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	err := api.Database.UpsertJFrogXrayScanByWorkspaceAndAgentID(ctx, database.UpsertJFrogXrayScanByWorkspaceAndAgentIDParams{
		WorkspaceID: req.WorkspaceID,
		AgentID:     req.AgentID,
		Critical:    int32(req.Critical),
		High:        int32(req.High),
		Medium:      int32(req.Medium),
		ResultsUrl:  req.ResultsURL,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.Response{
		Message: "Successfully inserted JFrog XRay scan!",
	})
}

// Get workspace agent results for a JFrog XRay scan.
//
// @Summary Get JFrog XRay scan by workspace agent ID.
// @ID get-jfrog-xray-scan-by-workspace-agent-id
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param workspace_id query string true "Workspace ID"
// @Param agent_id query string true "Agent ID"
// @Success 200 {object} codersdk.JFrogXrayScan
// @Router /integrations/jfrog/xray-scan [get]
func (api *API) jFrogXrayScan(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		vals    = r.URL.Query()
		p       = httpapi.NewQueryParamParser()
		wsID    = p.Required("workspace_id").UUID(vals, uuid.UUID{}, "workspace_id")
		agentID = p.Required("agent_id").UUID(vals, uuid.UUID{}, "agent_id")
	)

	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query params.",
			Validations: p.Errors,
		})
		return
	}

	scan, err := api.Database.GetJFrogXrayScanByWorkspaceAndAgentID(ctx, database.GetJFrogXrayScanByWorkspaceAndAgentIDParams{
		WorkspaceID: wsID,
		AgentID:     agentID,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.JFrogXrayScan{
		WorkspaceID: scan.WorkspaceID,
		AgentID:     scan.AgentID,
		Critical:    int(scan.Critical),
		High:        int(scan.High),
		Medium:      int(scan.Medium),
		ResultsURL:  scan.ResultsUrl,
	})
}

func (api *API) jfrogEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		api.entitlementsMu.RLock()
		// This doesn't actually use the external auth feature but we want
		// to lock this behind an enterprise license and it's somewhat
		// related to external auth (in that it is JFrog integration).
		enabled := api.entitlements.Features[codersdk.FeatureMultipleExternalAuth].Enabled
		api.entitlementsMu.RUnlock()

		if !enabled {
			httpapi.RouteNotFound(rw)
			return
		}

		next.ServeHTTP(rw, r)
	})
}
