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
// Live MCP server tool lists are produced by this package's own
// self-contained MCP runner: it connects to the MCP servers declared in
// the .mcp.json files the resolver discovers, lists their tools, and
// surfaces them as KindMCPServer resources so MCP servers and their
// tools are pushed to coderd alongside instruction files and skills.
// This runs independently of agent/x/agentmcp, which owns the agent's
// MCP HTTP proxy; the two MCP paths share no state and both continue to
// operate unchanged during the rollout.
package agentcontext
