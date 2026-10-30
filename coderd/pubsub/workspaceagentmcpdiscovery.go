package pubsub

import (
	"fmt"

	"github.com/google/uuid"
)

// WorkspaceAgentMCPDiscoveryChannel returns the pubsub channel that
// receives an empty message after a write commits that can end a chat's
// wait for the agent's workspace MCP discovery: the agent run id, the
// run's discovery start time, or the run id or discovery phase of the
// latest context snapshot. PostgreSQL rejects NOTIFY channel names over
// 63 bytes, so the prefix stays short.
func WorkspaceAgentMCPDiscoveryChannel(agentID uuid.UUID) string {
	return fmt.Sprintf("agent:mcp_discovery:%s", agentID)
}
