package coderd

import (
	"context"
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
	"github.com/coder/coder/v2/codersdk"
)

const (
	aiAuditTrailDefaultLimit = 100
	aiAuditTrailMaxLimit     = 1000
)

// aiAuditTrailEventTypes is the closed set of timeline event types.
var aiAuditTrailEventTypes = map[codersdk.AIAuditTrailEventType]struct{}{
	codersdk.AIAuditTrailEventAIAgentLifecycle:       {},
	codersdk.AIAuditTrailEventAuthorizationLifecycle: {},
	codersdk.AIAuditTrailEventCredentialLifecycle:    {},
	codersdk.AIAuditTrailEventCredentialUse:          {},
	codersdk.AIAuditTrailEventSandboxSession:         {},
	codersdk.AIAuditTrailEventEgress:                 {},
}

// @Summary Get AI agent audit trail timeline
// @ID get-ai-agent-audit-trail-timeline
// @Security CoderSessionToken
// @Produce json
// @Tags Audit
// @Param owner query string false "Owner user ID, username, or 'me' (default). Current-owner semantics."
// @Param after_time query string false "Lower time bound, exclusive (RFC3339)" format(date-time)
// @Param before_time query string false "Upper time bound, exclusive (RFC3339)" format(date-time)
// @Param ai_agent_id query string false "Filter to one AI agent" format(uuid)
// @Param types query string false "Comma-separated event types"
// @Param limit query int false "Page size (default 100, max 1000)"
// @Success 200 {object} codersdk.AIAuditTrailResponse
// @Router /ai-audit/timeline [get]
func (api *API) aiAuditTimeline(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// The key holder must be a user: an AI agent's own credential cannot
	// read its owner's trail.
	callerID, isUser := httpmw.APIKey(r).UserID()
	if !isUser {
		httpapi.Forbidden(rw)
		return
	}
	values := r.URL.Query()

	parser := httpapi.NewQueryParamParser()
	afterTime := parser.Time3339Nano(values, time.Time{}, "after_time")
	beforeTime := parser.Time3339Nano(values, time.Time{}, "before_time")
	agentID := parser.UUID(values, uuid.Nil, "ai_agent_id")
	limit := parser.Int(values, aiAuditTrailDefaultLimit, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid query parameters.",
			Validations: parser.Errors,
		})
		return
	}
	if limit < 1 || limit > aiAuditTrailMaxLimit {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid limit value.",
			Detail:  fmt.Sprintf("limit must be in range [1, %d]", aiAuditTrailMaxLimit),
		})
		return
	}
	if !beforeTime.IsZero() && !afterTime.IsZero() && !beforeTime.After(afterTime) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid time bounds.",
			Detail:  "before_time must be after after_time.",
		})
		return
	}

	wantTypes, ok := parseAIAuditTrailTypes(ctx, rw, values.Get("types"))
	if !ok {
		return
	}

	owner, ok := api.resolveAIAuditTrailOwner(rw, r, callerID, values.Get("owner"))
	if !ok {
		return
	}

	sdkOwner := codersdk.AIAuditTrailOwner{
		Type:     "user",
		ID:       owner.ID,
		Username: owner.Username,
	}

	events := make([]codersdk.AIAuditTrailEvent, 0, limit)
	collect := func(eventType codersdk.AIAuditTrailEventType, fetch func() ([]codersdk.AIAuditTrailEvent, error)) bool {
		if _, wanted := wantTypes[eventType]; !wanted {
			return true
		}
		batch, err := fetch()
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return false
		}
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return false
		}
		events = append(events, batch...)
		return true
	}

	// #nosec G115 -- limit is bounded to [1, 1000] above.
	limit32 := int32(limit)
	ok = collect(codersdk.AIAuditTrailEventAIAgentLifecycle, func() ([]codersdk.AIAuditTrailEvent, error) {
		rows, err := api.Database.ListAIAgentLifecycleTrailEvents(ctx, database.ListAIAgentLifecycleTrailEventsParams{
			OwnerID: owner.ID, AIAgentID: agentID, AfterTime: afterTime, BeforeTime: beforeTime, Limit: limit32,
		})
		if err != nil {
			return nil, err
		}
		return convertAIAgentLifecycleTrailEvents(rows, sdkOwner), nil
	})
	if !ok {
		return
	}
	ok = collect(codersdk.AIAuditTrailEventAuthorizationLifecycle, func() ([]codersdk.AIAuditTrailEvent, error) {
		rows, err := api.Database.ListAuthorizationLifecycleTrailEvents(ctx, database.ListAuthorizationLifecycleTrailEventsParams{
			OwnerID: owner.ID, AIAgentID: agentID, AfterTime: afterTime, BeforeTime: beforeTime, Limit: limit32,
		})
		if err != nil {
			return nil, err
		}
		return convertAuthorizationLifecycleTrailEvents(rows, sdkOwner), nil
	})
	if !ok {
		return
	}
	ok = collect(codersdk.AIAuditTrailEventCredentialLifecycle, func() ([]codersdk.AIAuditTrailEvent, error) {
		rows, err := api.Database.ListCredentialLifecycleTrailEvents(ctx, database.ListCredentialLifecycleTrailEventsParams{
			OwnerID: owner.ID, AIAgentID: agentID, AfterTime: afterTime, BeforeTime: beforeTime, Limit: limit32,
		})
		if err != nil {
			return nil, err
		}
		return convertCredentialLifecycleTrailEvents(rows, sdkOwner), nil
	})
	if !ok {
		return
	}
	ok = collect(codersdk.AIAuditTrailEventCredentialUse, func() ([]codersdk.AIAuditTrailEvent, error) {
		rows, err := api.Database.ListCredentialUseTrailEvents(ctx, database.ListCredentialUseTrailEventsParams{
			OwnerID: owner.ID, AIAgentID: agentID, AfterTime: afterTime, BeforeTime: beforeTime, Limit: limit32,
		})
		if err != nil {
			return nil, err
		}
		return convertCredentialUseTrailEvents(rows, sdkOwner), nil
	})
	if !ok {
		return
	}
	ok = collect(codersdk.AIAuditTrailEventSandboxSession, func() ([]codersdk.AIAuditTrailEvent, error) {
		rows, err := api.Database.ListAISandboxSessionTrailRows(ctx, database.ListAISandboxSessionTrailRowsParams{
			OwnerID: owner.ID, AIAgentID: agentID, AfterTime: afterTime, BeforeTime: beforeTime, Limit: limit32,
		})
		if err != nil {
			return nil, err
		}
		return convertAISandboxSessionTrailEvents(rows, sdkOwner, afterTime, beforeTime), nil
	})
	if !ok {
		return
	}
	ok = collect(codersdk.AIAuditTrailEventEgress, func() ([]codersdk.AIAuditTrailEvent, error) {
		rows, err := api.Database.ListAISandboxEgressTrailAggregates(ctx, database.ListAISandboxEgressTrailAggregatesParams{
			OwnerID: owner.ID, AIAgentID: agentID, AfterTime: afterTime, BeforeTime: beforeTime, Limit: limit32,
		})
		if err != nil {
			return nil, err
		}
		return convertAISandboxEgressTrailEvents(rows, sdkOwner), nil
	})
	if !ok {
		return
	}

	// Merge newest-first. Cross-source ordering by occurred_at is
	// presentation; the tiebreak only makes pagination deterministic.
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].OccurredAt.After(events[j].OccurredAt)
		}
		if events[i].Type != events[j].Type {
			return events[i].Type < events[j].Type
		}
		return events[i].ID > events[j].ID
	})
	if len(events) > limit {
		events = events[:limit]
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AIAuditTrailResponse{
		Events: events,
		Count:  len(events),
	})
}

