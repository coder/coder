// Package agentcontext consolidates the agent-side plumbing that
// resolves, watches, and pushes workspace context (instruction
// files, skills, and MCP configuration) to coderd.
//
// This is the agent half of the design described in
// "RFC: Workspace Context Sources for Coder Agents". It owns:
//
//   - User-declared scan roots (Sources) layered on top of
//     built-in defaults.
//   - A resolver that classifies files under each scan root into
//     typed Resources (instruction files, skills, MCP configs,
//     MCP servers).
//   - A unified recursive fsnotify watcher that signals a
//     re-resolve when any recognized file changes.
//   - An HTTP API at /api/v0/context/sources for source CRUD
//     and /api/v0/context/resync for synchronous push barriers.
//   - A Pusher abstraction so the latest Snapshot can be shipped
//     to coderd without coupling this package to any particular
//     drpc client version.
//
// The package is purely additive: existing agent code paths
// (agent/agentcontextconfig and agent/x/agentmcp) continue to
// operate unchanged. Wiring the Manager into the agent's HTTP
// router and the drpc client lives in a follow-up change.
package agentcontext
