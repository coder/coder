package codersdk

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AIAuditEventType identifies the source of an AI audit timeline event.
type AIAuditEventType string

const (
	AIAuditEventTypeSandboxSessionStarted AIAuditEventType = "sandbox_session_started"
	AIAuditEventTypeSandboxSessionEnded   AIAuditEventType = "sandbox_session_ended"
	AIAuditEventTypeEgress                AIAuditEventType = "egress"
	AIAuditEventTypeBridgeSessionStarted  AIAuditEventType = "bridge_session_started"
	AIAuditEventTypeToolCall              AIAuditEventType = "tool_call"
	AIAuditEventTypeEscalationCreated     AIAuditEventType = "escalation_created"
	AIAuditEventTypeEscalationResolved    AIAuditEventType = "escalation_resolved"
)

// AIAuditEvent is one entry in the sponsor activity timeline. Detail carries
// content-free, type-specific fields; conversation content stays behind the
// drill-down surfaces (bridge session threads, escalation viewer, and the
// per-session egress table).
type AIAuditEvent struct {
	ID         uuid.UUID        `json:"id" format:"uuid"`
	Type       AIAuditEventType `json:"type"`
	OccurredAt time.Time        `json:"occurred_at" format:"date-time"`
	AIAgentID  uuid.UUID        `json:"ai_agent_id" format:"uuid"`
	Sponsor    MinimalUser      `json:"sponsor"`
	// WorkspaceID is zero when the source record does not reference a
	// workspace or the reference did not survive cleanup.
	WorkspaceID uuid.UUID `json:"workspace_id,omitempty" format:"uuid"`
	// WorkspaceName is only set for sources that snapshot it (escalations).
	WorkspaceName string         `json:"workspace_name,omitempty"`
	Summary       string         `json:"summary"`
	Detail        map[string]any `json:"detail"`
}

type AIAuditTimelineResponse struct {
	Events []AIAuditEvent `json:"events"`
	// Count is the number of events returned. Heterogeneous sources make a
	// grand total impractical, so there is none.
	Count int `json:"count"`
}

// AIAuditTimelineFilter filters the sponsor activity timeline.
//
// @typescript-ignore AIAuditTimelineFilter
type AIAuditTimelineFilter struct {
	// Sponsor is a user ID, username, or "me" (default). Naming another
	// user requires audit log read permission.
	Sponsor string
	// AIAgentID restricts events to a single agentic identity.
	AIAgentID uuid.UUID
	// AfterTime and BeforeTime exclusively bound occurred_at. Pass the
	// occurred_at of the last received event as BeforeTime to fetch the
	// next page.
	AfterTime  time.Time
	BeforeTime time.Time
	// Types restricts the event types returned; empty means all.
	Types []AIAuditEventType
	// Limit caps returned events. The server defaults to 100, max 1000.
	Limit int
}

func (f AIAuditTimelineFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if f.Sponsor != "" {
			q.Set("sponsor", f.Sponsor)
		}
		if f.AIAgentID != uuid.Nil {
			q.Set("ai_agent_id", f.AIAgentID.String())
		}
		if !f.AfterTime.IsZero() {
			q.Set("after_time", f.AfterTime.Format(time.RFC3339Nano))
		}
		if !f.BeforeTime.IsZero() {
			q.Set("before_time", f.BeforeTime.Format(time.RFC3339Nano))
		}
		if len(f.Types) > 0 {
			types := make([]string, 0, len(f.Types))
			for _, eventType := range f.Types {
				types = append(types, string(eventType))
			}
			q.Set("types", strings.Join(types, ","))
		}
		if f.Limit > 0 {
			q.Set("limit", strconv.Itoa(f.Limit))
		}
		r.URL.RawQuery = q.Encode()
	}
}

// AIAuditTimeline returns the merged AI activity timeline for a sponsor,
// newest first.
func (c *Client) AIAuditTimeline(ctx context.Context, filter AIAuditTimelineFilter) (AIAuditTimelineResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai-audit/timeline", nil, filter.asRequestOption())
	if err != nil {
		return AIAuditTimelineResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIAuditTimelineResponse{}, ReadBodyAsError(res)
	}
	var timeline AIAuditTimelineResponse
	return timeline, ReadBodyAsJSON(res, &timeline)
}

// AIAuditAgent is an agentic identity from the AI agent registry, attributed
// to the sponsoring user accountable for its activity.
type AIAuditAgent struct {
	// UserID identifies the AI agent's user record. Audit records reference
	// it as ai_agent_id.
	UserID      uuid.UUID `json:"user_id" format:"uuid"`
	Username    string    `json:"username"`
	OwnerUserID uuid.UUID `json:"owner_user_id" format:"uuid"`
	OriginType  string    `json:"origin_type"`
	OriginID    uuid.UUID `json:"origin_id" format:"uuid"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	Deleted     bool      `json:"deleted"`
}

// AIAuditAgents lists the agentic identities sponsored by the given user.
// Sponsor may be a user ID, a username, or "me"/empty for the caller. Naming
// another user requires audit log read permission.
func (c *Client) AIAuditAgents(ctx context.Context, sponsor string) ([]AIAuditAgent, error) {
	opts := []RequestOption{}
	if sponsor != "" {
		opts = append(opts, WithQueryParam("sponsor", sponsor))
	}
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai-audit/agents", nil, opts...)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var agents []AIAuditAgent
	return agents, ReadBodyAsJSON(res, &agents)
}