func parseAIAuditTrailTypes(ctx context.Context, rw http.ResponseWriter, raw string) (map[codersdk.AIAuditTrailEventType]struct{}, bool) {
	if raw == "" {
		return aiAuditTrailEventTypes, true
	}
	want := make(map[codersdk.AIAuditTrailEventType]struct{})
	for _, name := range strings.Split(raw, ",") {
		eventType := codersdk.AIAuditTrailEventType(strings.TrimSpace(name))
		if _, known := aiAuditTrailEventTypes[eventType]; !known {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid event type filter.",
				Detail:  fmt.Sprintf("unknown event type %q", eventType),
			})
			return nil, false
		}
		want[eventType] = struct{}{}
	}
	return want, true
}

// resolveAIAuditTrailOwner resolves the owner query parameter to a user
// under current-owner semantics. "me" and the empty string mean the caller.
func (api *API) resolveAIAuditTrailOwner(rw http.ResponseWriter, r *http.Request, callerID uuid.UUID, raw string) (database.User, bool) {
	ctx := r.Context()

	var (
		owner database.User
		err   error
	)
	switch {
	case raw == "" || raw == codersdk.Me:
		owner, err = api.Database.GetUserByID(ctx, callerID)
	default:
		if ownerID, parseErr := uuid.Parse(raw); parseErr == nil {
			owner, err = api.Database.GetUserByID(ctx, ownerID)
		} else {
			owner, err = api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
				Username: raw,
			})
		}
	}
	if dbauthz.IsNotAuthorizedError(err) {
		httpapi.Forbidden(rw)
		return database.User{}, false
	}
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown owner.",
			Detail:  fmt.Sprintf("no user matches %q", raw),
		})
		return database.User{}, false
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return database.User{}, false
	}
	return owner, true
}

