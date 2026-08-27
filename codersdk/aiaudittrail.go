package codersdk

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AIAuditTrailEventType names one source feeding the AI audit trail
// timeline. The type names the source, not the verb: journal-sourced events
// keep their journal's own event vocabulary in Detail under "event".
type AIAuditTrailEventType string

const (
	AIAuditTrailEventAIAgentLifecycle       AIAuditTrailEventType = "ai_agent_lifecycle"
	AIAuditTrailEventAuthorizationLifecycle AIAuditTrailEventType = "authorization_lifecycle"
	//nolint:gosec // An event type name, not a credential.
	AIAuditTrailEventCredentialLifecycle AIAuditTrailEventType = "credential_lifecycle"
	//nolint:gosec // An event type name, not a credential.
	AIAuditTrailEventCredentialUse  AIAuditTrailEventType = "credential_use"
	AIAuditTrailEventSandboxSession AIAuditTrailEventType = "sandbox_session"
	AIAuditTrailEventEgress         AIAuditTrailEventType = "egress"
)

// AIAuditTrailOwner is the principal the trail was filtered by, under
// current-owner semantics: the ledger's present owner, not necessarily the
// owner at the time an event occurred.
type AIAuditTrailOwner struct {
	Type     string    `json:"type"`
	ID       uuid.UUID `json:"id" format:"uuid"`
	Username string    `json:"username"`
}

// AIAuditTrailEvent is one event in the merged timeline. OccurredAt is the
// effective date and RecordedAt the recording date; both are preserved
// because an observed transition may be recorded long after it happened.
// Cross-source ordering by OccurredAt is presentation, not a total-order
// claim; within one journal the entry id in Detail is authoritative.
type AIAuditTrailEvent struct {
	ID          string                `json:"id"`
	Type        AIAuditTrailEventType `json:"type"`
	OccurredAt  time.Time             `json:"occurred_at" format:"date-time"`
	RecordedAt  time.Time             `json:"recorded_at" format:"date-time"`
	AIAgentID   uuid.UUID             `json:"ai_agent_id" format:"uuid"`
	Owner       AIAuditTrailOwner     `json:"owner"`
	WorkspaceID *uuid.UUID            `json:"workspace_id,omitempty" format:"uuid"`
	Summary     string                `json:"summary"`
	Detail      map[string]any        `json:"detail"`
}

type AIAuditTrailResponse struct {
	Events []AIAuditTrailEvent `json:"events"`
	// Count is the number of events returned. There is no total across the
	// heterogeneous sources.
	Count int `json:"count"`
}

// AIAuditTrailFilter filters the timeline. Owner accepts a user ID, a
// username, or "me" (the default). Time bounds are exclusive; pass
// BeforeTime = the OccurredAt of the last event received to fetch the next
// page.
//
// @typescript-ignore AIAuditTrailFilter
type AIAuditTrailFilter struct {
	Owner      string
	AfterTime  time.Time
	BeforeTime time.Time
	AIAgentID  uuid.UUID
	Types      []AIAuditTrailEventType
	// Limit defaults to 100, max is 1000.
	Limit int
}

func (f AIAuditTrailFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if f.Owner != "" {
			q.Set("owner", f.Owner)
		}
		if !f.AfterTime.IsZero() {
			q.Set("after_time", f.AfterTime.Format(time.RFC3339Nano))
		}
		if !f.BeforeTime.IsZero() {
			q.Set("before_time", f.BeforeTime.Format(time.RFC3339Nano))
		}
		if f.AIAgentID != uuid.Nil {
			q.Set("ai_agent_id", f.AIAgentID.String())
		}
		if len(f.Types) > 0 {
			names := make([]string, 0, len(f.Types))
			for _, eventType := range f.Types {
				names = append(names, string(eventType))
			}
			q.Set("types", strings.Join(names, ","))
		}
		if f.Limit > 0 {
			q.Set("limit", strconv.Itoa(f.Limit))
		}
		r.URL.RawQuery = q.Encode()
	}
}

// AIAuditTrailTimeline returns the merged, owner-scoped timeline of AI
// agent activity.
func (c *Client) AIAuditTrailTimeline(ctx context.Context, filter AIAuditTrailFilter) (AIAuditTrailResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai-audit/timeline", nil, filter.asRequestOption())
	if err != nil {
		return AIAuditTrailResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIAuditTrailResponse{}, ReadBodyAsError(res)
	}
	var response AIAuditTrailResponse
	return response, ReadBodyAsJSON(res, &response)
}
