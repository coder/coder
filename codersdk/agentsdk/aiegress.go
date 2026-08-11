package agentsdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// AIEgressPolicy fetches the materialized AI egress policy for this
// agent's template. The response includes coderd-injected implicit allow
// rules for the control plane in addition to admin-authored rules. It is
// intended for the confinement supervisor's fork-time bootstrap; a fetch
// failure must result in deny-all, never in unconfined execution.
func (c *Client) AIEgressPolicy(ctx context.Context) (codersdk.AIEgressPolicy, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/ai-egress-policy", nil)
	if err != nil {
		return codersdk.AIEgressPolicy{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.AIEgressPolicy{}, codersdk.ReadBodyAsError(res)
	}
	var policy codersdk.AIEgressPolicy
	return policy, codersdk.ReadBodyAsJSON(res, &policy)
}

// WatchAIEgressPolicy opens an SSE stream of materialized AI egress
// policy revisions for this agent's template. The server sends the
// current policy immediately, then a complete replacement policy after
// every revision write. The returned closer terminates the stream; the
// channel closes when the stream ends for any reason, after which the
// caller should re-establish the watch with backoff and treat prolonged
// failure as reason to keep the last applied policy (never to widen).
func (c *Client) WatchAIEgressPolicy(ctx context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error) {
	watchURL, err := c.SDK.URL.Parse("/api/v2/workspaceagents/me/ai-egress-policy/watch")
	if err != nil {
		return nil, nil, xerrors.Errorf("parse url: %w", err)
	}

	httpClient := &http.Client{
		Transport: c.SDK.HTTPClient.Transport,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, watchURL.String(), nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("build request: %w", err)
	}
	req.Header[codersdk.SessionTokenHeader] = []string{c.SDK.SessionToken()}

	// The response body is the long-lived SSE stream: it is returned to
	// the caller as the closer and closed by the reader goroutine on
	// stream end.
	//nolint:bodyclose
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, xerrors.Errorf("execute request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, nil, codersdk.ReadBodyAsError(res)
	}

	policies := make(chan codersdk.AIEgressPolicy)
	go func() {
		defer close(policies)
		defer res.Body.Close()
		nextEvent := codersdk.ServerSentEventReader(ctx, res.Body)
		for {
			sse, err := nextEvent()
			if err != nil {
				return
			}
			switch sse.Type {
			case codersdk.ServerSentEventTypePing:
				continue
			case codersdk.ServerSentEventTypeData:
			default:
				return
			}
			b, ok := sse.Data.([]byte)
			if !ok {
				return
			}
			var policy codersdk.AIEgressPolicy
			if err := json.Unmarshal(b, &policy); err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case policies <- policy:
			}
		}
	}()
	return policies, res.Body, nil
}

// AISandboxNetworkProtocol identifies how the egress proxy observed a
// connection attempt.
type AISandboxNetworkProtocol = codersdk.AISandboxNetworkProtocol

const (
	AISandboxNetworkProtocolConnect = codersdk.AISandboxNetworkProtocolConnect
	AISandboxNetworkProtocolHTTP    = codersdk.AISandboxNetworkProtocolHTTP
	AISandboxNetworkProtocolSNI     = codersdk.AISandboxNetworkProtocolSNI
	AISandboxNetworkProtocolTCP     = codersdk.AISandboxNetworkProtocolTCP
)

// AISandboxNetworkEventAction is the proxy's policy decision for one
// observed connection attempt.
type AISandboxNetworkEventAction = codersdk.AISandboxNetworkEventAction

const (
	AISandboxNetworkEventActionAllowed = codersdk.AISandboxNetworkEventActionAllowed
	AISandboxNetworkEventActionDenied  = codersdk.AISandboxNetworkEventActionDenied
)

// PostAISandboxSessionRequest opens (or idempotently re-asserts) a
// confinement session. The supervisor generates ID so events can
// reference the session before the create round-trip completes and so
// retries are safe. ChildAgentID is empty when the caller itself is the
// confined, AI-bound agent (the AI-designated workspace shape); for a
// sandboxed child inside a normal workspace, the unbound parent agent
// sets ChildAgentID to the bound child's workspace agent ID and coderd
// verifies the parent-child relationship. Attribution (AI agent and
// sponsor UUIDs) is always resolved and snapshotted server-side from the
// binding; the caller cannot assert it.
//
// Re-POSTing the same ID with EndedAt set closes the session.
type PostAISandboxSessionRequest struct {
	ID                uuid.UUID                           `json:"id" format:"uuid"`
	ChildAgentID      uuid.UUID                           `json:"child_agent_id,omitempty" format:"uuid"`
	EgressEnforcement codersdk.AISandboxEgressEnforcement `json:"egress_enforcement"`
	StartedAt         time.Time                           `json:"started_at" format:"date-time"`
	EndedAt           *time.Time                          `json:"ended_at,omitempty" format:"date-time"`
}

// PostAISandboxSession creates or updates a confinement session record.
func (c *Client) PostAISandboxSession(ctx context.Context, req PostAISandboxSessionRequest) error {
	res, err := c.SDK.Request(ctx, http.MethodPost, "/api/v2/workspaceagents/me/ai-sandbox-sessions", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}

// AISandboxNetworkEvent is one egress policy decision observed by the
// supervisor-owned proxy. Only the proxy owner (the supervisor or the
// sandbox parent agent) emits these; the confined child never does.
type AISandboxNetworkEvent struct {
	SessionID  uuid.UUID                   `json:"session_id" format:"uuid"`
	OccurredAt time.Time                   `json:"occurred_at" format:"date-time"`
	Protocol   AISandboxNetworkProtocol    `json:"protocol"`
	Host       string                      `json:"host"`
	Port       int                         `json:"port"`
	Action     AISandboxNetworkEventAction `json:"action"`
	// PolicyRevision is the egress policy revision that produced this
	// decision, or 0 while running the bootstrap deny-all fallback.
	PolicyRevision int64 `json:"policy_revision"`
}

// PatchAISandboxNetworkEventsRequest batches proxy decisions for
// insertion into the retained egress audit stream.
type PatchAISandboxNetworkEventsRequest struct {
	Events []AISandboxNetworkEvent `json:"events"`
}

// PatchAISandboxNetworkEvents reports a batch of egress policy decisions.
func (c *Client) PatchAISandboxNetworkEvents(ctx context.Context, req PatchAISandboxNetworkEventsRequest) error {
	res, err := c.SDK.Request(ctx, http.MethodPatch, "/api/v2/workspaceagents/me/ai-sandbox-network-events", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.ReadBodyAsError(res)
	}
	return nil
}