func convertAIAgentLifecycleTrailEvents(rows []database.ListAIAgentLifecycleTrailEventsRow, owner codersdk.AIAuditTrailOwner) []codersdk.AIAuditTrailEvent {
	events := make([]codersdk.AIAuditTrailEvent, 0, len(rows))
	for _, row := range rows {
		detail := map[string]any{
			"event":      row.Event,
			"entry_id":   row.EntryID,
			"actor_type": row.ActorType,
			"actor":      row.Actor,
		}
		summary := "AI agent " + row.Event
		switch row.Event {
		case "create":
			summary = "AI agent created"
			if row.CreationSiteType.Valid {
				summary = fmt.Sprintf("AI agent created in %s", row.CreationSiteType.String)
				detail["creation_site_type"] = row.CreationSiteType.String
			}
			if row.CreationSiteID.Valid {
				detail["creation_site_id"] = row.CreationSiteID.UUID
			}
		case "finish":
			summary = "AI agent finished"
		case "kill":
			summary = "AI agent killed"
		}
		events = append(events, codersdk.AIAuditTrailEvent{
			ID:         fmt.Sprintf("ai_agent_lifecycle:%d", row.EntryID),
			Type:       codersdk.AIAuditTrailEventAIAgentLifecycle,
			OccurredAt: row.EffectiveDate,
			RecordedAt: row.RecordingDate,
			AIAgentID:  row.AIAgentID,
			Owner:      owner,
			Summary:    summary,
			Detail:     detail,
		})
	}
	return events
}

func convertAuthorizationLifecycleTrailEvents(rows []database.ListAuthorizationLifecycleTrailEventsRow, owner codersdk.AIAuditTrailOwner) []codersdk.AIAuditTrailEvent {
	events := make([]codersdk.AIAuditTrailEvent, 0, len(rows))
	for _, row := range rows {
		detail := map[string]any{
			"event":            row.Event,
			"entry_id":         row.EntryID,
			"authorization_id": row.AuthorizationID,
			"principal_type":   row.PrincipalType,
			"principal_id":     row.PrincipalID,
		}
		if row.ActorType.Valid {
			detail["actor_type"] = row.ActorType.String
		}
		if row.Actor.Valid {
			detail["actor"] = row.Actor.UUID
		}
		summary := "authorization " + row.Event
		switch row.Event {
		case "grant":
			summary = "authorization granted"
		case "lapse":
			summary = "authorization lapsed"
		}
		events = append(events, codersdk.AIAuditTrailEvent{
			ID:         fmt.Sprintf("authorization_lifecycle:%d:%d", row.EntryID, row.Line),
			Type:       codersdk.AIAuditTrailEventAuthorizationLifecycle,
			OccurredAt: row.EffectiveDate.Time,
			RecordedAt: row.RecordingDate.Time,
			AIAgentID:  row.AIAgentID,
			Owner:      owner,
			Summary:    summary,
			Detail:     detail,
		})
	}
	return events
}

func convertCredentialLifecycleTrailEvents(rows []database.ListCredentialLifecycleTrailEventsRow, owner codersdk.AIAuditTrailOwner) []codersdk.AIAuditTrailEvent {
	events := make([]codersdk.AIAuditTrailEvent, 0, len(rows))
	for _, row := range rows {
		detail := map[string]any{
			"event":           row.Event,
			"entry_id":        row.EntryID,
			"credential_id":   row.CredentialID,
			"credential_type": row.CredentialType,
		}
		if row.ActorType.Valid {
			detail["actor_type"] = row.ActorType.String
		}
		if row.Actor.Valid {
			detail["actor"] = row.Actor.UUID
		}
		if row.TokenName.Valid {
			detail["token_name"] = row.TokenName.String
		}
		summary := fmt.Sprintf("credential %s (%s)", pastTenseCredentialEvent(row.Event), row.CredentialType)
		events = append(events, codersdk.AIAuditTrailEvent{
			ID:         fmt.Sprintf("credential_lifecycle:%d:%d", row.EntryID, row.Line),
			Type:       codersdk.AIAuditTrailEventCredentialLifecycle,
			OccurredAt: row.EffectiveDate,
			RecordedAt: row.RecordingDate,
			AIAgentID:  row.AIAgentID,
			Owner:      owner,
			Summary:    summary,
			Detail:     detail,
		})
	}
	return events
}

// pastTenseCredentialEvent renders the journal's event word for a summary
// sentence. The Detail payload always carries the verbatim word.
func pastTenseCredentialEvent(event string) string {
	switch event {
	case "issue":
		return "issued"
	case "revoke":
		return "revoked"
	case "lapse":
		return "lapsed"
	case "discharge":
		return "discharged"
	default:
		return event
	}
}

