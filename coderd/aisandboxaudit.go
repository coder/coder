package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const maxAISandboxNetworkEvents = 1000

// @Summary Report an AI sandbox session
// @ID report-ai-sandbox-session
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PostAISandboxSessionRequest true "AI sandbox session"
// @Success 200 {object} codersdk.Response
// @Router /api/v2/workspaceagents/me/ai-sandbox-sessions [post]
func (api *API) postAISandboxSession(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reporter := httpmw.WorkspaceAgent(r)

	var req agentsdk.PostAISandboxSessionRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := validateAISandboxSession(req); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI sandbox session.",
			Validations: validations,
		})
		return
	}

	// The agent subject cannot read arbitrary retained sessions. Ownership is
	// checked against the authenticated reporter before a row is reused.
	//nolint:gocritic
	existing, err := api.Database.GetAISandboxSessionByID(dbauthz.AsSystemRestricted(ctx), req.ID)
	if err == nil {
		if existing.ReporterAgentID != reporter.ID {
			httpapi.ResourceNotFound(rw)
			return
		}
		err = api.upsertAISandboxSession(ctx, database.UpsertAISandboxSessionParams{
			ID:                existing.ID,
			WorkspaceID:       existing.WorkspaceID,
			ReporterAgentID:   existing.ReporterAgentID,
			ConfinedAgentID:   existing.ConfinedAgentID,
			AIAgentID:         existing.AIAgentID,
			SponsorUserID:     existing.SponsorUserID,
			EgressEnforcement: existing.EgressEnforcement,
			StartedAt:         existing.StartedAt,
			EndedAt:           nullableAISandboxEndedAt(req.EndedAt),
			CreatedAt:         existing.CreatedAt,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "AI sandbox session reported."})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox session: %w", err))
		return
	}

	workspace, err := api.workspaceAgentWorkspace(ctx, r)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	confinedAgentID := reporter.ID
	var aiAgentID, sponsorUserID uuid.UUID
	if req.ChildAgentID == uuid.Nil {
		actor, ok := aiagentidentity.ActorFromContext(ctx)
		if !ok {
			httpapi.Forbidden(rw)
			return
		}
		aiAgentID = actor.AgentUserID
		sponsorUserID = actor.OwnerUserID
	} else {
		// The agent subject cannot read arbitrary agent rows. Parent ownership is
		// verified below before any child attributes are used.
		//nolint:gocritic
		child, childErr := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), req.ChildAgentID)
		if childErr != nil {
			if errors.Is(childErr, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.InternalServerError(rw, xerrors.Errorf("get confined child workspace agent: %w", childErr))
			return
		}
		if !child.ParentID.Valid || child.ParentID.UUID != reporter.ID {
			httpapi.ResourceNotFound(rw)
			return
		}
		if !child.AIAgentID.Valid {
			httpapi.Forbidden(rw)
			return
		}
		identity, resolveErr := aiagentidentity.Resolve(ctx, api.Database, child.AIAgentID.UUID)
		if resolveErr != nil {
			httpapi.Forbidden(rw)
			return
		}
		confinedAgentID = child.ID
		aiAgentID = identity.Actor.AgentUserID
		sponsorUserID = identity.Actor.OwnerUserID
	}

	err = api.upsertAISandboxSession(ctx, database.UpsertAISandboxSessionParams{
		ID:                req.ID,
		WorkspaceID:       workspace.ID,
		ReporterAgentID:   reporter.ID,
		ConfinedAgentID:   confinedAgentID,
		AIAgentID:         aiAgentID,
		SponsorUserID:     sponsorUserID,
		EgressEnforcement: string(req.EgressEnforcement),
		StartedAt:         req.StartedAt,
		EndedAt:           nullableAISandboxEndedAt(req.EndedAt),
		CreatedAt:         dbtime.Now(),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "AI sandbox session reported."})
}

func (api *API) upsertAISandboxSession(ctx context.Context, params database.UpsertAISandboxSessionParams) error {
	// Session attribution is derived from the authenticated reporter or a
	// previously verified retained row, so the internal write cannot be
	// performed with the restricted agent subject.
	//nolint:gocritic
	_, err := api.Database.UpsertAISandboxSession(dbauthz.AsSystemRestricted(ctx), params)
	if err != nil {
		return xerrors.Errorf("upsert AI sandbox session: %w", err)
	}
	return nil
}

