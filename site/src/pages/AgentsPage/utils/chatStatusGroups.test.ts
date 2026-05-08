import type { Chat } from "#/api/typesGenerated";
import { getChatStatusGroup } from "./chatStatusGroups";

const baseChat: Chat = {
	id: "test-id",
	organization_id: "org-id",
	owner_id: "owner-id",
	title: "Test",
	status: "completed",
	last_model_config_id: "",
	mcp_server_ids: [],
	labels: {},
	created_at: "2026-01-01T00:00:00Z",
	updated_at: "2026-01-01T00:00:00Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_turn_summary: null,
	children: [],
};

describe("getChatStatusGroup", () => {
	it("returns Running for pending status", () => {
		expect(getChatStatusGroup({ ...baseChat, status: "pending" })).toBe(
			"Running",
		);
	});

	it("returns Running for running status", () => {
		expect(getChatStatusGroup({ ...baseChat, status: "running" })).toBe(
			"Running",
		);
	});

	it("returns Unread when has_unread is true and not running", () => {
		expect(
			getChatStatusGroup({ ...baseChat, has_unread: true }),
		).toBe("Unread");
	});

	it("returns Running over Unread when both apply", () => {
		expect(
			getChatStatusGroup({
				...baseChat,
				status: "running",
				has_unread: true,
			}),
		).toBe("Running");
	});

	it("returns Error for error status", () => {
		expect(getChatStatusGroup({ ...baseChat, status: "error" })).toBe(
			"Error",
		);
	});

	it("returns Archived for archived chats regardless of status", () => {
		expect(
			getChatStatusGroup({
				...baseChat,
				status: "running",
				archived: true,
			}),
		).toBe("Archived");
	});

	it("returns Awaiting feedback for paused status", () => {
		expect(getChatStatusGroup({ ...baseChat, status: "paused" })).toBe(
			"Awaiting feedback",
		);
	});

	it("returns Awaiting feedback for requires_action status", () => {
		expect(
			getChatStatusGroup({ ...baseChat, status: "requires_action" }),
		).toBe("Awaiting feedback");
	});

	it("returns Idle for completed status", () => {
		expect(getChatStatusGroup({ ...baseChat, status: "completed" })).toBe(
			"Idle",
		);
	});

	it("returns Idle for waiting status", () => {
		expect(getChatStatusGroup({ ...baseChat, status: "waiting" })).toBe(
			"Idle",
		);
	});
});
