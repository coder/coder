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
