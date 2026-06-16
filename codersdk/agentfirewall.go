package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AgentFirewallSession represents a firewall session for a workspace agent.
type AgentFirewallSession struct {
	ID              uuid.UUID `json:"id" format:"uuid"`
	WorkspaceID     uuid.UUID `json:"workspace_id" format:"uuid"`
	OwnerID         uuid.UUID `json:"owner_id" format:"uuid"`
	ConfinedProcess string    `json:"confined_process"`
	StartedAt       time.Time `json:"started_at" format:"date-time"`
}

// AgentFirewallSessionByID returns an agent firewall session by its ID.
func (c *Client) AgentFirewallSessionByID(ctx context.Context, id uuid.UUID) (AgentFirewallSession, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/agent-firewall/sessions/%s", id), nil)
	if err != nil {
		return AgentFirewallSession{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentFirewallSession{}, ReadBodyAsError(res)
	}
	var session AgentFirewallSession
	return session, json.NewDecoder(res.Body).Decode(&session)
}
