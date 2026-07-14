// Package agentcontext consolidates the agent-side plumbing that
// resolves, watches, and pushes workspace context (instruction
// files, skills, and MCP configuration) to coderd.
//
// This is the agent half of the design described in
// "RFC: Workspace Context Sources for Coder Agents". It owns:
//
//   - User-declared scan roots (Sources) layered on top of
//     built-in defaults and the working directory.
//   - A resolver that classifies files at fixed locations under
//     each scan root into typed Resources (instruction files,
//     skills, MCP configs, MCP servers). Discovery is shallow:
//     instruction files (AGENTS.md, CLAUDE.md, .cursorrules) and
//     .mcp.json are read only at a scan root's top level, skills
//     only from fixed container directories (skills, .agents/skills,
//     .claude/skills, .codex/skills), and the resolver never walks
//     the tree downward or up to a parent directory.
//   - A fixed-location fsnotify watcher that signals a re-resolve
//     when any recognized file changes.
//   - A readiness gate (Manager.SetReady). The Manager starts gated,
//     publishing only an empty version-0 snapshot until the agent calls
//     SetReady from the workspace lifecycle transition once startup
//     scripts finish. This keeps pre-startup partial state out of
//     coderd and chats.
//   - An HTTP API at /api/v0/context/sources for source CRUD
//     and /api/v0/context/resync for synchronous push barriers.
//   - A Pusher abstraction so the latest Snapshot can be shipped
//     to coderd without coupling this package to any particular
//     drpc client version.
//
// Live MCP server tool lists come from the shared MCP engine in
// agent/x/agentmcp, which owns the single set of MCP server connections
// used for both tool discovery and tool-call execution. This package
// reads that engine's catalog through the injected MCPCatalog option and
// surfaces the servers and their tools as KindMCPServer resources, so
// MCP servers are pushed to coderd alongside instruction files and
// skills. The engine notifies this package through the Manager's Trigger
// when its catalog changes, driving a re-resolve and re-push.
package agentcontext
