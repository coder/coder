package aiagentidentity

import "github.com/coder/coder/v2/coderd/database"

// WorkspaceAgentAllowsOwnerCredentials reports whether a workspace agent may
// receive ambient credentials owned by its workspace sponsor. Vertical 2 phase
// 1 treats every bound agent as ai_credential_mode=none. Phase 2 can replace
// this row-only binding check with the resolved actor from agent middleware.
func WorkspaceAgentAllowsOwnerCredentials(agent database.WorkspaceAgent) bool {
	return !agent.AIAgentID.Valid
}
