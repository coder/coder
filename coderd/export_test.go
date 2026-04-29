package coderd

// InsertAgentChatTestModelConfig exposes insertAgentChatTestModelConfig for external tests.
var InsertAgentChatTestModelConfig = insertAgentChatTestModelConfig

// ChatStartWorkspace exposes chatStartWorkspace for external tests.
//
// chatStartWorkspace is intentionally unexported to keep symmetry with
// its sister chatCreateWorkspace. Both methods bundle DB lookups,
// version resolution, preset handling, and build creation; testing
// the auto-update preset-clearing branch (DEREM-16/17 from PR #24694)
// without an alias would require either exporting the method or
// stubbing the entire DB layer. The proper fix is to extract a pure
// request builder; tracked in CODAGT-292.
var ChatStartWorkspace = (*API).chatStartWorkspace
