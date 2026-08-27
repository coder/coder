import type {
	AIAuditTrailEvent,
	AIAuditTrailOwner,
	AIAuditTrailResponse,
} from "#/api/typesGenerated";

const MockAIAuditTrailOwner: AIAuditTrailOwner = {
	type: "user",
	id: "45e57ab6-becd-4a6c-a19f-b2ec2c00c1fb",
	username: "kira",
};

const mockAgentID = "9d0d4c33-6b7a-4f9f-8bbd-2f2121ea6b1b";

export const MockAIAuditTrailEvents: AIAuditTrailEvent[] = [
	{
		id: "egress:6e2a9a72-0f39-4a2e-b3a5-13e2c0aa6f2f:pastebin.example.com:denied",
		type: "egress",
		occurred_at: "2026-08-25T11:58:00Z",
		recorded_at: "2026-08-25T11:58:05Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		summary: "denied tcp pastebin.example.com:443 (x12)",
		detail: {
			event: "denied",
			session_id: "6e2a9a72-0f39-4a2e-b3a5-13e2c0aa6f2f",
			host: "pastebin.example.com",
			port: 443,
			protocol: "tcp",
			count: 12,
		},
	},
	{
		id: "sandbox_session:6e2a9a72-0f39-4a2e-b3a5-13e2c0aa6f2f:started",
		type: "sandbox_session",
		occurred_at: "2026-08-25T11:55:00Z",
		recorded_at: "2026-08-25T11:55:01Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		workspace_id: "0b8a2f0f-45f7-4b0e-9f1e-282f5d3f9d6e",
		summary: "sandbox egress session started (forced)",
		detail: {
			event: "started",
			session_id: "6e2a9a72-0f39-4a2e-b3a5-13e2c0aa6f2f",
			egress_enforcement: "forced",
		},
	},
	{
		id: "credential_use:12",
		type: "credential_use",
		occurred_at: "2026-08-25T11:54:00Z",
		recorded_at: "2026-08-25T11:54:00Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		summary: "credential presentation accepted (password)",
		detail: {
			event: "presentation_accepted",
			entry_id: 12,
			credential_id: "70cbe5e4-79b0-4ad6-9b13-abcbbf7f7ce2",
			credential_type: "password",
		},
	},
	{
		id: "credential_use:11",
		type: "credential_use",
		occurred_at: "2026-08-25T11:53:30Z",
		recorded_at: "2026-08-25T11:53:30Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		summary: "credential presentation refused (password)",
		detail: {
			event: "presentation_refused",
			entry_id: 11,
			credential_id: "70cbe5e4-79b0-4ad6-9b13-abcbbf7f7ce2",
			credential_type: "password",
		},
	},
	{
		id: "credential_lifecycle:8:0",
		type: "credential_lifecycle",
		occurred_at: "2026-08-25T11:52:00Z",
		recorded_at: "2026-08-25T11:52:00Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		summary: "credential issued (password)",
		detail: {
			event: "issue",
			entry_id: 8,
			credential_id: "70cbe5e4-79b0-4ad6-9b13-abcbbf7f7ce2",
			credential_type: "password",
		},
	},
	{
		id: "authorization_lifecycle:5:0",
		type: "authorization_lifecycle",
		occurred_at: "2026-08-25T11:52:00Z",
		recorded_at: "2026-08-25T11:52:00Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		summary: "authorization granted",
		detail: {
			event: "grant",
			entry_id: 5,
			authorization_id: "e0e493f1-9895-4a25-a7a5-2eeb96acdbb1",
			principal_type: "user",
			principal_id: MockAIAuditTrailOwner.id,
		},
	},
	{
		// An observed transition recorded well after it happened: the
		// timeline renders both dates for this one.
		id: "ai_agent_lifecycle:3",
		type: "ai_agent_lifecycle",
		occurred_at: "2026-08-25T11:52:00Z",
		recorded_at: "2026-08-25T12:30:00Z",
		ai_agent_id: mockAgentID,
		owner: MockAIAuditTrailOwner,
		summary: "AI agent created in workspace",
		detail: {
			event: "create",
			entry_id: 3,
			creation_site_type: "workspace",
			creation_site_id: "0b8a2f0f-45f7-4b0e-9f1e-282f5d3f9d6e",
		},
	},
];

export const MockAIAuditTrailResponse: AIAuditTrailResponse = {
	events: MockAIAuditTrailEvents,
	count: MockAIAuditTrailEvents.length,
};
