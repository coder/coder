package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

const mcpGatewayEscalationsPath = "/api/v2/mcp-gateway/escalations"

// MCPGatewayEscalationStatus is the resolution state of an MCP gateway
// escalation.
type MCPGatewayEscalationStatus string

const (
	MCPGatewayEscalationStatusPending  MCPGatewayEscalationStatus = "pending"
	MCPGatewayEscalationStatusApproved MCPGatewayEscalationStatus = "approved"
	MCPGatewayEscalationStatusDenied   MCPGatewayEscalationStatus = "denied"
	MCPGatewayEscalationStatusExpired  MCPGatewayEscalationStatus = "expired"
)

// MCPGatewayEscalation is an MCP tool call awaiting or recording sponsor
// approval.
type MCPGatewayEscalation struct {
	ID            uuid.UUID                  `json:"id" format:"uuid"`
	ServerSlug    string                     `json:"server_slug"`
	Tool          string                     `json:"tool"`
	Input         string                     `json:"input"`
	WorkspaceName string                     `json:"workspace_name"`
	Status        MCPGatewayEscalationStatus `json:"status"`
	CreatedAt     time.Time                  `json:"created_at" format:"date-time"`
	ExpiresAt     time.Time                  `json:"expires_at" format:"date-time"`
}

// MCPGatewayEscalations lists escalations sponsored by the authenticated user.
// An empty status includes escalations in every state.
func (c *Client) MCPGatewayEscalations(ctx context.Context, status MCPGatewayEscalationStatus) ([]MCPGatewayEscalation, error) {
	reqPath := mcpGatewayEscalationsPath
	if status != "" {
		query := url.Values{}
		query.Set("status", string(status))
		reqPath += "?" + query.Encode()
	}

	res, err := c.Request(ctx, http.MethodGet, reqPath, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var escalations []MCPGatewayEscalation
	return escalations, ReadBodyAsJSON(res, &escalations)
}

// ApproveMCPGatewayEscalation approves a pending MCP gateway escalation.
func (c *Client) ApproveMCPGatewayEscalation(ctx context.Context, id uuid.UUID) error {
	return c.resolveMCPGatewayEscalation(ctx, id, "approve")
}

// DenyMCPGatewayEscalation denies a pending MCP gateway escalation.
func (c *Client) DenyMCPGatewayEscalation(ctx context.Context, id uuid.UUID) error {
	return c.resolveMCPGatewayEscalation(ctx, id, "deny")
}

func (c *Client) resolveMCPGatewayEscalation(ctx context.Context, id uuid.UUID, action string) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("%s/%s/%s", mcpGatewayEscalationsPath, id, action), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
