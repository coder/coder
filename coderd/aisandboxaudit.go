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

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

const (
	maxAISandboxNetworkEvents         = 1000
	maxAISandboxNetworkEventsPageSize = 100
)

// @Summary List AI sandbox sessions
// @ID list-ai-sandbox-sessions
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {array} codersdk.AISandboxSession
// @Router /api/v2/workspaces/{workspace}/ai-sandbox-sessions [get]
func (api *API) workspaceAISandboxSessions(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)

	sessions, err := api.Database.GetAISandboxSessionsByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox sessions: %w", err))
		return
	}

	response := make([]codersdk.AISandboxSession, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, convertAISandboxSession(session))
	}
	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary List AI sandbox session network events
// @ID list-ai-sandbox-session-network-events
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param session path string true "AI sandbox session ID" format(uuid)
// @Param before_id query int false "Return events with database ID less than before_id"
// @Param limit query int false "Page size, 1 to 100. Defaults to 100."
// @Success 200 {array} codersdk.AISandboxNetworkEventView
// @Router /api/v2/workspaces/{workspace}/ai-sandbox-sessions/{session}/network-events [get]
func (api *API) workspaceAISandboxSessionNetworkEvents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	sessionID, ok := httpmw.ParseUUIDParam(rw, r, "session")
	if !ok {
		return
	}

	parser := httpapi.NewQueryParamParser()
	beforeID := parser.PositiveInt64(r.URL.Query(), 0, "before_id")
	limit := parser.PositiveInt32(r.URL.Query(), maxAISandboxNetworkEventsPageSize, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if limit < 1 || limit > maxAISandboxNetworkEventsPageSize {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid limit parameter (1-%d).", maxAISandboxNetworkEventsPageSize),
		})
		return
	}

	// Retained sessions are read internally so a session belonging to another
	// workspace can be hidden before the workspace-authorized event query runs.
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
	if session.WorkspaceID != workspace.ID {
		httpapi.ResourceNotFound(rw)
		return
	}

	events, err := api.Database.GetAISandboxNetworkEventsBySessionIDPaged(ctx, database.GetAISandboxNetworkEventsBySessionIDPagedParams{
		SessionID:   sessionID,
		BeforeID:    beforeID,
		WorkspaceID: workspace.ID,
		LimitCount:  limit,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get AI sandbox network events: %w", err))
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertAISandboxNetworkEvents(events))
}

// @Summary List workspace AI sandbox network events
// @ID list-workspace-ai-sandbox-network-events
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param before_id query int false "Return events older than the event with before_id"
// @Param limit query int false "Page size, 1 to 100. Defaults to 100."
// @Success 200 {array} codersdk.AISandboxNetworkEventView
// @Router /api/v2/workspaces/{workspace}/ai-sandbox-activity [get]
func (api *API) workspaceAISandboxNetworkEvents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	parser := httpapi.NewQueryParamParser()
	beforeID := parser.PositiveInt64(r.URL.Query(), 0, "before_id")
	limit := parser.PositiveInt32(r.URL.Query(), maxAISandboxNetworkEventsPageSize, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if limit < 1 || limit > maxAISandboxNetworkEventsPageSize {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid limit parameter (1-%d).", maxAISandboxNetworkEventsPageSize),
		})
		return
	}

	events, err := api.Database.GetAISandboxNetworkEventsByWorkspaceIDPaged(ctx, database.GetAISandboxNetworkEventsByWorkspaceIDPagedParams{
		WorkspaceID: workspace.ID,
		BeforeID:    beforeID,
		LimitCount:  limit,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get workspace AI sandbox network events: %w", err))
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, convertAISandboxNetworkEvents(events))
}

// @Summary Watch AI sandbox activity
// @ID watch-workspace-ai-sandbox-activity
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 101
// @Router /api/v2/workspaces/{workspace}/ai-sandbox-activity/watch [get]
func (api *API) watchWorkspaceAISandboxActivity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	updates := make(chan struct{}, 1)

	cancelSubscribe, err := api.Pubsub.SubscribeWithErr(workspaceAISandboxActivityChannel(workspace.ID), func(callbackCtx context.Context, _ []byte, err error) {
		if err != nil {
			api.Logger.Warn(callbackCtx, "ai sandbox activity update delivered with error",
				slog.F("workspace_id", workspace.ID), slog.Error(err))
		}
		select {
		case updates <- struct{}{}:
		default:
		}
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("subscribe to AI sandbox activity updates: %w", err))
		return
	}
	defer cancelSubscribe()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	_ = conn.CloseRead(context.Background())

	ctx = api.wsWatcher.Watch(ctx, api.Logger, conn)
	enc := wsjson.NewEncoder[struct{}](conn, websocket.MessageText)
	for {
		select {
		case <-ctx.Done():
			return
		case <-updates:
			if err := enc.Encode(struct{}{}); err != nil {
				return
			}
		}
	}
}

func convertAISandboxSession(session database.AISandboxSession) codersdk.AISandboxSession {
	var endedAt *time.Time
	if session.EndedAt.Valid {
		endedAt = &session.EndedAt.Time
	}
	return codersdk.AISandboxSession{
		ID:                session.ID,
		WorkspaceID:       session.WorkspaceID,
		ReporterAgentID:   session.ReporterAgentID,
		ConfinedAgentID:   session.ConfinedAgentID,
		AIAgentID:         session.AIAgentID,
		SponsorUserID:     session.SponsorUserID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcement(session.EgressEnforcement),
		StartedAt:         session.StartedAt,
		EndedAt:           endedAt,
		CreatedAt:         session.CreatedAt,
	}
}

func convertAISandboxNetworkEvents(events []database.AISandboxNetworkEvent) []codersdk.AISandboxNetworkEventView {
	response := make([]codersdk.AISandboxNetworkEventView, 0, len(events))
	for _, event := range events {
		response = append(response, codersdk.AISandboxNetworkEventView{
			ID:             event.ID,
			SessionID:      event.SessionID,
			OccurredAt:     event.OccurredAt,
			Protocol:       codersdk.AISandboxNetworkProtocol(event.Protocol),
			Host:           event.Host,
			Port:           int(event.Port),
			Action:         codersdk.AISandboxNetworkEventAction(event.Action),
			PolicyRevision: event.PolicyRevision,
		})
	}
	return response
}

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
		api.publishWorkspaceAISandboxActivityUpdate(ctx, existing.WorkspaceID)
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
	api.publishWorkspaceAISandboxActivityUpdate(ctx, workspace.ID)
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
	workspaceIDs := make(map[uuid.UUID]struct{})
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
		workspaceIDs[session.WorkspaceID] = struct{}{}
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
	for workspaceID := range workspaceIDs {
		api.publishWorkspaceAISandboxActivityUpdate(ctx, workspaceID)
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{Message: "AI sandbox network events reported."})
}

func workspaceAISandboxActivityChannel(workspaceID uuid.UUID) string {
	return fmt.Sprintf("ws-ai-sandbox:%s", workspaceID)
}

func (api *API) publishWorkspaceAISandboxActivityUpdate(ctx context.Context, workspaceID uuid.UUID) {
	err := api.Pubsub.Publish(workspaceAISandboxActivityChannel(workspaceID), []byte{})
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish AI sandbox activity update",
			slog.F("workspace_id", workspaceID), slog.Error(err))
	}
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
