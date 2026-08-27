package coderd

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List MCP gateway escalations
// @ID list-mcp-gateway-escalations
// @Security CoderSessionToken
// @Produce json
// @Tags MCP Servers
// @Param status query string false "Escalation status" Enums(pending,approved,denied,expired)
// @Success 200 {array} codersdk.MCPGatewayEscalation
// @Router /api/v2/mcp-gateway/escalations [get]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) mcpGatewayEscalations(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	now := dbtime.Now()

	//nolint:gocritic // Sponsor scoping is enforced below after the system-wide expiry sweep.
	_, err := api.Database.ExpireMCPGatewayEscalations(dbauthz.AsSystemRestricted(ctx), sql.NullTime{
		Time:  now,
		Valid: true,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to expire MCP gateway escalations.",
			Detail:  err.Error(),
		})
		return
	}

	// The approvals surface is management plane: only the human sponsor may
	// list their escalations. An AI-held key is refused rather than silently
	// treated as its sponsor.
	sponsorUserID, ok := apiKey.UserID()
	if !ok {
		httpapi.Forbidden(rw)
		return
	}

	status := codersdk.MCPGatewayEscalationStatus(r.URL.Query().Get("status"))
	if !validMCPGatewayEscalationStatus(status, true) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid MCP gateway escalation status.",
			Detail:  "status must be pending, approved, denied, or expired.",
		})
		return
	}

	//nolint:gocritic // The system-guarded query is explicitly scoped to the authenticated sponsor.
	escalations, err := api.Database.ListMCPGatewayEscalationsBySponsor(dbauthz.AsSystemRestricted(ctx), database.ListMCPGatewayEscalationsBySponsorParams{
		SponsorUserID: sponsorUserID,
		Status:        string(status),
		// Zero values disable the timeline window, agent filter, and limit.
		AIAgentID:  uuid.Nil,
		AfterTime:  time.Time{},
		BeforeTime: time.Time{},
		Limit:      0,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list MCP gateway escalations.",
			Detail:  err.Error(),
		})
		return
	}

	response := make([]codersdk.MCPGatewayEscalation, 0, len(escalations))
	for _, escalation := range escalations {
		response = append(response, convertMCPGatewayEscalation(escalation))
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Approve an MCP gateway escalation
// @ID approve-mcp-gateway-escalation
// @Security CoderSessionToken
// @Produce json
// @Tags MCP Servers
// @Param id path string true "MCP gateway escalation ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /api/v2/mcp-gateway/escalations/{id}/approve [post]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) approveMCPGatewayEscalation(rw http.ResponseWriter, r *http.Request) {
	api.resolveMCPGatewayEscalation(rw, r, codersdk.MCPGatewayEscalationStatusApproved)
}

// @Summary Deny an MCP gateway escalation
// @ID deny-mcp-gateway-escalation
// @Security CoderSessionToken
// @Produce json
// @Tags MCP Servers
// @Param id path string true "MCP gateway escalation ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /api/v2/mcp-gateway/escalations/{id}/deny [post]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) denyMCPGatewayEscalation(rw http.ResponseWriter, r *http.Request) {
	api.resolveMCPGatewayEscalation(rw, r, codersdk.MCPGatewayEscalationStatusDenied)
}

func (api *API) resolveMCPGatewayEscalation(rw http.ResponseWriter, r *http.Request, status codersdk.MCPGatewayEscalationStatus) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.MCPGatewayEscalation](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	apiKey := httpmw.APIKey(r)
	// Only the human sponsor may resolve an escalation. An AI-held key is
	// refused before any lookup so it cannot probe escalation IDs.
	sponsorUserID, isUser := apiKey.UserID()
	if !isUser {
		httpapi.Forbidden(rw)
		return
	}
	escalationID, ok := httpmw.ParseUUIDParam(rw, r, "id")
	if !ok {
		return
	}

	//nolint:gocritic // Sponsor ownership is checked immediately after this system-guarded lookup.
	escalation, err := api.Database.GetMCPGatewayEscalationByID(dbauthz.AsSystemRestricted(ctx), escalationID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && escalation.SponsorUserID != sponsorUserID) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP gateway escalation.",
			Detail:  err.Error(),
		})
		return
	}
	if escalation.Status != string(codersdk.MCPGatewayEscalationStatusPending) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "MCP gateway escalation is no longer pending.",
		})
		return
	}
	aReq.Old = escalation

	now := dbtime.Now()
	//nolint:gocritic // Sponsor ownership was verified above before this guarded update.
	updated, err := api.Database.ResolveMCPGatewayEscalation(dbauthz.AsSystemRestricted(ctx), database.ResolveMCPGatewayEscalationParams{
		ID:         escalation.ID,
		Status:     string(status),
		ResolvedAt: sql.NullTime{Time: now, Valid: true},
		ResolvedBy: uuid.NullUUID{UUID: sponsorUserID, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "MCP gateway escalation is no longer pending.",
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to resolve MCP gateway escalation.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = updated

	message := "MCP gateway escalation approved."
	if status == codersdk.MCPGatewayEscalationStatusDenied {
		message = "MCP gateway escalation denied."
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: message})
}

func validMCPGatewayEscalationStatus(status codersdk.MCPGatewayEscalationStatus, allowEmpty bool) bool {
	switch status {
	case "":
		return allowEmpty
	case codersdk.MCPGatewayEscalationStatusPending,
		codersdk.MCPGatewayEscalationStatusApproved,
		codersdk.MCPGatewayEscalationStatusDenied,
		codersdk.MCPGatewayEscalationStatusExpired:
		return true
	default:
		return false
	}
}

func convertMCPGatewayEscalation(escalation database.MCPGatewayEscalation) codersdk.MCPGatewayEscalation {
	return codersdk.MCPGatewayEscalation{
		ID:            escalation.ID,
		ServerSlug:    escalation.ServerSlug,
		Tool:          escalation.Tool,
		Input:         string(escalation.Input),
		WorkspaceName: escalation.WorkspaceName,
		Status:        codersdk.MCPGatewayEscalationStatus(escalation.Status),
		CreatedAt:     escalation.CreatedAt,
		ExpiresAt:     escalation.ExpiresAt,
	}
}
