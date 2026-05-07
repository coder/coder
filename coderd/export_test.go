package coderd

// InsertAgentChatTestModelConfig exposes insertAgentChatTestModelConfig for external tests.
var InsertAgentChatTestModelConfig = insertAgentChatTestModelConfig

// ChatStartWorkspace exposes chatStartWorkspace for external tests.
//
// chatStartWorkspace is intentionally unexported to keep symmetry with
// its sister chatCreateWorkspace. The alias lets external tests drive
// the RequireActiveVersion auto-update path end-to-end without
// stubbing the entire DB layer. The proper fix is to extract a pure
// request builder; tracked in CODAGT-292.
var ChatStartWorkspace = (*API).chatStartWorkspace
