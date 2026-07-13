import type {
	Chat,
	ChatContext,
	ChatContextResource,
	ChatMessage,
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
	status: "waiting",
	last_turn_summary: null,
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

export const MockChatQueuedMessage: ChatQueuedMessage = {
	id: 1,
	chat_id: "chat-1",
	content: [{ type: "text", text: "Queued message" }],
	created_at: MOCK_TIMESTAMP,
};
