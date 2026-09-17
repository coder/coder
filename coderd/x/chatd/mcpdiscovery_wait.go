package chatd

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// mcpDiscoveryAttemptRetention bounds how long a timed-out attempt stays
// cached. Discovery still pending after this long is not going to finish
// on its own, so a later turn spends one more bounded wait instead of
// trusting the old outcome forever, and the cache cannot grow without
// bound.
const mcpDiscoveryAttemptRetention = 10 * time.Minute

// mcpDiscoveryAttempt is one bounded wait for an agent process's workspace
// MCP discovery. Attempts are cached per chat on this replica and keyed by
// agent and agent run id so preparation steps, user turns, and the
// create/start workspace hooks share a single budget for the same process;
// a new agent or a new process (run id) rearms it. Another replica picking
// up the chat spends its own bounded wait; the cache is an optimization,
// not the bound.
type mcpDiscoveryAttempt struct {
	agentID    uuid.UUID
	agentRunID string
	done       chan struct{}
	// outcome is written by the owning goroutine before done is closed and
	// read by joiners only after it.
	outcome chattool.MCPDiscoveryOutcome
	// expiresAt is guarded by Server.mcpDiscoveryMu; zero while the attempt
	// is in flight, set when it times out.
	expiresAt time.Time
}

// waitForMCPDiscovery waits, at most once per chat and agent process, for
// the agent's workspace MCP discovery to complete. Waits that end in a
// timeout stay cached for mcpDiscoveryAttemptRetention so a later turn on
// the same process does not spend the budget again; completed waits are
// dropped because re-checking a complete snapshot is cheap. Every outcome
// fails open: the caller continues with the current-run tools the view
// exposes.
func (p *Server) waitForMCPDiscovery(ctx context.Context, chatID, agentID uuid.UUID) chattool.MCPDiscoveryOutcome {
	var agentRunID string
	if agent, err := p.db.GetWorkspaceAgentByID(ctx, agentID); err == nil {
		agentRunID = agent.AgentRunID
	} else {
		p.logger.Debug(ctx, "failed to read agent run id before MCP discovery wait",
			slog.F("chat_id", chatID), slog.F("agent_id", agentID), slog.Error(err))
	}

	now := time.Now()
	if p.clock != nil {
		now = p.clock.Now()
	}
	p.mcpDiscoveryMu.Lock()
	attempt := p.mcpDiscoveryAttempts[chatID]
	if attempt != nil && attempt.agentID == agentID && attempt.agentRunID == agentRunID &&
		(attempt.expiresAt.IsZero() || now.Before(attempt.expiresAt)) {
		p.mcpDiscoveryMu.Unlock()
		select {
		case <-attempt.done:
			return attempt.outcome
		case <-ctx.Done():
			return chattool.MCPDiscoveryOutcome{WaitTimedOut: true}
		}
	}
	for id, expired := range p.mcpDiscoveryAttempts {
		if !expired.expiresAt.IsZero() && !now.Before(expired.expiresAt) {
			delete(p.mcpDiscoveryAttempts, id)
		}
	}
	attempt = &mcpDiscoveryAttempt{agentID: agentID, agentRunID: agentRunID, done: make(chan struct{})}
	if p.mcpDiscoveryAttempts == nil {
		p.mcpDiscoveryAttempts = make(map[uuid.UUID]*mcpDiscoveryAttempt)
	}
	p.mcpDiscoveryAttempts[chatID] = attempt
	p.mcpDiscoveryMu.Unlock()

	outcome := chattool.WaitForMCPDiscovery(ctx, p.db, agentID)
	p.mcpDiscoveryMu.Lock()
	if outcome.WaitTimedOut {
		attempt.expiresAt = now.Add(mcpDiscoveryAttemptRetention)
	} else if p.mcpDiscoveryAttempts[chatID] == attempt {
		delete(p.mcpDiscoveryAttempts, chatID)
	}
	p.mcpDiscoveryMu.Unlock()
	attempt.outcome = outcome
	close(attempt.done)
	return outcome
}

// mcpDiscoveryWaiter binds waitForMCPDiscovery to a chat for the
// create_workspace and start_workspace tools.
func (p *Server) mcpDiscoveryWaiter(chatID uuid.UUID) chattool.MCPDiscoveryWaiter {
	return func(ctx context.Context, agentID uuid.UUID) chattool.MCPDiscoveryOutcome {
		return p.waitForMCPDiscovery(ctx, chatID, agentID)
	}
}

// appendWorkspaceMCPNote adds the workspace MCP discovery summary to the
// workspace-context system block so the model can explain a missing or
// incomplete tool inventory instead of guessing. An empty instruction
// block (no instruction files and no agent metadata) gets a minimal block
// of its own.
func appendWorkspaceMCPNote(instruction, summary string) string {
	if summary == "" {
		return instruction
	}
	const closing = "</workspace-context>"
	line := "\nWorkspace MCP: " + summary + "\n"
	if instruction == "" {
		return "<workspace-context>" + line + closing
	}
	if idx := strings.LastIndex(instruction, closing); idx >= 0 {
		return instruction[:idx] + line + instruction[idx:]
	}
	return instruction + line
}
