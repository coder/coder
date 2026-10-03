import type {
	Chat,
	ChatAutomation,
	ChatContext,
	ChatContextResource,
	ChatFileMetadata,
	ChatMessage,
	ChatProject,
	ChatQueuedMessage,
	MCPServerConfig,
} from "#/api/typesGenerated";
import { MockUserOwner } from "./entities";

export const MOCK_TIMESTAMP = "2024-01-01T00:00:00Z";

export const MockChat: Chat = {
	id: "chat-1",
	organization_id: "test-org-id",
	owner_id: MockUserOwner.id,
	owner_username: MockUserOwner.username,
	owner_name: MockUserOwner.name,
	last_model_config_id: "model-config-1",
	title: "Agent",
	title_source: "generated",
	title_updated_at: MOCK_TIMESTAMP,
	status: "waiting",
	last_turn_summary: null,
	summary: null,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
	archived: false,
	shared: false,
	pin_order: 0,
	mcp_server_ids: [],
	labels: {},
	has_unread: false,
	client_type: "ui",
	children: [],
};

// Pinned workspace-context resources the prompt is built from.
const MockChatContextResources: ChatContextResource[] = [
	{
		source: "/home/coder/AGENTS.md",
		kind: "instruction_file",
		size_bytes: 248,
		status: "ok",
	},
	{
		source: "/home/coder/.coder/skills/deploy",
		kind: "skill",
		size_bytes: 96,
		status: "ok",
		skill_name: "deploy",
		skill_description: "Deploy the app to staging.",
	},
	{
		source: "/home/coder/.mcp.json",
		kind: "mcp_config",
		size_bytes: 184,
		status: "ok",
	},
	{
		source: "github",
		kind: "mcp_server",
		size_bytes: 512,
		status: "ok",
		tools: [
			{
				name: "search_issues",
				description: "Search issues and pull requests.",
			},
			{ name: "create_issue", description: "Open a new issue." },
		],
	},
	{
		// An invalid skill the agent rejected: surfaced as an issue with its
		// error rather than silently dropped.
		source: "/home/coder/test/.agents/skills/moo",
		kind: "skill",
		size_bytes: 356,
		status: "invalid",
		error: 'front-matter name "coder-review" does not match directory "moo"',
	},
];

export const MockChatContextClean: ChatContext = {
	dirty: false,
	resources: MockChatContextResources,
};

export const MockChatContextDirty: ChatContext = {
	dirty: true,
	dirty_since: "2024-01-02T00:00:00Z",
	resources: MockChatContextResources,
};

export const MockMCPServerConfig: MCPServerConfig = {
	id: "mcp-1",
	organization_id: "00000000-0000-4000-8000-000000000001",
	display_name: "MCP Server",
	slug: "mcp-server",
	description: "",
	icon_url: "",
	transport: "streamable_http",
	url: "https://mcp.example.com/sse",
	auth_type: "none",
	has_oauth2_secret: false,
	has_api_key: false,
	has_custom_headers: false,
	has_signing_secret: false,
	tool_allow_list: [],
	tool_deny_list: [],
	availability: "default_on",
	enabled: true,
	model_intent: false,
	allow_in_plan_mode: false,
	forward_coder_headers: false,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
	auth_connected: false,
};

export const MockChatMessage: ChatMessage = {
	id: 1,
	chat_id: "chat-1",
	created_at: MOCK_TIMESTAMP,
	role: "user",
	content: [{ type: "text", text: "Hello" }],
};

export const MockChatFileMetadata: ChatFileMetadata = {
	id: "chat-file-1",
	owner_id: MockUserOwner.id,
	organization_id: "test-org-id",
	name: "notes.txt",
	mime_type: "text/plain",
	size_bytes: 128,
	created_at: MOCK_TIMESTAMP,
};

export const MockChatCompactionMessage: ChatMessage = {
	...MockChatMessage,
	id: 3,
	role: "tool",
	content: [
		{
			type: "tool-result",
			tool_call_id: "summary-1",
			tool_name: "chat_summarized",
			result: {
				summary: "Compacted conversation",
				source: "manual",
				context_tokens: 90000,
				context_limit_tokens: 100000,
				estimated_context_tokens: 12000,
			},
		},
	],
};

export const MockChatQueuedMessage: ChatQueuedMessage = {
	id: 1,
	chat_id: "chat-1",
	content: [{ type: "text", text: "Queued message" }],
	created_at: MOCK_TIMESTAMP,
};

export const MockChatAutomation: ChatAutomation = {
	id: "7f1c2b9e-4d3a-4c1f-9b2e-5a6d7e8f9a0b",
	organization_id: "test-org-id",
	owner_id: MockUserOwner.id,
	name: "CI heartbeat",
	kind: "schedule",
	enabled: true,
	target_mode: "existing_chat",
	target_chat_id: "chat-1",
	when_busy: "queue",
	webhook_secret_version: 0,
	prompt: "Check the nightly build.",
	schedule_cron: "0 9 * * *",
	schedule_time_zone: "UTC",
	next_run_times: [],
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockWebhookChatAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "2b8e4f6a-1c3d-4e5f-8a9b-0c1d2e3f4a5b",
	name: "Deploy notifier",
	kind: "webhook",
	webhook_use: "multi",
	webhook_secret_version: 1,
	prompt: "Summarize the deploy event.",
	schedule_cron: undefined,
	schedule_time_zone: undefined,
};

export const MockChatProject: ChatProject = {
	id: "4c5d6e7f-8a9b-4c0d-9e1f-2a3b4c5d6e7f",
	organization_id: "test-org-id",
	owner_id: MockUserOwner.id,
	name: "Release work",
	description: "",
	icon: "",
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};
