package chattool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// MCPDiscoveryWaitTimeout bounds how long chats wait for an agent
	// run's workspace MCP discovery, measured from the run's
	// workspace_agents.mcp_discovery_started_at. The anchor lives in the
	// database, so every step, turn, chat, lifecycle tool, and replica
	// shares the same limit for a run.
	MCPDiscoveryWaitTimeout  = 15 * time.Second
	mcpDiscoveryPollInterval = 500 * time.Millisecond
)

// MCPDiscoveryOutcome summarizes workspace MCP discovery for a tool result
// so the model can explain an incomplete initialization. It describes
// discovery only, never whether a later invocation will succeed.
type MCPDiscoveryOutcome struct {
	Phase codersdk.ChatContextMCPDiscoveryPhase `json:"phase"`
	// Stale reports that the latest snapshot belongs to a previous agent
	// process; its tools are withheld.
	Stale bool `json:"stale,omitempty"`
	// WaitTimedOut reports that the run's discovery wait limit passed
	// before the current process reported complete discovery.
	WaitTimedOut  bool `json:"wait_timed_out,omitempty"`
	ServersOK     int  `json:"servers_ok"`
	ServersEmpty  int  `json:"servers_empty"`
	ServersFailed int  `json:"servers_failed"`
	ConfigErrors  int  `json:"config_errors"`
}

// MCPDiscoveryWaiter waits for the agent's workspace MCP discovery and
// reports the outcome.
type MCPDiscoveryWaiter func(ctx context.Context, agentID uuid.UUID) MCPDiscoveryOutcome

// WaitForMCPDiscovery polls the agent's latest context snapshot until it
// reports complete discovery for the current agent process, or until the
// run's limit (MCPDiscoveryWaitTimeout after mcp_discovery_started_at)
// passes. It fails open: a database error, a legacy agent with no
// completeness guarantee, startup scripts still running, a passed limit,
// and cancellation all end the wait, and the caller continues with
// whatever the pinned rows hold. A snapshot from a previous process (run
// id mismatch) counts as no snapshot: only a current-process snapshot can
// establish completeness.
func WaitForMCPDiscovery(ctx context.Context, db database.Store, agentID uuid.UUID) MCPDiscoveryOutcome {
	outcome := MCPDiscoveryOutcome{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
	for {
		// Snapshot first, agent row second: a restart between the two
		// reads then shows up as a mismatch, whereas the reverse order
		// could pair the previous process's row with its own complete
		// snapshot and accept it as current.
		snap, snapErr := db.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
		if snapErr != nil && !errors.Is(snapErr, sql.ErrNoRows) {
			return mcpDiscoveryReadFailed(ctx, outcome)
		}
		agent, err := db.GetWorkspaceAgentByID(ctx, agentID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return mcpDiscoveryReadFailed(ctx, outcome)
		}

		outcome = MCPDiscoveryOutcome{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
		if snapErr == nil {
			// Same rule as chatd's workspace MCP view: a one-sided id
			// also names a previous process; only two empty ids are
			// indeterminate.
			outcome.Stale = agent.AgentRunID != snap.AgentRunID
			outcome.Phase = phaseFromSnapshot(snap)
		}
		current := snapErr == nil && !outcome.Stale
		switch {
		case agent.AgentRunID == "":
			// A legacy agent reports no run, so nothing can establish
			// current-run completeness.
			return summarizeMCPDiscovery(ctx, db, agentID, outcome)
		case current && outcome.Phase != codersdk.ChatContextMCPDiscoveryPhasePending:
			// Complete, or unspecified: no completeness guarantee from
			// this process, so there is nothing to wait for.
			return summarizeMCPDiscovery(ctx, db, agentID, outcome)
		case !agent.McpDiscoveryStartedAt.Valid:
			// Discovery starts when startup scripts finish; until then
			// (or for a row from before the anchor existed) there is no
			// limit to wait against.
			return summarizeMCPDiscovery(ctx, db, agentID, outcome)
		}

		remaining := time.Until(agent.McpDiscoveryStartedAt.Time.Add(MCPDiscoveryWaitTimeout))
		if remaining <= 0 {
			outcome.WaitTimedOut = true
			return summarizeMCPDiscovery(ctx, db, agentID, outcome)
		}
		timer := time.NewTimer(min(mcpDiscoveryPollInterval, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			return outcome
		case <-timer.C:
		}
	}
}

// mcpDiscoveryReadFailed ends the wait on a read error. Cancellation keeps
// the outcome observed so far; any other error fails open as unknown.
func mcpDiscoveryReadFailed(ctx context.Context, outcome MCPDiscoveryOutcome) MCPDiscoveryOutcome {
	if ctx.Err() != nil {
		return outcome
	}
	return MCPDiscoveryOutcome{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
}

func phaseFromSnapshot(snap database.WorkspaceAgentContextSnapshot) codersdk.ChatContextMCPDiscoveryPhase {
	switch snap.McpDiscoveryPhase {
	case database.WorkspaceAgentMcpDiscoveryPhasePending:
		return codersdk.ChatContextMCPDiscoveryPhasePending
	case database.WorkspaceAgentMcpDiscoveryPhaseComplete:
		return codersdk.ChatContextMCPDiscoveryPhaseComplete
	default:
		return codersdk.ChatContextMCPDiscoveryPhaseUnknown
	}
}

// summarizeMCPDiscovery counts the agent's MCP rows into the outcome. A
// read failure leaves the counts at zero; the phase still tells the model
// what happened.
func summarizeMCPDiscovery(ctx context.Context, db database.Store, agentID uuid.UUID, outcome MCPDiscoveryOutcome) MCPDiscoveryOutcome {
	if ctx.Err() != nil {
		return outcome
	}
	rows, err := db.ListWorkspaceAgentContextResources(ctx, agentID)
	if err != nil {
		return outcome
	}
	for _, r := range rows {
		switch r.BodyKind {
		case database.WorkspaceAgentContextBodyKindMcpServer:
			switch {
			case r.Status != database.WorkspaceAgentContextResourceStatusOk:
				outcome.ServersFailed++
			case mcpServerBodyToolCount(r.Body) == 0:
				outcome.ServersEmpty++
			default:
				outcome.ServersOK++
			}
		case database.WorkspaceAgentContextBodyKindMcpConfig:
			if r.Status != database.WorkspaceAgentContextResourceStatusOk {
				outcome.ConfigErrors++
			}
		default:
		}
	}
	return outcome
}

// mcpServerBodyToolCount reads the tool count from a protojson mcp_server
// body without depending on the proto package.
func mcpServerBodyToolCount(body json.RawMessage) int {
	var decoded struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return 0
	}
	return len(decoded.Tools)
}
