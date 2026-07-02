package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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

// AgentFirewallLog represents a single audit event from an agent firewall proxy.
type AgentFirewallLog struct {
	ID             uuid.UUID  `json:"id" format:"uuid"`
	SessionID      uuid.UUID  `json:"session_id" format:"uuid"`
	SequenceNumber int32      `json:"sequence_number"`
	Allowed        bool       `json:"allowed"`
	CreatedAt      time.Time  `json:"created_at" format:"date-time"`
	Proto          string     `json:"proto"`
	Method         string     `json:"method"`
	Detail         string     `json:"detail"`
	MatchedRule    *string    `json:"matched_rule"`
	CapturedAt     *time.Time `json:"captured_at,omitempty" format:"date-time"`
}

// AgentFirewallSessionLogsResponse is the response for
// GET /api/v2/agent-firewall/sessions/{id}/logs.
type AgentFirewallSessionLogsResponse struct {
	Results []AgentFirewallLog `json:"results"`
}

// AgentFirewallSessionLogsParams are query parameters for listing
// agent firewall session logs.
type AgentFirewallSessionLogsParams struct {
	// SeqAfter is an inclusive lower bound on sequence_number.
	// Only logs with sequence_number >= SeqAfter are returned.
	SeqAfter *int64 `json:"seq_after,omitempty"`
	// SeqBefore is an exclusive upper bound on sequence_number.
	// Only logs with sequence_number < SeqBefore are returned.
	SeqBefore *int64 `json:"seq_before,omitempty"`
	// Limit caps the number of returned rows. Defaults to 100.
	Limit *int32 `json:"limit,omitempty"`
}

func (p AgentFirewallSessionLogsParams) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if p.SeqAfter != nil {
			q.Set("seq_after", strconv.FormatInt(*p.SeqAfter, 10))
		}
		if p.SeqBefore != nil {
			q.Set("seq_before", strconv.FormatInt(*p.SeqBefore, 10))
		}
		if p.Limit != nil {
			q.Set("limit", strconv.FormatInt(int64(*p.Limit), 10))
		}
		r.URL.RawQuery = q.Encode()
	}
}

// AgentFirewallSessionLogs returns agent firewall audit logs for the
// given session, sorted by sequence number ascending.
func (c *Client) AgentFirewallSessionLogs(ctx context.Context, sessionID uuid.UUID, params AgentFirewallSessionLogsParams) (AgentFirewallSessionLogsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/agent-firewall/sessions/%s/logs", sessionID),
		nil,
		params.asRequestOption(),
	)
	if err != nil {
		return AgentFirewallSessionLogsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AgentFirewallSessionLogsResponse{}, ReadBodyAsError(res)
	}

	var resp AgentFirewallSessionLogsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
