import type {
	Chat,
	ChatMessage,
	ChatQueuedMessage,
	MCPServerConfig,
} from "#/api/typesGenerated";
import { MockUserOwner } from "./entities";

const MOCK_TIMESTAMP = "2024-01-01T00:00:00Z";

/**
 * A Chat for tests and stories: a completed, owned root chat. Spread it and
 * override the fields a case cares about, e.g. `{ ...MockChat, id: "chat-2" }`.
 * Override created_at/updated_at when a case needs relative ordering rather
 * than relying on the shared default timestamp.
 */
export const MockChat: Chat = {
	id: "chat-1",
	organization_id: "test-org-id",
	owner_id: MockUserOwner.id,
	owner_username: MockUserOwner.username,
	owner_name: MockUserOwner.name,
	last_model_config_id: "model-config-1",
	title: "Agent",
	status: "completed",
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

/**
 * An MCPServerConfig for tests and stories: an enabled, streamable-HTTP server
 * with no auth. Spread it and override the fields a case cares about.
 */
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

/**
 * A ChatMessage for tests and stories: a user text message with a fixed
 * timestamp. Spread it and override the fields a case cares about. Override
 * created_at when a case needs relative ordering.
 */
export const MockChatMessage: ChatMessage = {
	id: 1,
	chat_id: "chat-1",
	created_at: MOCK_TIMESTAMP,
	role: "user",
	content: [{ type: "text", text: "Hello" }],
};

/**
 * A ChatQueuedMessage for tests and stories: a single text part with a fixed
 * timestamp. Spread it and override the fields a case cares about.
 */
export const MockChatQueuedMessage: ChatQueuedMessage = {
	id: 1,
	chat_id: "chat-1",
	content: [{ type: "text", text: "Queued message" }],
	created_at: MOCK_TIMESTAMP,
};
