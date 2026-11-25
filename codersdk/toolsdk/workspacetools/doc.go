// Package workspacetools implements the workspace tool behavior shared by
// Coder Agents (coderd/x/chatd/chattool) and the Coder MCP server
// (codersdk/toolsdk). It talks to the workspace agent through
// workspacesdk.AgentConn and has no dependency on either tool framework,
// so both surfaces run the same commands and return the same result
// formats.
package workspacetools
