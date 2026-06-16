import type { Chat } from "#/api/typesGenerated";
import { MockUserOwner } from "./entities";

const MOCK_TIMESTAMP = "2024-01-01T00:00:00Z";

/**
 * Builds a Chat for tests and stories. Defaults to a completed, owned root
 * chat; pass overrides for the fields a case cares about. Timestamps default
 * to a fixed value — pass created_at/updated_at when a case needs relative
 * ordering rather than relying on a shared default.
 */
export const makeChat = (overrides: Partial<Chat> = {}): Chat => ({
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
	...overrides,
});
