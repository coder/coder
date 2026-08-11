package aiagentidentity

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

// WorkspaceAgentAllowsOwnerCredentials reports whether a workspace agent may
// receive ambient credentials owned by its workspace sponsor. Vertical 2 phase
// 1 treats every bound agent as ai_credential_mode=none. Actor context support
// lets phase 2 replace the row-only identity check without changing callers.
func WorkspaceAgentAllowsOwnerCredentials(ctx context.Context, agent database.WorkspaceAgent) bool {
	if _, ok := ActorFromContext(ctx); ok {
		return false
	}
	return !agent.AIAgentID.Valid
}
