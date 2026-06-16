package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
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
