package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

const (
	aiAuditTimelineDefaultLimit = 100
	aiAuditTimelineMaxLimit     = 1000
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

// aiAuditTimelineQuery carries the validated timeline filters shared by
// every per-source query.
type aiAuditTimelineQuery struct {
	SponsorUserID uuid.UUID
	AIAgentID     uuid.UUID
	AfterTime     time.Time
	BeforeTime    time.Time
	Limit         int32
	Include       map[codersdk.AIAuditEventType]bool
}

func (q aiAuditTimelineQuery) includes(eventType codersdk.AIAuditEventType) bool {
	if len(q.Include) == 0 {
		return true
	}
	return q.Include[eventType]
}

// inWindow reports whether an event timestamp falls inside the exclusive
// (AfterTime, BeforeTime) window; zero bounds are disabled.
func (q aiAuditTimelineQuery) inWindow(at time.Time) bool {
	if !q.AfterTime.IsZero() && !at.After(q.AfterTime) {
		return false
	}
	if !q.BeforeTime.IsZero() && !at.Before(q.BeforeTime) {
		return false
	}
	return true
}

// @Summary Get the AI activity timeline for a sponsor
// @ID get-the-ai-activity-timeline-for-a-sponsor
// @Security CoderSessionToken
// @Produce json
// @Tags Audit
// @Param sponsor query string false "Sponsor user ID, username, or 'me' (default)"
// @Param ai_agent_id query string false "Restrict events to one agentic identity" format(uuid)
// @Param after_time query string false "Exclusive lower bound on occurred_at (RFC3339)" format(date-time)
// @Param before_time query string false "Exclusive upper bound on occurred_at (RFC3339); pass the last event's occurred_at to page" format(date-time)
// @Param types query string false "Comma-separated event types to include"
// @Param limit query int false "Page size (default 100, max 1000)"
// @Success 200 {object} codersdk.AIAuditTimelineResponse
// @Router /api/v2/ai-audit/timeline [get]
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) aiAuditTimeline(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sponsorID, ok := api.resolveAIAuditSponsor(rw, r)
	if !ok {
		return
	}

	query, ok := parseAIAuditTimelineQuery(rw, r, sponsorID)
	if !ok {
		return
	}

	//nolint:gocritic // All per-source queries are system-guarded and scoped to the resolved sponsor.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	var (
		events []codersdk.AIAuditEvent
		// floors records, per truncated source, the timestamp below which
		// that source's events may be incomplete. Events older than the
		// max floor are dropped from this page; the next page (bounded by
		// before_time) re-fetches them.
		floors []time.Time
	)

	appendSource := func(sourceEvents []codersdk.AIAuditEvent, truncated bool, oldestComplete time.Time) {
		events = append(events, sourceEvents...)
		if truncated {
			floors = append(floors, oldestComplete)
		}
	}

	if query.includes(codersdk.AIAuditEventTypeSandboxSessionStarted) || query.includes(codersdk.AIAuditEventTypeSandboxSessionEnded) {
		rows, err := api.Database.ListAISandboxSessionsBySponsor(sysCtx, database.ListAISandboxSessionsBySponsorParams{
			SponsorUserID: query.SponsorUserID,
			AIAgentID:     query.AIAgentID,
			AfterTime:     query.AfterTime,
			BeforeTime:    query.BeforeTime,
			Limit:         query.Limit,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		sourceEvents := make([]codersdk.AIAuditEvent, 0, len(rows))
		var oldest time.Time
		for _, row := range rows {
			oldest = row.StartedAt
			if row.EndedAt.Valid && row.EndedAt.Time.After(oldest) {
				oldest = row.EndedAt.Time
			}
			if query.includes(codersdk.AIAuditEventTypeSandboxSessionStarted) && query.inWindow(row.StartedAt) {
				sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
					ID:          row.ID,
					Type:        codersdk.AIAuditEventTypeSandboxSessionStarted,
					OccurredAt:  row.StartedAt,
					AIAgentID:   row.AIAgentID,
					WorkspaceID: row.WorkspaceID,
					Summary:     fmt.Sprintf("sandbox session started (egress %s)", row.EgressEnforcement),
					Detail: map[string]any{
						"egress_enforcement": row.EgressEnforcement,
					},
				})
			}
			if query.includes(codersdk.AIAuditEventTypeSandboxSessionEnded) && row.EndedAt.Valid && query.inWindow(row.EndedAt.Time) {
				sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
					ID:          row.ID,
					Type:        codersdk.AIAuditEventTypeSandboxSessionEnded,
					OccurredAt:  row.EndedAt.Time,
					AIAgentID:   row.AIAgentID,
					WorkspaceID: row.WorkspaceID,
					Summary:     fmt.Sprintf("sandbox session ended after %s", row.EndedAt.Time.Sub(row.StartedAt).Round(time.Second)),
					Detail: map[string]any{
						"egress_enforcement": row.EgressEnforcement,
						"duration_ms":        row.EndedAt.Time.Sub(row.StartedAt).Milliseconds(),
					},
				})
			}
		}
		// The source orders rows by GREATEST(started_at, ended_at), so
		// when truncated, events strictly older than the last row's order
		// key may be missing.
		appendSource(sourceEvents, len(rows) == int(query.Limit), oldest)
	}

	if query.includes(codersdk.AIAuditEventTypeEgress) {
		rows, err := api.Database.ListAISandboxNetworkEventAggregatesBySponsor(sysCtx, database.ListAISandboxNetworkEventAggregatesBySponsorParams{
			SponsorUserID: query.SponsorUserID,
			AIAgentID:     query.AIAgentID,
			AfterTime:     query.AfterTime,
			BeforeTime:    query.BeforeTime,
			Limit:         query.Limit,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		sourceEvents := make([]codersdk.AIAuditEvent, 0, len(rows))
		var oldest time.Time
		for _, row := range rows {
			oldest = row.LastOccurredAt
			sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
				// Deterministic synthetic ID: aggregate buckets have no
				// row identity of their own.
				ID:          uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "ai-audit-egress|%s|%s|%s", row.SessionID, row.Host, row.Action)),
				Type:        codersdk.AIAuditEventTypeEgress,
				OccurredAt:  row.LastOccurredAt,
				AIAgentID:   row.AIAgentID,
				WorkspaceID: row.WorkspaceID.UUID,
				Summary:     fmt.Sprintf("%s %s %s:%d (x%d)", row.Action, row.Protocol, row.Host, row.Port, row.EventCount),
				Detail: map[string]any{
					"session_id": row.SessionID,
					"host":       row.Host,
					"port":       row.Port,
					"protocol":   row.Protocol,
					"action":     row.Action,
					"count":      row.EventCount,
				},
			})
		}
		appendSource(sourceEvents, len(rows) == int(query.Limit), oldest)
	}

	if query.includes(codersdk.AIAuditEventTypeBridgeSessionStarted) {
		rows, err := api.Database.ListAIBridgeSessionStartsBySponsor(sysCtx, database.ListAIBridgeSessionStartsBySponsorParams{
			SponsorUserID: query.SponsorUserID,
			AIAgentID:     query.AIAgentID,
			AfterTime:     query.AfterTime,
			BeforeTime:    query.BeforeTime,
			Limit:         query.Limit,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		sourceEvents := make([]codersdk.AIAuditEvent, 0, len(rows))
		var oldest time.Time
		for _, row := range rows {
			oldest = row.StartedAt
			id, err := uuid.Parse(row.SessionID)
			if err != nil {
				// Client-supplied session IDs are free-form text.
				id = uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "ai-audit-bridge-session|%s", row.SessionID))
			}
			sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
				ID:         id,
				Type:       codersdk.AIAuditEventTypeBridgeSessionStarted,
				OccurredAt: row.StartedAt,
				AIAgentID:  row.InitiatorID,
				Summary:    fmt.Sprintf("bridge session started via %s", row.Client),
				Detail: map[string]any{
					"session_id": row.SessionID,
					"client":     row.Client,
					"providers":  row.Providers,
					"models":     row.Models,
				},
			})
		}
		appendSource(sourceEvents, len(rows) == int(query.Limit), oldest)
	}

	if query.includes(codersdk.AIAuditEventTypeToolCall) {
		rows, err := api.Database.ListAIBridgeToolUsagesBySponsor(sysCtx, database.ListAIBridgeToolUsagesBySponsorParams{
			SponsorUserID: query.SponsorUserID,
			AIAgentID:     query.AIAgentID,
			AfterTime:     query.AfterTime,
			BeforeTime:    query.BeforeTime,
			Limit:         query.Limit,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		sourceEvents := make([]codersdk.AIAuditEvent, 0, len(rows))
		var oldest time.Time
		for _, row := range rows {
			oldest = row.CreatedAt
			detail := map[string]any{
				"interception_id": row.InterceptionID,
				"server_url":      row.ServerUrl,
				"tool":            row.Tool,
				"disposition":     row.Disposition,
			}
			if row.EscalationID.Valid {
				detail["escalation_id"] = row.EscalationID.UUID
			}
			sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
				ID:         row.ID,
				Type:       codersdk.AIAuditEventTypeToolCall,
				OccurredAt: row.CreatedAt,
				AIAgentID:  row.AIAgentID,
				Summary:    fmt.Sprintf("tool call %s: %s", row.Tool, row.Disposition),
				Detail:     detail,
			})
		}
		appendSource(sourceEvents, len(rows) == int(query.Limit), oldest)
	}

	if query.includes(codersdk.AIAuditEventTypeEscalationCreated) || query.includes(codersdk.AIAuditEventTypeEscalationResolved) {
		rows, err := api.Database.ListMCPGatewayEscalationsBySponsor(sysCtx, database.ListMCPGatewayEscalationsBySponsorParams{
			SponsorUserID: query.SponsorUserID,
			Status:        "",
			AIAgentID:     query.AIAgentID,
			AfterTime:     query.AfterTime,
			BeforeTime:    query.BeforeTime,
			Limit:         query.Limit,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		sourceEvents := make([]codersdk.AIAuditEvent, 0, len(rows))
		var oldest time.Time
		for _, row := range rows {
			oldest = row.CreatedAt
			if row.ResolvedAt.Valid && row.ResolvedAt.Time.After(oldest) {
				oldest = row.ResolvedAt.Time
			}
			if query.includes(codersdk.AIAuditEventTypeEscalationCreated) && query.inWindow(row.CreatedAt) {
				sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
					ID:            row.ID,
					Type:          codersdk.AIAuditEventTypeEscalationCreated,
					OccurredAt:    row.CreatedAt,
					AIAgentID:     row.AIAgentID,
					WorkspaceName: row.WorkspaceName,
					Summary:       fmt.Sprintf("escalation created for tool %s on %s", row.Tool, row.ServerSlug),
					Detail: map[string]any{
						"escalation_id": row.ID,
						"server_slug":   row.ServerSlug,
						"tool":          row.Tool,
						"expires_at":    row.ExpiresAt,
					},
				})
			}
			if query.includes(codersdk.AIAuditEventTypeEscalationResolved) && row.ResolvedAt.Valid && query.inWindow(row.ResolvedAt.Time) {
				detail := map[string]any{
					"escalation_id": row.ID,
					"server_slug":   row.ServerSlug,
					"tool":          row.Tool,
					"status":        row.Status,
				}
				if row.ResolvedBy.Valid {
					detail["resolved_by"] = row.ResolvedBy.UUID
				}
				sourceEvents = append(sourceEvents, codersdk.AIAuditEvent{
					ID:            row.ID,
					Type:          codersdk.AIAuditEventTypeEscalationResolved,
					OccurredAt:    row.ResolvedAt.Time,
					AIAgentID:     row.AIAgentID,
					WorkspaceName: row.WorkspaceName,
					Summary:       fmt.Sprintf("escalation %s for tool %s", row.Status, row.Tool),
					Detail:        detail,
				})
			}
		}
		appendSource(sourceEvents, len(rows) == int(query.Limit), oldest)
	}

	// Merge newest-first with a stable tiebreak so pagination is
	// deterministic.
	sort.Slice(events, func(i, j int) bool {
		if !events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].OccurredAt.After(events[j].OccurredAt)
		}
		if events[i].Type != events[j].Type {
			return events[i].Type < events[j].Type
		}
		return events[i].ID.String() < events[j].ID.String()
	})

	// Trim below the completeness floor: a truncated source may be missing
	// events older than its last returned row, so events from other sources
	// older than that boundary would render a misleading gap. They are
	// re-fetched on the next page via before_time.
	var floor time.Time
	for _, f := range floors {
		if f.After(floor) {
			floor = f
		}
	}
	if !floor.IsZero() {
		trimmed := events[:0]
		for _, event := range events {
			if !event.OccurredAt.Before(floor) {
				trimmed = append(trimmed, event)
			}
		}
		events = trimmed
	}
	if len(events) > int(query.Limit) {
		events = events[:query.Limit]
	}

	sponsor := codersdk.MinimalUser{ID: query.SponsorUserID}
	if user, err := api.Database.GetUserByID(sysCtx, query.SponsorUserID); err == nil {
		sponsor.Username = user.Username
		sponsor.Name = user.Name
		sponsor.AvatarURL = user.AvatarURL
	}
	for i := range events {
		events[i].Sponsor = sponsor
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AIAuditTimelineResponse{
		Events: events,
		Count:  len(events),
	})
}

