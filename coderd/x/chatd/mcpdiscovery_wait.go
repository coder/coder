package chatd

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// mcpDiscoveryAttemptRetention bounds how long a timed-out attempt stays
// cached. Discovery still pending after this long is not going to finish
// on its own, so a later turn spends one more bounded wait instead of
// trusting the old outcome forever, and the cache cannot grow without
// bound.
const mcpDiscoveryAttemptRetention = 10 * time.Minute

// mcpDiscoveryRefreshTimeout bounds the single read that refreshes a
// spent attempt's outcome, so a slow database cannot turn a reuse into a
// second wait.
const mcpDiscoveryRefreshTimeout = 2 * time.Second

// mcpDiscoveryAttempt is one bounded wait for an agent process's workspace
// MCP discovery. Attempts are cached per chat on this replica and keyed by
// agent and agent run id so preparation steps, user turns, and the
// create/start workspace hooks share a single budget for the same process;
// a new agent or a new process (run id) rearms it. Another replica picking
// up the chat spends its own bounded wait; the cache is an optimization,
// not the bound.
type mcpDiscoveryAttempt struct {
	agentID uuid.UUID
	// agentRunID is guarded by Server.mcpDiscoveryMu: the run id the wait
	// started on, replaced by the run it last observed once it settles.
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
	now := time.Now()
	if p.clock != nil {
		now = p.clock.Now()
	}
	join := func(attempt *mcpDiscoveryAttempt) chattool.MCPDiscoveryOutcome {
		select {
		case <-attempt.done:
			outcome := attempt.outcome
			if outcome.WaitTimedOut && ctx.Err() == nil {
				// Discovery may have finished after the budget was spent;
				// report the current state with one bounded read rather
				// than the stale timeout, without arming another wait.
				refreshCtx, cancel := context.WithTimeout(ctx, mcpDiscoveryRefreshTimeout)
				fresh, complete := chattool.CurrentMCPDiscovery(refreshCtx, p.db, agentID)
				cancel()
				if complete {
					return fresh
				}
			}
			return outcome
		case <-ctx.Done():
			return chattool.MCPDiscoveryOutcome{WaitTimedOut: true}
		}
	}

	agent, err := p.db.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil {
		// Without the run id the cache key is unknown. Reuse the chat's
		// attempt for this agent if there is one rather than replacing it
		// with an attempt built on a failing context; otherwise fail open
		// without caching anything.
		p.logger.Debug(ctx, "failed to read agent run id before MCP discovery wait",
			slog.F("chat_id", chatID), slog.F("agent_id", agentID), slog.Error(err))
		p.mcpDiscoveryMu.Lock()
		attempt := p.mcpDiscoveryAttempts[chatID]
		p.mcpDiscoveryMu.Unlock()
		if attempt != nil && attempt.agentID == agentID {
			return join(attempt)
		}
		return chattool.MCPDiscoveryOutcome{Phase: codersdk.ChatContextMCPDiscoveryPhaseUnknown}
	}
	agentRunID := agent.AgentRunID

	p.mcpDiscoveryMu.Lock()
	attempt := p.mcpDiscoveryAttempts[chatID]
	if attempt != nil && attempt.agentID == agentID && attempt.agentRunID == agentRunID &&
		(attempt.expiresAt.IsZero() || now.Before(attempt.expiresAt)) {
		p.mcpDiscoveryMu.Unlock()
		return join(attempt)
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
	observedRunID := agentRunID
	if outcome.WaitTimedOut {
		// The agent may have restarted during the wait, in which case the
		// wait spent its budget on the new process; key the spent attempt
		// to that process so the next turn reuses it instead of waiting
		// again. The read outlives a canceled turn because the attempt
		// stays cached for later turns.
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mcpDiscoveryRefreshTimeout)
		if current, err := p.db.GetWorkspaceAgentByID(readCtx, agentID); err == nil {
			observedRunID = current.AgentRunID
		}
		cancel()
	}
	p.mcpDiscoveryMu.Lock()
	attempt.agentRunID = observedRunID
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
