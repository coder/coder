package agentapi

import (
	"context"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

// publishMCPDiscoveryUpdate wakes chats waiting on the agent's workspace
// MCP discovery so they read the committed state again. A lost message
// only delays a waiter until its run's deadline, so a failure is logged
// rather than failing the agent's request.
func publishMCPDiscoveryUpdate(ctx context.Context, ps pubsub.Pubsub, log slog.Logger, agentID uuid.UUID) {
	if ps == nil {
		return
	}
	if err := ps.Publish(coderdpubsub.WorkspaceAgentMCPDiscoveryChannel(agentID), nil); err != nil {
		log.Warn(ctx, "failed to publish workspace agent MCP discovery update",
			slog.F("agent_id", agentID), slog.Error(err))
	}
}
