package coderd

import (
	"database/sql"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get agent firewall session by ID
// @ID get-agent-firewall-session-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param id path string true "Agent firewall session ID" format(uuid)
// @Success 200 {object} codersdk.AgentFirewallSession
// @Router /api/v2/agent-firewall/sessions/{id} [get]
func (api *API) agentFirewallSessionByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := httpmw.ParseUUIDParam(rw, r, "id")
	if !ok {
		return
	}

	session, err := api.Database.GetBoundarySessionByID(ctx, id)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentFirewallSession{
		ID:              session.ID,
		WorkspaceID:     session.WorkspaceID,
		OwnerID:         session.WorkspaceOwnerID,
		ConfinedProcess: session.ConfinedProcessName,
		StartedAt:       session.StartedAt,
	})
}

// @Summary Get agent firewall session logs
// @ID get-agent-firewall-session-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param id path string true "Agent firewall session ID" format(uuid)
// @Param seq_after query int false "Inclusive lower bound on sequence number"
// @Param seq_before query int false "Exclusive upper bound on sequence number"
// @Param limit query int false "Maximum number of logs to return (default 100)"
// @Success 200 {object} codersdk.AgentFirewallSessionLogsResponse
// @Router /api/v2/agent-firewall/sessions/{id}/logs [get]
func (api *API) agentFirewallSessionLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceBoundaryLog) {
		httpapi.ResourceNotFound(rw)
		return
	}

	sessionID, ok := httpmw.ParseUUIDParam(rw, r, "id")
	if !ok {
		return
	}

	qp := r.URL.Query()
	p := httpapi.NewQueryParamParser()
	seqAfter := p.Int(qp, 0, "seq_after")
	seqBefore := p.Int(qp, 0, "seq_before")
	limitOpt := p.PositiveInt32(qp, 0, "limit")
	p.ErrorExcessParams(qp)
	if len(p.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: p.Errors,
		})
		return
	}

	params := database.ListBoundaryLogsBySessionIDParams{
		SessionID: sessionID,
		SeqAfter:  sql.NullInt32{},
		SeqBefore: sql.NullInt32{},
		LimitOpt:  limitOpt,
	}
	if qp.Has("seq_after") {
		params.SeqAfter = sql.NullInt32{Int32: int32(seqAfter), Valid: true} // #nosec G115 - Fits int32 for valid sequence numbers.
	}
	if qp.Has("seq_before") {
		params.SeqBefore = sql.NullInt32{Int32: int32(seqBefore), Valid: true} // #nosec G115 - Fits int32 for valid sequence numbers.
	}

	dbLogs, err := api.Database.ListBoundaryLogsBySessionID(ctx, params)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AgentFirewallSessionLogsResponse{
		Results: agentFirewallLogsFromDB(dbLogs),
	})
}

// agentFirewallLogsFromDB converts database boundary logs to SDK
// representation. Allowed is derived from MatchedRule being non-NULL.
func agentFirewallLogsFromDB(dbLogs []database.BoundaryLog) []codersdk.AgentFirewallLog {
	results := make([]codersdk.AgentFirewallLog, 0, len(dbLogs))
	for _, l := range dbLogs {
		bl := codersdk.AgentFirewallLog{
			ID:             l.ID,
			SessionID:      l.SessionID,
			SequenceNumber: l.SequenceNumber,
			Allowed:        l.MatchedRule.Valid,
			CreatedAt:      l.CreatedAt,
			Proto:          l.Proto,
			Method:         l.Method,
			Detail:         l.Detail,
			CapturedAt:     &l.CapturedAt,
		}
		if l.MatchedRule.Valid {
			bl.MatchedRule = &l.MatchedRule.String
		}
		results = append(results, bl)
	}
	return results
}
