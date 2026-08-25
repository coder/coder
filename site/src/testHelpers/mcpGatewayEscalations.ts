import type { MCPGatewayEscalation } from "#/api/typesGenerated";

export const MockMCPGatewayEscalation: MCPGatewayEscalation = {
	id: "00000000-0000-4000-8000-000000000001",
	server_slug: "github",
	tool: "create_pull_request",
	input: JSON.stringify(
		{
			owner: "coder",
			repository: "coder",
			title: "Add MCP approvals",
		},
		null,
		2,
	),
	workspace_name: "mcp-escalation-demo",
	status: "pending",
	created_at: "2026-08-25T11:58:00Z",
	expires_at: "2026-08-25T12:03:00Z",
};

export const MockNewerMCPGatewayEscalation: MCPGatewayEscalation = {
	id: "00000000-0000-4000-8000-000000000002",
	server_slug: "linear",
	tool: "create_issue",
	input: JSON.stringify(
		{
			team: "ENG",
			title: "Investigate gateway latency",
			priority: 2,
		},
		null,
		2,
	),
	workspace_name: "agent-workspace",
	status: "pending",
	created_at: "2026-08-25T11:59:00Z",
	expires_at: "2026-08-25T12:04:00Z",
};
