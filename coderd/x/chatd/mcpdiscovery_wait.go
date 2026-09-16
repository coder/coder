package chatd

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// mcpDiscoveryAttempt is one bounded wait for an agent process's workspace
// MCP discovery. Attempts are cached per chat and keyed by agent and agent
// run id so preparation steps, user turns, and the create/start workspace
// hooks share a single budget for the same process; a new agent or a new
// process (run id) rearms it.
type mcpDiscoveryAttempt struct {
	agentID    uuid.UUID
	agentRunID string
	done       chan struct{}
	outcome    chattool.MCPDiscoveryOutcome
}

// waitForMCPDiscovery waits, at most once per chat and agent process, for
// the agent's workspace MCP discovery to complete. Waits that end in a
// timeout stay cached so a later turn on the same process does not spend
// the budget again; completed waits are dropped because re-checking a
// complete snapshot is cheap. Every outcome fails open: the caller
// continues with the current-run tools the view exposes.
func (p *Server) waitForMCPDiscovery(ctx context.Context, chatID, agentID uuid.UUID) chattool.MCPDiscoveryOutcome {
	var agentRunID string
	if agent, err := p.db.GetWorkspaceAgentByID(ctx, agentID); err == nil {
		agentRunID = agent.AgentRunID
	} else {
		p.logger.Debug(ctx, "failed to read agent run id before MCP discovery wait",
			slog.F("chat_id", chatID), slog.F("agent_id", agentID), slog.Error(err))
	}

	p.mcpDiscoveryMu.Lock()
	attempt := p.mcpDiscoveryAttempts[chatID]
	if attempt != nil && attempt.agentID == agentID && attempt.agentRunID == agentRunID {
		p.mcpDiscoveryMu.Unlock()
		select {
		case <-attempt.done:
			return attempt.outcome
		case <-ctx.Done():
			return chattool.MCPDiscoveryOutcome{Phase: attempt.outcome.Phase, WaitTimedOut: true}
		}
	}
	attempt = &mcpDiscoveryAttempt{agentID: agentID, agentRunID: agentRunID, done: make(chan struct{})}
	if p.mcpDiscoveryAttempts == nil {
		p.mcpDiscoveryAttempts = make(map[uuid.UUID]*mcpDiscoveryAttempt)
	}
	p.mcpDiscoveryAttempts[chatID] = attempt
	p.mcpDiscoveryMu.Unlock()

	attempt.outcome = chattool.WaitForMCPDiscovery(ctx, p.db, agentID)
	close(attempt.done)
	if !attempt.outcome.WaitTimedOut {
		p.mcpDiscoveryMu.Lock()
		if p.mcpDiscoveryAttempts[chatID] == attempt {
			delete(p.mcpDiscoveryAttempts, chatID)
		}
		p.mcpDiscoveryMu.Unlock()
	}
	return attempt.outcome
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
