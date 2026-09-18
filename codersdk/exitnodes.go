package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

const (
	// ExitNodeTokenHeader authenticates an exit node to coderd. The value is
	// "<exit node ID>:<secret>", mirroring workspace proxy tokens.
	ExitNodeTokenHeader = "Coder-Exit-Node-Token"

	// ExitNodeTailnetPort is the port an exit node listens on inside the
	// tailnet for HTTP CONNECT requests from workspace agents.
	ExitNodeTailnetPort = 3128
)

// ExitNode is a tailnet peer that terminates workspace egress, enforces
// policy, and reports flows back to coderd.
type ExitNode struct {
	ID             uuid.UUID  `json:"id" format:"uuid" table:"id"`
	OrganizationID uuid.UUID  `json:"organization_id" format:"uuid" table:"organization id"`
	Name           string     `json:"name" table:"name,default_sort"`
	DisplayName    string     `json:"display_name" table:"display name"`
	CreatedAt      time.Time  `json:"created_at" format:"date-time" table:"created at"`
	UpdatedAt      time.Time  `json:"updated_at" format:"date-time" table:"updated at"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty" format:"date-time" table:"last seen at"`
	Version        string     `json:"version" table:"version"`
	// WireguardEndpoints are the public ip:port pairs agents may use for
	// direct WireGuard connections. Agents exempt them from enforcement.
	WireguardEndpoints []string `json:"wireguard_endpoints" table:"wireguard endpoints"`
	// TailnetAddress is the deterministic tailnet IP agents dial, derived
	// from the exit node ID.
	TailnetAddress string `json:"tailnet_address" table:"tailnet address"`
}

type CreateExitNodeRequest struct {
	Name        string `json:"name" validate:"required,template_name"`
	DisplayName string `json:"display_name,omitempty"`
}

// CreateExitNodeResponse carries the token exactly once; coderd stores only a
// hash.
type CreateExitNodeResponse struct {
	ExitNode
	Token string `json:"token"`
}

type RegisterExitNodeRequest struct {
	Version            string   `json:"version"`
	Hostname           string   `json:"hostname"`
	WireguardEndpoints []string `json:"wireguard_endpoints"`
}

type RegisterExitNodeResponse struct {
	DERPMap             *tailcfg.DERPMap `json:"derp_map"`
	DERPForceWebSockets bool             `json:"derp_force_websockets"`
	// AgentIDs are the workspace agents this exit node must open tunnels
	// to. Coderd computes the set from templates bound to the exit node.
	AgentIDs []uuid.UUID `json:"agent_ids" format:"uuid"`
}

// ExitNodeFlowDecision is the policy outcome for a single flow.
type ExitNodeFlowDecision string

const (
	ExitNodeFlowAllow ExitNodeFlowDecision = "allow"
	ExitNodeFlowDeny  ExitNodeFlowDecision = "deny"
)

// ExitNodeFlowReport describes one TCP flow observed by an exit node. The
// same FlowID is sent twice for allowed flows: once on connect and once on
// disconnect with byte counts filled in.
type ExitNodeFlowReport struct {
	FlowID          uuid.UUID `json:"flow_id" format:"uuid"`
	AgentID         uuid.UUID `json:"agent_id" format:"uuid"`
	DestinationIP   string    `json:"destination_ip"`
	DestinationPort int       `json:"destination_port"`
	// Host is the hostname learned from TLS SNI or the HTTP Host header, or
	// empty when neither was present.
	Host           string               `json:"host,omitempty"`
	Decision       ExitNodeFlowDecision `json:"decision"`
	RuleID         string               `json:"rule_id,omitempty"`
	Reason         string               `json:"reason,omitempty"`
	BytesIn        int64                `json:"bytes_in"`
	BytesOut       int64                `json:"bytes_out"`
	ConnectTime    time.Time            `json:"connect_time" format:"date-time"`
	DisconnectTime *time.Time           `json:"disconnect_time,omitempty" format:"date-time"`
}

type ReportExitNodeFlowsRequest struct {
	Flows []ExitNodeFlowReport `json:"flows"`
}

// CreateExitNode registers a new exit node in the organization and returns
// its token. The token is not retrievable afterwards.
func (c *Client) CreateExitNode(ctx context.Context, organizationID uuid.UUID, req CreateExitNodeRequest) (CreateExitNodeResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/organizations/"+organizationID.String()+"/exitnodes", req)
	if err != nil {
		return CreateExitNodeResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return CreateExitNodeResponse{}, ReadBodyAsError(res)
	}
	var resp CreateExitNodeResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ExitNodes lists the exit nodes in an organization.
func (c *Client) ExitNodes(ctx context.Context, organizationID uuid.UUID) ([]ExitNode, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/organizations/"+organizationID.String()+"/exitnodes", nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var nodes []ExitNode
	return nodes, json.NewDecoder(res.Body).Decode(&nodes)
}

// ExitNodeByName fetches one exit node by name or ID.
func (c *Client) ExitNodeByName(ctx context.Context, organizationID uuid.UUID, name string) (ExitNode, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/organizations/"+organizationID.String()+"/exitnodes/"+name, nil)
	if err != nil {
		return ExitNode{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ExitNode{}, ReadBodyAsError(res)
	}
	var node ExitNode
	return node, json.NewDecoder(res.Body).Decode(&node)
}

// DeleteExitNode soft-deletes an exit node by name or ID.
func (c *Client) DeleteExitNode(ctx context.Context, organizationID uuid.UUID, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, "/api/v2/organizations/"+organizationID.String()+"/exitnodes/"+name, nil)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
