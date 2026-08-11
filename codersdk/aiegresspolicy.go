package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// AIEgressRule permits outbound traffic to a single host pattern. The
// policy is default-deny: traffic matching no rule is refused. Host is an
// exact hostname ("github.com") or a single-label wildcard
// ("*.github.com"); IP literals are matched exactly. An empty Ports list
// permits 80 and 443 only.
type AIEgressRule struct {
	Host  string `json:"host"`
	Ports []int  `json:"ports,omitempty"`
}

// AIEgressPolicy is the versioned, template-level egress allow list for
// AI-confined execution. It is stored per template and edited through the
// API without a template push or workspace rebuild. Revision increases
// monotonically with every write; revision 0 with no rules is the
// implicit default for templates that have never stored a policy and
// means deny-all beyond the platform's implicit allows.
//
// The agent-facing read of this object is materialized: coderd injects
// implicit allow rules for the control plane (the deployment access URL
// and any configured AI gateway) so the confined child can always reach
// coderd. Template admins do not need to (and cannot) remove those.
type AIEgressPolicy struct {
	TemplateID uuid.UUID      `json:"template_id" format:"uuid"`
	Revision   int64          `json:"revision"`
	Rules      []AIEgressRule `json:"rules"`
	UpdatedAt  time.Time      `json:"updated_at" format:"date-time"`
	// UpdatedBy is the coderd actor that wrote this revision. It is the
	// nil UUID for the implicit revision-0 default.
	UpdatedBy uuid.UUID `json:"updated_by" format:"uuid"`
}

// UpdateAIEgressPolicyRequest replaces the template's egress allow list,
// producing a new revision. Writes are admin-only and audited.
type UpdateAIEgressPolicyRequest struct {
	Rules []AIEgressRule `json:"rules"`
}

// TemplateAIEgressPolicy returns the current AI egress policy revision
// for a template. Templates with no stored policy return the revision-0
// deny-all default rather than a 404.
func (c *Client) TemplateAIEgressPolicy(ctx context.Context, templateID uuid.UUID) (AIEgressPolicy, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/ai-egress-policy", templateID), nil)
	if err != nil {
		return AIEgressPolicy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIEgressPolicy{}, ReadBodyAsError(res)
	}
	var policy AIEgressPolicy
	return policy, ReadBodyAsJSON(res, &policy)
}

// UpdateTemplateAIEgressPolicy stores a new revision of the template's AI
// egress policy and returns it.
func (c *Client) UpdateTemplateAIEgressPolicy(ctx context.Context, templateID uuid.UUID, req UpdateAIEgressPolicyRequest) (AIEgressPolicy, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/templates/%s/ai-egress-policy", templateID), req)
	if err != nil {
		return AIEgressPolicy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIEgressPolicy{}, ReadBodyAsError(res)
	}
	var policy AIEgressPolicy
	return policy, ReadBodyAsJSON(res, &policy)
}

// AISandboxEgressEnforcement is an admin attestation of how thoroughly a
// confinement mechanism routes traffic through the platform egress proxy.
// It is recorded, not verified: the platform cannot prove an arbitrary
// admin script's routing coverage at declaration time.
type AISandboxEgressEnforcement string

const (
	// AISandboxEgressEnforcementForced attests that all egress from the
	// confined execution structurally routes through the platform proxy
	// (for example a network namespace or internal-only container
	// network with no other route out).
	AISandboxEgressEnforcementForced AISandboxEgressEnforcement = "forced"
	// AISandboxEgressEnforcementAdvisory attests that egress is routed
	// through the proxy only cooperatively (for example proxy
	// environment variables that a process may ignore).
	AISandboxEgressEnforcementAdvisory AISandboxEgressEnforcement = "advisory"
	// AISandboxEgressEnforcementNone attests no egress routing at all.
	AISandboxEgressEnforcementNone AISandboxEgressEnforcement = "none"
)

// AISandboxNetworkProtocol identifies how the egress proxy observed a
// connection attempt.
type AISandboxNetworkProtocol string