// parseAIAuditTimelineQuery validates the timeline query parameters. It
// writes an error response and returns false when validation fails.
func parseAIAuditTimelineQuery(rw http.ResponseWriter, r *http.Request, sponsorID uuid.UUID) (aiAuditTimelineQuery, bool) {
	ctx := r.Context()
	values := r.URL.Query()

	parser := httpapi.NewQueryParamParser()
	query := aiAuditTimelineQuery{
		SponsorUserID: sponsorID,
		AIAgentID:     parser.UUID(values, uuid.Nil, "ai_agent_id"),
		AfterTime:     parser.Time3339Nano(values, time.Time{}, "after_time"),
		BeforeTime:    parser.Time3339Nano(values, time.Time{}, "before_time"),
		Limit:         parser.PositiveInt32(values, aiAuditTimelineDefaultLimit, "limit"),
	}
	if query.Limit <= 0 {
		query.Limit = aiAuditTimelineDefaultLimit
	}
	if query.Limit > aiAuditTimelineMaxLimit {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  "limit",
			Detail: fmt.Sprintf("limit must be in range (0, %d]", aiAuditTimelineMaxLimit),
		})
	}
	if typesParam := values.Get("types"); typesParam != "" {
		query.Include = make(map[codersdk.AIAuditEventType]bool)
		for _, raw := range strings.Split(typesParam, ",") {
			eventType := codersdk.AIAuditEventType(strings.TrimSpace(raw))
			switch eventType {
			case codersdk.AIAuditEventTypeSandboxSessionStarted,
				codersdk.AIAuditEventTypeSandboxSessionEnded,
				codersdk.AIAuditEventTypeEgress,
				codersdk.AIAuditEventTypeBridgeSessionStarted,
				codersdk.AIAuditEventTypeToolCall,
				codersdk.AIAuditEventTypeEscalationCreated,
				codersdk.AIAuditEventTypeEscalationResolved:
				query.Include[eventType] = true
			default:
				parser.Errors = append(parser.Errors, codersdk.ValidationError{
					Field:  "types",
					Detail: fmt.Sprintf("unknown event type %q", raw),
				})
			}
		}
	}
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI audit timeline query.",
			Validations: parser.Errors,
		})
		return aiAuditTimelineQuery{}, false
	}
	return query, true
}
