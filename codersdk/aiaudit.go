package codersdk

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

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