func convertCredentialUseTrailEvents(rows []database.ListCredentialUseTrailEventsRow, owner codersdk.AIAuditTrailOwner) []codersdk.AIAuditTrailEvent {
	events := make([]codersdk.AIAuditTrailEvent, 0, len(rows))
	for _, row := range rows {
		detail := map[string]any{
			"event":           row.Event,
			"entry_id":        row.EntryID,
			"credential_id":   row.CredentialID,
			"credential_type": row.CredentialType,
			"actor_type":      row.ActorType,
			"actor":           row.Actor,
		}
		if row.AnnotationSource.Valid {
			detail["source"] = row.AnnotationSource.String
		}
		summary := fmt.Sprintf("credential presentation accepted (%s)", row.CredentialType)
		if row.Event == "presentation_refused" {
			summary = fmt.Sprintf("credential presentation refused (%s)", row.CredentialType)
		}
		events = append(events, codersdk.AIAuditTrailEvent{
			ID:         fmt.Sprintf("credential_use:%d", row.EntryID),
			Type:       codersdk.AIAuditTrailEventCredentialUse,
			OccurredAt: row.EffectiveDate,
			RecordedAt: row.RecordingDate,
			AIAgentID:  row.AIAgentID,
			Owner:      owner,
			Summary:    summary,
			Detail:     detail,
		})
	}
	return events
}

// convertAISandboxSessionTrailEvents expands each session row into started
// and ended events, trimming to the requested window because the row query
// matches a superset.
func convertAISandboxSessionTrailEvents(rows []database.AISandboxSession, owner codersdk.AIAuditTrailOwner, afterTime, beforeTime time.Time) []codersdk.AIAuditTrailEvent {
	inWindow := func(t time.Time) bool {
		if !afterTime.IsZero() && !t.After(afterTime) {
			return false
		}
		if !beforeTime.IsZero() && !t.Before(beforeTime) {
			return false
		}
		return true
	}
	events := make([]codersdk.AIAuditTrailEvent, 0, len(rows)*2)
	for _, row := range rows {
		workspaceID := row.WorkspaceID
		if inWindow(row.StartedAt) {
			events = append(events, codersdk.AIAuditTrailEvent{
				ID:          fmt.Sprintf("sandbox_session:%s:started", row.ID),
				Type:        codersdk.AIAuditTrailEventSandboxSession,
				OccurredAt:  row.StartedAt,
				RecordedAt:  row.CreatedAt,
				AIAgentID:   row.AIAgentID,
				Owner:       owner,
				WorkspaceID: &workspaceID,
				Summary:     fmt.Sprintf("sandbox egress session started (%s)", row.EgressEnforcement),
				Detail: map[string]any{
					"event":              "started",
					"session_id":         row.ID,
					"egress_enforcement": row.EgressEnforcement,
				},
			})
		}
		if row.EndedAt.Valid && inWindow(row.EndedAt.Time) {
			events = append(events, codersdk.AIAuditTrailEvent{
				ID:          fmt.Sprintf("sandbox_session:%s:ended", row.ID),
				Type:        codersdk.AIAuditTrailEventSandboxSession,
				OccurredAt:  row.EndedAt.Time,
				RecordedAt:  row.CreatedAt,
				AIAgentID:   row.AIAgentID,
				Owner:       owner,
				WorkspaceID: &workspaceID,
				Summary:     fmt.Sprintf("sandbox egress session ended (%s)", row.EgressEnforcement),
				Detail: map[string]any{
					"event":              "ended",
					"session_id":         row.ID,
					"egress_enforcement": row.EgressEnforcement,
					"duration_ms":        row.EndedAt.Time.Sub(row.StartedAt).Milliseconds(),
				},
			})
		}
	}
	return events
}

func convertAISandboxEgressTrailEvents(rows []database.ListAISandboxEgressTrailAggregatesRow, owner codersdk.AIAuditTrailOwner) []codersdk.AIAuditTrailEvent {
	events := make([]codersdk.AIAuditTrailEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, codersdk.AIAuditTrailEvent{
			ID:         fmt.Sprintf("egress:%s:%s:%s", row.SessionID, row.Host, row.Action),
			Type:       codersdk.AIAuditTrailEventEgress,
			OccurredAt: row.OccurredAt,
			RecordedAt: row.RecordedAt,
			AIAgentID:  row.AIAgentID,
			Owner:      owner,
			Summary:    fmt.Sprintf("%s %s %s:%d (x%d)", row.Action, row.Protocol, row.Host, row.Port, row.EventCount),
			Detail: map[string]any{
				"event":      row.Action,
				"session_id": row.SessionID,
				"host":       row.Host,
				"port":       row.Port,
				"protocol":   row.Protocol,
				"count":      row.EventCount,
			},
		})
	}
	return events
}
