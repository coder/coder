package coderd

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// Post workspace agent results for a JFrog XRay scan.
//
// @Summary Post JFrog XRay scan by workspace agent ID.
// @ID post-jfrog-xray-scan-by-workspace-agent-id
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.JFrogXrayScan true "Post JFrog XRay scan request"
// @Success 200 {object} codersdk.Response
// @Router /exp/jfrog/xray-scan [post]
func (api *API) postJFrogXrayScan(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req codersdk.JFrogXrayScan
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	payload, err := json.Marshal(req)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	err = api.Database.UpsertJFrogXrayScanByWorkspaceAndAgentID(ctx, database.UpsertJFrogXrayScanByWorkspaceAndAgentIDParams{
		WorkspaceID: req.WorkspaceID,
		AgentID:     req.AgentID,
		Payload:     payload,
	})
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
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
// @Router /exp/jfrog/xray-scan [post]
func (api *API) jFrogXrayScan(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		wid = r.URL.Query().Get("workspace_id")
		aid = r.URL.Query().Get("agent_id")
	)

	wsID, err := uuid.Parse(wid)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "'workspace_id' must be a valid UUID.",
		})
		return
	}

	agentID, err := uuid.Parse(aid)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "'agent_id' must be a valid UUID.",
		})
		return
	}

	scan, err := api.Database.GetJFrogXrayScanByWorkspaceAndAgentID(ctx, database.GetJFrogXrayScanByWorkspaceAndAgentIDParams{
		WorkspaceID: wsID,
		AgentID:     agentID,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}
	if scan.Payload == nil {
		scan.Payload = []byte("{}")
	}

	httpapi.Write(ctx, rw, http.StatusOK, scan.Payload)
}

func (api *API) jfrogEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		api.entitlementsMu.RLock()
		enabled := api.entitlements.Features[codersdk.FeatureMultipleExternalAuth].Enabled
		api.entitlementsMu.RUnlock()

		if !enabled {
			httpapi.RouteNotFound(rw)
			return
		}

		next.ServeHTTP(rw, r)
	})
}
