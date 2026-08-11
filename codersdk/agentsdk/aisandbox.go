package agentsdk

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// CreateAISandboxRequest asks coderd to create (or reconcile to) a sandbox
// owned by the calling parent workspace agent. Name is the declaration key:
// re-posting the same name after a parent restart returns the existing
// sandbox and child agent rather than creating a duplicate.
//
// The platform owns everything security relevant here. The caller cannot
// choose the child's AI identity, its binding, or its credentials: coderd
// resolves the identity from the parent's own binding (AI-requested
// sandbox, reusing the requester's identity) or from the workspace-origin
// identity (human-declared sandbox), per identity continuity.
type CreateAISandboxRequest struct {
	Name string `json:"name"`
	// EgressEnforcement is the admin attestation declared for this
	// sandbox. It is recorded, never verified.
	EgressEnforcement codersdk.AISandboxEgressEnforcement `json:"egress_enforcement"`
}

// CreateAISandboxResponse carries the platform-minted material the create
// script needs. AgentToken authenticates the confined child agent;
// SessionToken is the scoped AI session token for CLI use inside the
// sandbox. Both belong to the same AI identity.
//
// The session token is returned only here: it cannot be recovered later, so
// a reconciling parent receives a freshly rotated one.
type CreateAISandboxResponse struct {
	ID           uuid.UUID `json:"id" format:"uuid"`
	ChildAgentID uuid.UUID `json:"child_agent_id" format:"uuid"`
	AIAgentID    uuid.UUID `json:"ai_agent_id" format:"uuid"`
	AgentToken   string    `json:"agent_token"`
	SessionToken string    `json:"session_token"`
	// Reconciled reports that an existing sandbox record was reused
	// instead of a new one being created.
	Reconciled bool `json:"reconciled"`
}

// CreateAISandbox creates or reconciles the named sandbox for this agent.
func (c *Client) CreateAISandbox(ctx context.Context, req CreateAISandboxRequest) (CreateAISandboxResponse, error) {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/ai-sandboxes", req)
	if err != nil {
		return CreateAISandboxResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return CreateAISandboxResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp CreateAISandboxResponse
	return resp, codersdk.ReadBodyAsJSON(res, &resp)
}

// AISandbox describes a sandbox owned by the calling parent agent.
type AISandbox struct {
	ID                uuid.UUID                           `json:"id" format:"uuid"`
	ChildAgentID      uuid.UUID                           `json:"child_agent_id" format:"uuid"`
	AIAgentID         uuid.UUID                           `json:"ai_agent_id" format:"uuid"`
	Name              string                              `json:"name"`
	EgressEnforcement codersdk.AISandboxEgressEnforcement `json:"egress_enforcement"`
}

// AISandboxes lists the calling parent agent's live sandboxes. A restarting
// parent uses this to discover sandboxes it must reconcile or destroy.
func (c *Client) AISandboxes(ctx context.Context) ([]AISandbox, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/ai-sandboxes", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	var sandboxes []AISandbox
	return sandboxes, codersdk.ReadBodyAsJSON(res, &sandboxes)
}

// DeleteAISandbox tears down a sandbox record: coderd soft-deletes the child
// agent row and revokes its keys, so the child can no longer authenticate.
// The caller runs its destroy script separately.
func (c *Client) DeleteAISandbox(ctx context.Context, id uuid.UUID) error {
	res, err := c.SDK.Request(ctx, http.MethodDelete, "/api/v2/workspaceagents/me/ai-sandboxes/"+id.String(), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}
