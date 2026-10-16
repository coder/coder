package chattool

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/quartz"
)

// ErrNoWorkspaceAgent is returned by ResolveChatAgent when the latest
// build has no agents.
var ErrNoWorkspaceAgent = xerrors.New("workspace has no agents in the latest build")

// DevcontainerUnavailableMessage tells the model that commands run on the
// parent agent because the devcontainer agent never became available.
const DevcontainerUnavailableMessage = "The workspace declares a devcontainer, but its agent is not available " +
	"(it failed to start or is ignored). Commands run on the parent workspace agent, " +
	"outside the devcontainer, so its tools and environment may be missing."

// ResolvedChatAgent is the agent a chat should use.
type ResolvedChatAgent struct {
	Agent database.WorkspaceAgent
	// DevcontainerUnavailable reports that Agent is a parent agent whose
	// declared devcontainer agent never became available.
	DevcontainerUnavailable bool
}

// ResolveChatAgent selects the chat agent for the workspace's latest build
// with agentselect.FindChatAgent. When the selected agent is a root agent
// that declares devcontainers but has no devcontainer agent yet, it waits up
// to startupScriptTimeout for one to register. Devcontainer agents register
// before the parent agent leaves its starting lifecycle states, so a parent
// in a terminal state without one means the devcontainer failed or is
// ignored, and the parent is returned with DevcontainerUnavailable set.
func ResolveChatAgent(
	ctx context.Context,
	db database.Store,
	clock quartz.Clock,
	workspaceID uuid.UUID,
) (ResolvedChatAgent, error) {
	selected, err := selectLatestChatAgent(ctx, db, workspaceID)
	if err != nil {
		return ResolvedChatAgent{}, err
	}
	return waitForDevcontainerAgent(ctx, db, clock, workspaceID, selected)
}

// waitForDevcontainerAgent implements the devcontainer wait of
// ResolveChatAgent for an agent already selected from the latest build.
func waitForDevcontainerAgent(
	ctx context.Context,
	db database.Store,
	clock quartz.Clock,
	workspaceID uuid.UUID,
	selected database.WorkspaceAgent,
) (ResolvedChatAgent, error) {
	var checkedParentID uuid.UUID
	// pending reports whether selected is a starting parent whose declared
	// devcontainer agent has not registered yet. Otherwise it returns the
	// final resolution.
	pending := func(selected database.WorkspaceAgent) (ResolvedChatAgent, bool, error) {
		if selected.ParentID.Valid {
			return ResolvedChatAgent{Agent: selected}, false, nil
		}
		if selected.ID != checkedParentID {
			declared, err := db.GetWorkspaceAgentDevcontainersByAgentID(ctx, selected.ID)
			if err != nil {
				return ResolvedChatAgent{}, false, xerrors.Errorf("get workspace agent devcontainers: %w", err)
			}
			if len(declared) == 0 {
				return ResolvedChatAgent{Agent: selected}, false, nil
			}
			checkedParentID = selected.ID
		}
		if !isAgentStarting(clock.Now(), selected) {
			return ResolvedChatAgent{Agent: selected, DevcontainerUnavailable: true}, false, nil
		}
		return ResolvedChatAgent{}, true, nil
	}

	resolved, waiting, err := pending(selected)
	if !waiting {
		return resolved, err
	}

	timer := clock.NewTimer(startupScriptTimeout, "chattool", "devcontainer_agent_wait")
	defer timer.Stop()
	ticker := clock.NewTicker(startupScriptPollInterval, "chattool", "devcontainer_agent_poll")
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ResolvedChatAgent{}, ctx.Err()
		case <-timer.C:
			return ResolvedChatAgent{Agent: selected, DevcontainerUnavailable: true}, nil
		case <-ticker.C:
		}

		selected, err = selectLatestChatAgent(ctx, db, workspaceID)
		if err != nil {
			return ResolvedChatAgent{}, err
		}
		resolved, waiting, err = pending(selected)
		if !waiting {
			return resolved, err
		}
	}
}

func selectLatestChatAgent(
	ctx context.Context,
	db database.Store,
	workspaceID uuid.UUID,
) (database.WorkspaceAgent, error) {
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return database.WorkspaceAgent{}, xerrors.Errorf("get workspace agents in latest build: %w", err)
	}
	if len(agents) == 0 {
		return database.WorkspaceAgent{}, ErrNoWorkspaceAgent
	}
	selected, err := agentselect.FindChatAgent(agents)
	if err != nil {
		return database.WorkspaceAgent{}, xerrors.Errorf("find chat agent: %w", err)
	}
	return selected, nil
}

// isAgentStarting reports whether agent may still register devcontainer
// agents: its lifecycle is not terminal and it has not timed out before
// its first connection.
func isAgentStarting(now time.Time, agent database.WorkspaceAgent) bool {
	if agent.LifecycleState != database.WorkspaceAgentLifecycleStateCreated &&
		agent.LifecycleState != database.WorkspaceAgentLifecycleStateStarting {
		return false
	}
	// The inactive timeout only affects agents that already connected,
	// which never report a timeout status.
	return agent.Status(now, 0).Status != database.WorkspaceAgentStatusTimeout
}

// setDevcontainerStatus reports a devcontainer fallback in a workspace
// tool result.
func setDevcontainerStatus(result map[string]any, resolved ResolvedChatAgent) {
	if resolved.DevcontainerUnavailable {
		result["devcontainer_status"] = "unavailable"
		result["devcontainer_message"] = DevcontainerUnavailableMessage
	}
}

func clockOrReal(clock quartz.Clock) quartz.Clock {
	if clock == nil {
		return quartz.NewReal()
	}
	return clock
}
