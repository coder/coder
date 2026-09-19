package codersdk

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

const (
	// ExitNodeTokenHeader authenticates an exit node to coderd. The value is
	// "<exit node ID>:<secret>", mirroring workspace proxy tokens.
	ExitNodeTokenHeader = "Coder-Exit-Node-Token" //nolint:gosec // Header name, not a credential.

	// ExitNodeTailnetPort is the port an exit node listens on inside the
	// tailnet for HTTP CONNECT requests from workspace agents.
	ExitNodeTailnetPort = 3128

	// ExitNodeProtocolHeader selects what a CONNECT stream carries. Absent or
	// "tcp" means a raw TCP tunnel to the target. "udp" means a stream of
	// length-prefixed datagrams relayed to one UDP peer. "dns" means a
	// stream of length-prefixed DNS messages resolved by the exit node.
	ExitNodeProtocolHeader = "X-Coder-Protocol"
	// ExitNodeOriginalHostHeader carries the hostname the workspace dialed
	// when the CONNECT target is an IP literal.
	ExitNodeOriginalHostHeader = "X-Coder-Original-Host"
	// ExitNodeDenyReasonHeader explains a 403 CONNECT response.
	ExitNodeDenyReasonHeader = "X-Coder-Deny-Reason"
	// ExitNodeDenyRuleHeader names the policy rule behind a 403 CONNECT
	// response, when one matched.
	ExitNodeDenyRuleHeader = "X-Coder-Deny-Rule"
)

// ExitNodeProtocol is the transport of a flow observed by an exit node.
type ExitNodeProtocol string

const (
	ExitNodeProtocolTCP ExitNodeProtocol = "tcp"
	ExitNodeProtocolUDP ExitNodeProtocol = "udp"
	// ExitNodeProtocolDNS is a DNS query resolved by the exit node on behalf
	// of the workspace. Host carries the query name and Reason the type.
	ExitNodeProtocolDNS ExitNodeProtocol = "dns"
)

// ExitNode is the admin-created unit that terminates workspace egress. One or
// more replicas, processes started with the exit node's token, do the work.
type ExitNode struct {
	ID             uuid.UUID `json:"id" format:"uuid" table:"id"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid" table:"organization id"`
	Name           string    `json:"name" table:"name,default_sort"`
	DisplayName    string    `json:"display_name" table:"display name"`
	CreatedAt      time.Time `json:"created_at" format:"date-time" table:"created at"`
	UpdatedAt      time.Time `json:"updated_at" format:"date-time" table:"updated at"`
	// Status summarizes replica liveness.
	Status ExitNodeStatus `json:"status" enums:"healthy,unreachable,unregistered" table:"status"`
	// PolicyMismatch is set when live replicas report different policy
	// hashes, meaning the node does not enforce one consistent policy.
	PolicyMismatch bool `json:"policy_mismatch" table:"policy mismatch"`
	// Replicas lists every replica that has ever registered, including
	// stale and stopped ones, newest last.
	Replicas []ExitNodeReplica `json:"replicas" table:"-"`
}

// ExitNodeStatus is the liveness summary of an exit node.
type ExitNodeStatus string

const (
	// ExitNodeStatusHealthy means at least one replica heartbeated recently.
	ExitNodeStatusHealthy ExitNodeStatus = "healthy"
	// ExitNodeStatusUnreachable means replicas exist but none heartbeated
	// within ExitNodeReplicaStaleAfter.
	ExitNodeStatusUnreachable ExitNodeStatus = "unreachable"
	// ExitNodeStatusUnregistered means no replica has ever registered.
	ExitNodeStatusUnregistered ExitNodeStatus = "unregistered"
)

// ExitNodeReplicaStaleAfter is how long after its last heartbeat a replica
// stops counting as live. Replicas register every 5 seconds.
const ExitNodeReplicaStaleAfter = 15 * time.Second

// ExitNodeReplicasPubsubChannel carries the ID of an exit node whose live
// replica set changed, so bound agents can be sent a fresh egress config.
const ExitNodeReplicasPubsubChannel = "exit_node_replicas"

// ExitNodeReplicaStatus is the liveness of one replica.
type ExitNodeReplicaStatus string

