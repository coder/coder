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
	// MCPDiscoveryWaitTimeout bounds how long a chat turn or lifecycle tool
	// waits for the current agent process to finish workspace MCP
	// discovery before proceeding with whatever has been discovered.
	MCPDiscoveryWaitTimeout = 15 * time.Second
	// mcpDiscoveryNoSnapshotGrace bounds the wait while the agent has no
	// current-run snapshot at all: Ready precedes the first push by a
	// moment, but a legacy coderd or a disabled context sync never pushes,
	// and that cannot be told apart from a slow first push.
	mcpDiscoveryNoSnapshotGrace = 3 * time.Second
	mcpDiscoveryPollInterval    = 500 * time.Millisecond
)

// MCPDiscoveryOutcome summarizes workspace MCP discovery for a tool result
// so the model can explain an incomplete initialization. It describes
// discovery only, never whether a later invocation will succeed.
type MCPDiscoveryOutcome struct {
	Phase codersdk.ChatContextMCPDiscoveryPhase `json:"phase"`
	// Stale reports that the latest snapshot belongs to a previous agent
	// process; its tools are withheld.
	Stale bool `json:"stale,omitempty"`
	// WaitTimedOut reports that the bounded wait ended before the current
	// process reported complete discovery.
	WaitTimedOut  bool `json:"wait_timed_out,omitempty"`
	ServersOK     int  `json:"servers_ok"`
	ServersEmpty  int  `json:"servers_empty"`
	ServersFailed int  `json:"servers_failed"`
	ConfigErrors  int  `json:"config_errors"`
}

// MCPDiscoveryWaiter waits for the agent's workspace MCP discovery and
// reports the outcome. chatd supplies one that shares a single attempt per
// chat and agent process across preparation steps and lifecycle tools.
type MCPDiscoveryWaiter func(ctx context.Context, agentID uuid.UUID) MCPDiscoveryOutcome

// WaitForMCPDiscovery polls the agent's latest context snapshot until it
// reports complete discovery for the current agent process, or until the
// budget is spent. It fails open: a database error, a legacy agent with no
// completeness guarantee, exhaustion, and cancellation all end the wait,
// and the caller continues with whatever the pinned rows hold. A snapshot
// from a previous process (run id mismatch) counts as no snapshot: only a
// current-process snapshot can establish completeness.
func WaitForMCPDiscovery(ctx context.Context, db database.Store, agentID uuid.UUID) MCPDiscoveryOutcome {
	waitCtx, cancel := context.WithTimeout(ctx, MCPDiscoveryWaitTimeout)
	defer cancel()

	ticker := time.NewTicker(mcpDiscoveryPollInterval)
	defer ticker.Stop()

	noSnapshotDeadline := time.Now().Add(mcpDiscoveryNoSnapshotGrace)
	seenCurrentSnapshot := false
	outcome := MCPDiscoveryOutcome{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
	for {
		var (
			agent database.WorkspaceAgent
			snap  database.WorkspaceAgentContextSnapshot
			err   error
		)
		agent, err = db.GetWorkspaceAgentByID(waitCtx, agentID)
		if err == nil {
			snap, err = db.GetLatestWorkspaceAgentContextSnapshot(waitCtx, agentID)
		}
		switch {
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			// Cancellation counts as a spent attempt so a canceled turn
			// does not rearm the budget; other errors fail open.
			outcome.WaitTimedOut = waitCtx.Err() != nil
			return outcome
		case err != nil:
			// Missing snapshots get bounded grace, not version detection.
			if !seenCurrentSnapshot && time.Now().After(noSnapshotDeadline) {
				outcome.WaitTimedOut = true
				return outcome
			}
		case agent.AgentRunID != snap.AgentRunID:
			// Same rule as chatd's workspace MCP view: a one-sided id
			// also names a previous process; only two empty ids are
			// indeterminate and fall through to the phase cases.
			outcome.Stale = true
			outcome.Phase = phaseFromSnapshot(snap)
			if !seenCurrentSnapshot && time.Now().After(noSnapshotDeadline) {
				outcome.WaitTimedOut = true
				return summarizeMCPDiscovery(waitCtx, db, agentID, outcome)
			}
		case snap.McpDiscoveryPhase == database.WorkspaceAgentMcpDiscoveryPhaseComplete:
			outcome.Stale = false
			outcome.Phase = codersdk.ChatContextMCPDiscoveryPhaseComplete
			return summarizeMCPDiscovery(waitCtx, db, agentID, outcome)
		case snap.McpDiscoveryPhase == database.WorkspaceAgentMcpDiscoveryPhasePending:
			outcome.Stale = false
			outcome.Phase = codersdk.ChatContextMCPDiscoveryPhasePending
			seenCurrentSnapshot = true
		default:
			// No completeness guarantee from this agent; nothing to wait for.
			outcome.Stale = false
			outcome.Phase = codersdk.ChatContextMCPDiscoveryPhaseUnknown
			return summarizeMCPDiscovery(waitCtx, db, agentID, outcome)
		}

		select {
		case <-waitCtx.Done():
			outcome.WaitTimedOut = true
			return summarizeMCPDiscovery(ctx, db, agentID, outcome)
		case <-ticker.C:
		}
	}
}

// CurrentMCPDiscovery reads the agent's discovery state once, without
// waiting, and reports whether the current process has completed
// discovery. chatd uses it to refresh a spent (timed-out) attempt whose
// discovery finished after the wait ended. Read errors report unknown
// and false.
func CurrentMCPDiscovery(ctx context.Context, db database.Store, agentID uuid.UUID) (MCPDiscoveryOutcome, bool) {
	outcome := MCPDiscoveryOutcome{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
	agent, err := db.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil {
		return outcome, false
	}
	snap, err := db.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
	if err != nil {
		return outcome, false
	}
	outcome.Phase = phaseFromSnapshot(snap)
	outcome.Stale = agent.AgentRunID != snap.AgentRunID
	if outcome.Stale || outcome.Phase != codersdk.ChatContextMCPDiscoveryPhaseComplete {
		return outcome, false
	}
	return summarizeMCPDiscovery(ctx, db, agentID, outcome), true
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
