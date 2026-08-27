package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// resolveAIAuditSponsor resolves the "sponsor" query parameter to a user ID.
// Absent, "me", or the caller's own ID/username resolve to the caller.
// Naming any other user requires site-wide audit log read (auditor/owner);
// unauthorized callers receive 403 whether or not the named user exists, so
// the parameter cannot be used to probe usernames.
func (api *API) resolveAIAuditSponsor(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	sponsor := r.URL.Query().Get("sponsor")
	if sponsor == "" || sponsor == codersdk.Me {
		return apiKey.UserID, true
	}

	var (
		user database.User
		err  error
	)
	//nolint:gocritic // The result is only revealed to the caller themselves or after an explicit audit-permission check below.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	if id, parseErr := uuid.Parse(sponsor); parseErr == nil {
		user, err = api.Database.GetUserByID(sysCtx, id)
	} else {
		user, err = api.Database.GetUserByEmailOrUsername(sysCtx, database.GetUserByEmailOrUsernameParams{
			Username: sponsor,
		})
	}
	if err == nil && user.ID == apiKey.UserID {
		return apiKey.UserID, true
	}

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceAuditLog) {
		httpapi.Forbidden(rw)
		return uuid.Nil, false
	}
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown sponsor user.",
			Detail:  "sponsor must be a user ID, username, or \"me\".",
		})
		return uuid.Nil, false
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to resolve sponsor user.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return user.ID, true
}

// @Summary List AI agent identities for a sponsor
// @ID list-ai-agent-identities-for-a-sponsor
// @Security CoderSessionToken
// @Produce json
// @Tags Audit
// @Param sponsor query string false "Sponsor user ID, username, or 'me' (default)"
// @Success 200 {array} codersdk.AIAuditAgent
// @Router /api/v2/ai-audit/agents [get]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) aiAuditAgents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sponsorID, ok := api.resolveAIAuditSponsor(rw, r)
	if !ok {
		return
	}

	//nolint:gocritic // Sponsor scoping is enforced by resolveAIAuditSponsor.
	rows, err := api.Database.GetAIAgentsByOwnerID(dbauthz.AsSystemRestricted(ctx), sponsorID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list AI agent identities.",
			Detail:  err.Error(),
		})
		return
	}

	agents := make([]codersdk.AIAuditAgent, 0, len(rows))
	for _, row := range rows {
		agents = append(agents, codersdk.AIAuditAgent{
			UserID:      row.AIAgent.UserID,
			Username:    row.Username,
			OwnerUserID: row.AIAgent.OwnerUserID,
			OriginType:  string(row.AIAgent.OriginType),
			OriginID:    row.AIAgent.OriginID,
			CreatedAt:   row.AIAgent.CreatedAt,
			Deleted:     row.AIAgent.Deleted,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, agents)
}
