package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// MCPServerProposalStatus is the lifecycle state of an MCP server
// proposal.
type MCPServerProposalStatus string

const (
	// MCPServerProposalStatusPending means the proposal awaits review.
	MCPServerProposalStatusPending MCPServerProposalStatus = "pending"
	// MCPServerProposalStatusAccepted means the proposal was accepted
	// and the MCP server config was created.
	MCPServerProposalStatusAccepted MCPServerProposalStatus = "accepted"
	// MCPServerProposalStatusRejected means the proposal was rejected
	// or canceled; nothing was created.
	MCPServerProposalStatusRejected MCPServerProposalStatus = "rejected"
)

// MCPServerProposal is a chat-initiated proposal to create a personal
// MCP server. Secrets from the proposed config are never returned;
// has_* booleans report which auth material was provided.
type MCPServerProposal struct {
	ID        uuid.UUID               `json:"id" format:"uuid"`
	ChatID    uuid.UUID               `json:"chat_id" format:"uuid"`
	Status    MCPServerProposalStatus `json:"status"`
	CreatedAt time.Time               `json:"created_at" format:"date-time"`

	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	IconURL     string `json:"icon_url,omitempty"`
	URL         string `json:"url"`
	Transport   string `json:"transport"`
	AuthType    string `json:"auth_type"`

	ToolAllowList []string `json:"tool_allow_list,omitempty"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty"`

	// Which auth material the proposal carries. The values themselves
	// are never returned.
	HasOAuth2ClientCredentials bool `json:"has_oauth2_client_credentials"`
	HasAPIKey                  bool `json:"has_api_key"`
	HasCustomHeaders           bool `json:"has_custom_headers"`

	// CreateDisabled reports that the server would be created in a
	// disabled state.
	CreateDisabled bool `json:"create_disabled,omitempty"`

	// Populated once the proposal is accepted.
	MCPServerConfigID uuid.UUID `json:"mcp_server_config_id,omitzero" format:"uuid"`
	Authenticated     bool      `json:"authenticated"`
	ConnectURL        string    `json:"connect_url,omitempty"`
}

// AcceptMCPServerProposalResponse is returned by the proposal accept
// endpoint. When ConnectURL is set and Authenticated is false, the
// frontend redirects to ConnectURL to finish OAuth2 authentication.
type AcceptMCPServerProposalResponse struct {
	MCPServerConfigID uuid.UUID `json:"mcp_server_config_id" format:"uuid"`
	Authenticated     bool      `json:"authenticated"`
	ConnectURL        string    `json:"connect_url,omitempty"`
}

// MCPServerProposal returns an MCP server proposal by ID.
func (c *Client) MCPServerProposal(ctx context.Context, id uuid.UUID) (MCPServerProposal, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/mcp-server-proposals/%s", id), nil)
	if err != nil {
		return MCPServerProposal{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerProposal{}, ReadBodyAsError(res)
	}
	var proposal MCPServerProposal
	return proposal, json.NewDecoder(res.Body).Decode(&proposal)
}

// AcceptMCPServerProposal accepts an MCP server proposal, creating the
// personal MCP server and enabling it for the proposing chat. The call
// is idempotent: repeating it returns the same config.
func (c *Client) AcceptMCPServerProposal(ctx context.Context, id uuid.UUID) (AcceptMCPServerProposalResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/mcp-server-proposals/%s/accept", id), nil)
	if err != nil {
		return AcceptMCPServerProposalResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AcceptMCPServerProposalResponse{}, ReadBodyAsError(res)
	}
	var resp AcceptMCPServerProposalResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// RejectMCPServerProposal rejects a pending MCP server proposal.
func (c *Client) RejectMCPServerProposal(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/mcp-server-proposals/%s/reject", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