const (
	ExitNodeReplicaStatusLive    ExitNodeReplicaStatus = "live"
	ExitNodeReplicaStatusStale   ExitNodeReplicaStatus = "stale"
	ExitNodeReplicaStatusStopped ExitNodeReplicaStatus = "stopped"
)

// ExitNodeReplica is one running exit node process. Its ID is also its
// tailnet peer ID, so agents derive its address from the ID alone.
type ExitNodeReplica struct {
	ID         uuid.UUID `json:"id" format:"uuid" table:"id"`
	ExitNodeID uuid.UUID `json:"exit_node_id" format:"uuid" table:"exit node id"`
	Hostname   string    `json:"hostname" table:"hostname,default_sort"`
	Version    string    `json:"version" table:"version"`
	// WireguardEndpoints are the public ip:port pairs agents may use for
	// direct WireGuard connections. Agents exempt them from enforcement.
	WireguardEndpoints []string `json:"wireguard_endpoints" table:"wireguard endpoints"`
	// PolicyHash identifies the policy the replica enforces.
	PolicyHash string `json:"policy_hash" table:"policy hash"`
	// TailnetAddress is the deterministic tailnet IP derived from ID.
	TailnetAddress string                `json:"tailnet_address" table:"tailnet address"`
	Status         ExitNodeReplicaStatus `json:"status" enums:"live,stale,stopped" table:"status"`
	StartedAt      time.Time             `json:"started_at" format:"date-time" table:"started at"`
	UpdatedAt      time.Time             `json:"updated_at" format:"date-time" table:"updated at"`
	StoppedAt      *time.Time            `json:"stopped_at,omitempty" format:"date-time" table:"stopped at"`
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

// RegisterExitNodeRequest is sent by a replica every 5 seconds. It is the
// replica's heartbeat.
type RegisterExitNodeRequest struct {
	// ReplicaID is generated once per process start and doubles as the
	// replica's tailnet peer ID. Required.
	ReplicaID          uuid.UUID `json:"replica_id" format:"uuid"`
	Version            string    `json:"version"`
	Hostname           string    `json:"hostname"`
	WireguardEndpoints []string  `json:"wireguard_endpoints"`
	// PolicyHash identifies the policy this replica enforces so coderd can
	// flag replicas of one exit node that disagree.
	PolicyHash string `json:"policy_hash"`
}

type RegisterExitNodeResponse struct {
	DERPMap             *tailcfg.DERPMap `json:"derp_map"`
	DERPForceWebSockets bool             `json:"derp_force_websockets"`
	// AgentIDs are the workspace agents this exit node must open tunnels
	// to. Coderd computes the set from templates bound to the exit node.
	AgentIDs []uuid.UUID `json:"agent_ids" format:"uuid"`
	// SiblingReplicas are the other live replicas of the same exit node.
	SiblingReplicas []ExitNodeReplica `json:"sibling_replicas"`
}

// DeregisterExitNodeRequest marks a replica stopped. A stopped replica may
// not register again; a restarted process uses a new ReplicaID.
type DeregisterExitNodeRequest struct {
	ReplicaID uuid.UUID `json:"replica_id" format:"uuid"`
}

// ExitNodeCoordinateReplicaIDParam is the query parameter naming the replica
// on the coordinate endpoint. The replica must be live.
const ExitNodeCoordinateReplicaIDParam = "replica_id"

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
	FlowID  uuid.UUID `json:"flow_id" format:"uuid"`
	AgentID uuid.UUID `json:"agent_id" format:"uuid"`
	// Protocol defaults to tcp when empty.
	Protocol        ExitNodeProtocol `json:"protocol,omitempty"`
	DestinationIP   string           `json:"destination_ip"`
	DestinationPort int              `json:"destination_port"`
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
	// DroppedReports is the number of reports lost since the previous
	// successful batch.
	DroppedReports int `json:"dropped_reports,omitempty"`
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
	return resp, ReadBodyAsJSON(res, &resp)
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
	return nodes, ReadBodyAsJSON(res, &nodes)
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
	return node, ReadBodyAsJSON(res, &node)
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
