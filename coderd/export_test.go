package coderd

import "github.com/coder/coder/v2/coderd/workspaceapps"

// InsertAgentChatTestModelConfig exposes insertAgentChatTestModelConfig for external tests.
var InsertAgentChatTestModelConfig = insertAgentChatTestModelConfig

// SetAgentProviderForTest replaces the workspace agent provider for external tests.
func SetAgentProviderForTest(api *API, provider workspaceapps.AgentProvider) func() {
	previous := api.agentProvider
	api.agentProvider = provider
	return func() {
		api.agentProvider = previous
	}
}

// ChatStartWorkspace exposes chatStartWorkspace for external tests.
//
// chatStartWorkspace is intentionally unexported to keep symmetry with
// its sister chatCreateWorkspace. The alias lets external tests drive
// the RequireActiveVersion auto-update path end-to-end without
// stubbing the entire DB layer. The proper fix is to extract a pure
// request builder; tracked in CODAGT-292.
var ChatStartWorkspace = (*API).chatStartWorkspace

// ChatStopWorkspace exposes chatStopWorkspace for external tests.
var ChatStopWorkspace = (*API).chatStopWorkspace