// @Summary Report AI sandbox network events
// @ID report-ai-sandbox-network-events
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PatchAISandboxNetworkEventsRequest true "AI sandbox network events"
// @Success 200 {object} codersdk.Response
// @Router /api/v2/workspaceagents/me/ai-sandbox-network-events [patch]
func (api *API) patchAISandboxNetworkEvents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reporter := httpmw.WorkspaceAgent(r)

	var req agentsdk.PatchAISandboxNetworkEventsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := validateAISandboxNetworkEvents(req.Events); len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI sandbox network events.",
			Validations: validations,
		})
		return
	}

	eventsBySession := make(map[uuid.UUID][]agentsdk.AISandboxNetworkEvent)
	for _, event := range req.Events {
		eventsBySession[event.SessionID] = append(eventsBySession[event.SessionID], event)
	}

	sessions := make(map[uuid.UUID]database.AISandboxSession, len(eventsBySession))
	for sessionID := range eventsBySession {
		// The agent subject cannot read retained sessions. Reporter ownership is
		// checked before any event in the request is inserted.
		//nolint:gocritic
		session, err := api.Database.GetAISandboxSessionByID(dbauthz.AsSystemRestricted(ctx), sessionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox session: %w", err))
			return
		}
		if session.ReporterAgentID != reporter.ID {
			httpapi.ResourceNotFound(rw)
			return
		}
		sessions[sessionID] = session
	}

	err := api.Database.InTx(func(tx database.Store) error {
		for sessionID, events := range eventsBySession {
			session := sessions[sessionID]
			params := database.InsertAISandboxNetworkEventsParams{
				SessionID:      make([]uuid.UUID, 0, len(events)),
				OccurredAt:     make([]time.Time, 0, len(events)),
				Protocol:       make([]string, 0, len(events)),
				Host:           make([]string, 0, len(events)),
				Port:           make([]int32, 0, len(events)),
				Action:         make([]string, 0, len(events)),
				PolicyRevision: make([]int64, 0, len(events)),
				AIAgentID:      make([]uuid.UUID, 0, len(events)),
				SponsorUserID:  make([]uuid.UUID, 0, len(events)),
				CreatedAt:      make([]time.Time, 0, len(events)),
			}
			createdAt := dbtime.Now()
			for _, event := range events {
				params.SessionID = append(params.SessionID, sessionID)
				params.OccurredAt = append(params.OccurredAt, event.OccurredAt)
				params.Protocol = append(params.Protocol, string(event.Protocol))
				params.Host = append(params.Host, event.Host)
				// #nosec G115 - Validation restricts ports to the uint16 range.
				params.Port = append(params.Port, int32(event.Port))
				params.Action = append(params.Action, string(event.Action))
				params.PolicyRevision = append(params.PolicyRevision, event.PolicyRevision)
				params.AIAgentID = append(params.AIAgentID, session.AIAgentID)
				params.SponsorUserID = append(params.SponsorUserID, session.SponsorUserID)
				params.CreatedAt = append(params.CreatedAt, createdAt)
			}

			// Event attribution comes from the verified retained session, so the
			// internal write cannot be performed with the restricted agent subject.
			//nolint:gocritic
			if _, err := tx.InsertAISandboxNetworkEvents(dbauthz.AsSystemRestricted(ctx), params); err != nil {
				return xerrors.Errorf("insert AI sandbox network events: %w", err)
			}
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "AI sandbox network events reported."})
}

func validateAISandboxSession(req agentsdk.PostAISandboxSessionRequest) []codersdk.ValidationError {
	validations := make([]codersdk.ValidationError, 0)
	if req.ID == uuid.Nil {
		validations = append(validations, codersdk.ValidationError{Field: "id", Detail: "Session ID must be non-nil."})
	}
	switch req.EgressEnforcement {
	case codersdk.AISandboxEgressEnforcementForced,
		codersdk.AISandboxEgressEnforcementAdvisory,
		codersdk.AISandboxEgressEnforcementNone:
	default:
		validations = append(validations, codersdk.ValidationError{Field: "egress_enforcement", Detail: "Egress enforcement must be forced, advisory, or none."})
	}
	if req.StartedAt.IsZero() {
		validations = append(validations, codersdk.ValidationError{Field: "started_at", Detail: "Started at must be non-zero."})
	}
	return validations
}

func validateAISandboxNetworkEvents(events []agentsdk.AISandboxNetworkEvent) []codersdk.ValidationError {
	validations := make([]codersdk.ValidationError, 0)
	if len(events) > maxAISandboxNetworkEvents {
		validations = append(validations, codersdk.ValidationError{
			Field:  "events",
			Detail: fmt.Sprintf("Must contain no more than %d events.", maxAISandboxNetworkEvents),
		})
	}
	for i, event := range events {
		field := func(name string) string { return fmt.Sprintf("events[%d].%s", i, name) }
		if event.SessionID == uuid.Nil {
			validations = append(validations, codersdk.ValidationError{Field: field("session_id"), Detail: "Session ID must be non-nil."})
		}
		if event.OccurredAt.IsZero() {
			validations = append(validations, codersdk.ValidationError{Field: field("occurred_at"), Detail: "Occurred at must be non-zero."})
		}
		switch event.Protocol {
		case agentsdk.AISandboxNetworkProtocolConnect,
			agentsdk.AISandboxNetworkProtocolHTTP,
			agentsdk.AISandboxNetworkProtocolSNI,
			agentsdk.AISandboxNetworkProtocolTCP:
		default:
			validations = append(validations, codersdk.ValidationError{Field: field("protocol"), Detail: "Protocol must be connect, http, sni, or tcp."})
		}
		if len(event.Host) > 253 {
			validations = append(validations, codersdk.ValidationError{Field: field("host"), Detail: "Host must be no more than 253 characters."})
		}
		if event.Port < 0 || event.Port > 65535 {
			validations = append(validations, codersdk.ValidationError{Field: field("port"), Detail: "Port must be between 0 and 65535."})
		}
		switch event.Action {
		case agentsdk.AISandboxNetworkEventActionAllowed, agentsdk.AISandboxNetworkEventActionDenied:
		default:
			validations = append(validations, codersdk.ValidationError{Field: field("action"), Detail: "Action must be allowed or denied."})
		}
	}
	return validations
}

func nullableAISandboxEndedAt(endedAt *time.Time) sql.NullTime {
	if endedAt == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *endedAt, Valid: true}
}