const (
	// AISandboxNetworkProtocolConnect is an HTTP CONNECT request to the
	// proxy, with the hostname supplied by the client.
	AISandboxNetworkProtocolConnect AISandboxNetworkProtocol = "connect"
	// AISandboxNetworkProtocolHTTP is a plain absolute-form HTTP request
	// forwarded through the proxy.
	AISandboxNetworkProtocolHTTP AISandboxNetworkProtocol = "http"
	// AISandboxNetworkProtocolSNI is a transparently intercepted TLS
	// connection whose hostname was read from the ClientHello SNI.
	AISandboxNetworkProtocolSNI AISandboxNetworkProtocol = "sni"
	// AISandboxNetworkProtocolTCP is a transparently intercepted
	// connection with no readable hostname, matched by address only.
	AISandboxNetworkProtocolTCP AISandboxNetworkProtocol = "tcp"
)

// AISandboxNetworkEventAction is the proxy's policy decision for one
// observed connection attempt.
type AISandboxNetworkEventAction string

const (
	AISandboxNetworkEventActionAllowed AISandboxNetworkEventAction = "allowed"
	AISandboxNetworkEventActionDenied  AISandboxNetworkEventAction = "denied"
)

// AISandboxSession describes a retained AI confinement session.
type AISandboxSession struct {
	ID                uuid.UUID                  `json:"id" format:"uuid"`
	WorkspaceID       uuid.UUID                  `json:"workspace_id" format:"uuid"`
	ReporterAgentID   uuid.UUID                  `json:"reporter_agent_id" format:"uuid"`
	ConfinedAgentID   uuid.UUID                  `json:"confined_agent_id" format:"uuid"`
	AIAgentID         uuid.UUID                  `json:"ai_agent_id" format:"uuid"`
	SponsorUserID     uuid.UUID                  `json:"sponsor_user_id" format:"uuid"`
	EgressEnforcement AISandboxEgressEnforcement `json:"egress_enforcement"`
	StartedAt         time.Time                  `json:"started_at" format:"date-time"`
	EndedAt           *time.Time                 `json:"ended_at,omitempty" format:"date-time"`
	CreatedAt         time.Time                  `json:"created_at" format:"date-time"`
}

// AISandboxNetworkEventView is one retained egress policy decision.
type AISandboxNetworkEventView struct {
	SessionID      uuid.UUID                   `json:"session_id" format:"uuid"`
	OccurredAt     time.Time                   `json:"occurred_at" format:"date-time"`
	Protocol       AISandboxNetworkProtocol    `json:"protocol"`
	Host           string                      `json:"host"`
	Port           int                         `json:"port"`
	Action         AISandboxNetworkEventAction `json:"action"`
	PolicyRevision int64                       `json:"policy_revision"`
}

// WorkspaceAISandboxSessions lists retained AI confinement sessions for a
// workspace, ordered by start time from newest to oldest.
func (c *Client) WorkspaceAISandboxSessions(ctx context.Context, workspaceID uuid.UUID) ([]AISandboxSession, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/ai-sandbox-sessions", workspaceID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var sessions []AISandboxSession
	return sessions, ReadBodyAsJSON(res, &sessions)
}

// AISandboxSessionNetworkEvents lists retained egress decisions for a session
// in ascending database ID order. AfterID is an exclusive cursor. Limit values
// of zero use the server default.
func (c *Client) AISandboxSessionNetworkEvents(ctx context.Context, workspaceID, sessionID uuid.UUID, afterID int64, limit int) ([]AISandboxNetworkEventView, error) {
	reqOpts := []RequestOption{func(r *http.Request) {
		query := r.URL.Query()
		if afterID > 0 {
			query.Set("after_id", strconv.FormatInt(afterID, 10))
		}
		if limit > 0 {
			query.Set("limit", strconv.Itoa(limit))
		}
		r.URL.RawQuery = query.Encode()
	}}
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/ai-sandbox-sessions/%s/network-events", workspaceID, sessionID), nil, reqOpts...)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var events []AISandboxNetworkEventView
	return events, ReadBodyAsJSON(res, &events)
}
