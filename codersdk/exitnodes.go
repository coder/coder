package codersdk

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

const (
	// ExitNodeTokenHeader authenticates an exit node as "<id>:<secret>".
	ExitNodeTokenHeader = "Coder-Exit-Node-Token" //nolint:gosec // Header name, not a credential.

	// ExitNodeTailnetPort accepts agent HTTP CONNECT requests.
	ExitNodeTailnetPort = 3128

	// ExitNodeProtocolHeader selects the CONNECT stream protocol.
	ExitNodeProtocolHeader = "X-Coder-Protocol"
	// ExitNodeOriginalHostHeader carries the original hostname for IP targets.
	ExitNodeOriginalHostHeader = "X-Coder-Original-Host"
	// ExitNodeDenyReasonHeader explains a 403 CONNECT response.
	ExitNodeDenyReasonHeader = "X-Coder-Deny-Reason"
	// ExitNodeDenyRuleHeader identifies the denying policy rule.
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

// ExitNode is an organization-scoped workspace egress endpoint.
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
	// Replicas includes live, stale, and stopped replicas.
	Replicas []ExitNodeReplica `json:"replicas" table:"-"`
}

// ExitNodeStatus is the liveness summary of an exit node.
type ExitNodeStatus string

const (
	// ExitNodeStatusHealthy means at least one replica heartbeated recently.
	ExitNodeStatusHealthy ExitNodeStatus = "healthy"
	// ExitNodeStatusUnreachable means replicas exist but none are live.
	ExitNodeStatusUnreachable ExitNodeStatus = "unreachable"
	// ExitNodeStatusUnregistered means no replica has ever registered.
	ExitNodeStatusUnregistered ExitNodeStatus = "unregistered"
)

// ExitNodeReplicaStaleAfter is the replica heartbeat timeout.
const ExitNodeReplicaStaleAfter = 15 * time.Second

var exitNodeReplicaPeerNamespace = uuid.MustParse("f76a3687-42ae-487a-978f-c30d73f13e06")

// ExitNodeReplicaPeerID derives a domain-separated peer ID.
func ExitNodeReplicaPeerID(exitNodeID, replicaID uuid.UUID) uuid.UUID {
	name := make([]byte, 0, 2*len(exitNodeID))
	name = append(name, exitNodeID[:]...)
	name = append(name, replicaID[:]...)
	return uuid.NewSHA1(exitNodeReplicaPeerNamespace, name)
}

// ExitNodeReplicasPubsubChannel carries replica and template updates.
const ExitNodeReplicasPubsubChannel = "exit_node_replicas"

const exitNodeTemplatePubsubPrefix = "template:"

// ExitNodeTemplatePubsubPayload identifies a changed template.
func ExitNodeTemplatePubsubPayload(templateID uuid.UUID) []byte {
	return []byte(exitNodeTemplatePubsubPrefix + templateID.String())
}

// ParseExitNodeTemplatePubsubPayload parses a template configuration event.
func ParseExitNodeTemplatePubsubPayload(payload []byte) (uuid.UUID, bool) {
	value, ok := strings.CutPrefix(string(payload), exitNodeTemplatePubsubPrefix)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(value)
	return id, err == nil
}

// ExitNodeReplicaStatus is the liveness of one replica.
type ExitNodeReplicaStatus string

const (
	ExitNodeReplicaStatusLive    ExitNodeReplicaStatus = "live"
	ExitNodeReplicaStatusStale   ExitNodeReplicaStatus = "stale"
	ExitNodeReplicaStatusStopped ExitNodeReplicaStatus = "stopped"
)

// ExitNodeReplica is one exit node process.
type ExitNodeReplica struct {
	ID         uuid.UUID `json:"id" format:"uuid" table:"id"`
	ExitNodeID uuid.UUID `json:"exit_node_id" format:"uuid" table:"exit node id"`
	Hostname   string    `json:"hostname" table:"hostname,default_sort"`
	Version    string    `json:"version" table:"version"`
	// WireguardEndpoints are direct endpoints agents exempt from enforcement.
	WireguardEndpoints []string `json:"wireguard_endpoints" table:"wireguard endpoints"`
	// PolicyHash identifies the policy the replica enforces.
	PolicyHash string `json:"policy_hash" table:"policy hash"`
	// TailnetAddress is derived from the server-assigned peer ID.
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

// CreateExitNodeResponse returns the token exactly once.
type CreateExitNodeResponse struct {
	ExitNode
	Token string `json:"token"`
}

// RegisterExitNodeRequest is a replica heartbeat.
type RegisterExitNodeRequest struct {
	// ReplicaID is generated once per process and is required.
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
	// AgentIDs are workspace agents bound to this exit node.
	AgentIDs []uuid.UUID `json:"agent_ids" format:"uuid"`
	// SiblingReplicas are the other live replicas of the same exit node.
	SiblingReplicas []ExitNodeReplica `json:"sibling_replicas"`
}

// DeregisterExitNodeRequest permanently stops a replica ID.
type DeregisterExitNodeRequest struct {
	ReplicaID uuid.UUID `json:"replica_id" format:"uuid"`
}

// ExitNodeCoordinateReplicaIDParam identifies the coordinating replica.
const ExitNodeCoordinateReplicaIDParam = "replica_id"

// ExitNodeFlowDecision is the policy outcome for a single flow.
type ExitNodeFlowDecision string

const (
	ExitNodeFlowAllow ExitNodeFlowDecision = "allow"
	ExitNodeFlowDeny  ExitNodeFlowDecision = "deny"
)

// ExitNodeFlowReport describes one observed egress flow.
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

func requestExitNode[T any](ctx context.Context, c *Client, method, path string, body any, status int) (T, error) {
	var value T
	res, err := c.Request(ctx, method, path, body)
	if err != nil {
		return value, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		return value, ReadBodyAsError(res)
	}
	return value, ReadBodyAsJSON(res, &value)
}

// CreateExitNode registers a new exit node in the organization and returns
// its token. The token is not retrievable afterwards.
func (c *Client) CreateExitNode(ctx context.Context, organizationID uuid.UUID, req CreateExitNodeRequest) (CreateExitNodeResponse, error) {
	return requestExitNode[CreateExitNodeResponse](ctx, c, http.MethodPost, "/api/v2/organizations/"+organizationID.String()+"/exitnodes", req, http.StatusCreated)
}

// ExitNodes lists the exit nodes in an organization.
func (c *Client) ExitNodes(ctx context.Context, organizationID uuid.UUID) ([]ExitNode, error) {
	return requestExitNode[[]ExitNode](ctx, c, http.MethodGet, "/api/v2/organizations/"+organizationID.String()+"/exitnodes", nil, http.StatusOK)
}

// ExitNodeByName fetches one exit node by name or ID.
func (c *Client) ExitNodeByName(ctx context.Context, organizationID uuid.UUID, name string) (ExitNode, error) {
	return requestExitNode[ExitNode](ctx, c, http.MethodGet, "/api/v2/organizations/"+organizationID.String()+"/exitnodes/"+name, nil, http.StatusOK)
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
