import type {
	AIAuditAgent,
	AIAuditEvent,
	MinimalUser,
} from "#/api/typesGenerated";

const MockSponsor: MinimalUser = {
	id: "00000000-0000-4000-8000-0000000000aa",
	username: "sponsor-user",
	avatar_url: "",
};

export const MockAIAuditAgent: AIAuditAgent = {
	user_id: "00000000-0000-4000-8000-0000000000a1",
	username: "ai-agent-4f2b",
	owner_user_id: MockSponsor.id,
	origin_type: "workspace",
	origin_id: "00000000-0000-4000-8000-0000000000b1",
	created_at: "2026-08-25T10:00:00Z",
	deleted: false,
};

export const MockAIAuditEvents: AIAuditEvent[] = [
	{
		id: "00000000-0000-4000-8000-000000000011",
		type: "escalation_resolved",
		occurred_at: "2026-08-25T11:59:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		workspace_name: "agent-workspace",
		summary: "escalation approved for tool create_pull_request",
		detail: {
			escalation_id: "00000000-0000-4000-8000-000000000021",
			server_slug: "github",
			tool: "create_pull_request",
			status: "approved",
		},
	},
	{
		id: "00000000-0000-4000-8000-000000000021",
		type: "escalation_created",
		occurred_at: "2026-08-25T11:58:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		workspace_name: "agent-workspace",
		summary: "escalation created for tool create_pull_request on github",
		detail: {
			escalation_id: "00000000-0000-4000-8000-000000000021",
			server_slug: "github",
			tool: "create_pull_request",
			expires_at: "2026-08-25T12:03:00Z",
		},
	},
	{
		id: "00000000-0000-4000-8000-000000000031",
		type: "tool_call",
		occurred_at: "2026-08-25T11:57:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		summary: "tool call create_pull_request: escalated_approved",
		detail: {
			interception_id: "00000000-0000-4000-8000-000000000041",
			server_url: "github",
			tool: "create_pull_request",
			disposition: "escalated_approved",
		},
	},
	{
		id: "00000000-0000-4000-8000-000000000051",
		type: "egress",
		occurred_at: "2026-08-25T11:56:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		workspace_id: "00000000-0000-4000-8000-000000000061",
		summary: "denied tcp evil.example.com:443 (x12)",
		detail: {
			session_id: "00000000-0000-4000-8000-000000000071",
			host: "evil.example.com",
			port: 443,
			protocol: "tcp",
			action: "denied",
			count: 12,
		},
	},
	{
		id: "00000000-0000-4000-8000-000000000052",
		type: "egress",
		occurred_at: "2026-08-25T11:55:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		workspace_id: "00000000-0000-4000-8000-000000000061",
		summary: "allowed tcp github.com:443 (x34)",
		detail: {
			session_id: "00000000-0000-4000-8000-000000000071",
			host: "github.com",
			port: 443,
			protocol: "tcp",
			action: "allowed",
			count: 34,
		},
	},
	{
		id: "00000000-0000-4000-8000-000000000041",
		type: "bridge_session_started",
		occurred_at: "2026-08-25T11:54:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		summary: "bridge session started via claude-code",
		detail: {
			session_id: "00000000-0000-4000-8000-000000000041",
			client: "claude-code",
			providers: ["anthropic"],
			models: ["claude-sonnet-4"],
		},
	},
	{
		id: "00000000-0000-4000-8000-000000000071",
		type: "sandbox_session_started",
		occurred_at: "2026-08-25T11:53:00Z",
		ai_agent_id: MockAIAuditAgent.user_id,
		sponsor: MockSponsor,
		workspace_id: "00000000-0000-4000-8000-000000000061",
		summary: "sandbox session started (egress forced)",
		detail: {
			egress_enforcement: "forced",
		},
	},
];
