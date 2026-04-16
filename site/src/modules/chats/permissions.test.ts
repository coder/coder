import type { Chat } from "#/api/typesGenerated";
import { type ChatPermissions, chatChecks } from "./permissions";

const makeChat = (overrides: Partial<Chat> = {}): Chat =>
	({
		id: "chat-1",
		owner_id: "owner-1",
		organization_id: "org-1",
		title: "hello",
		status: "completed",
		last_model_config_id: "model-1",
		mcp_server_ids: [],
		labels: {},
		created_at: "2024-01-01T00:00:00Z",
		updated_at: "2024-01-01T00:00:00Z",
		archived: false,
		pin_order: 0,
		has_unread: false,
		client_type: "ui",
		last_error: null,
		...overrides,
	}) as Chat;

describe("chatChecks", () => {
	it("scopes each check to the chat's owner and id", () => {
		const chat = makeChat({ id: "chat-xyz", owner_id: "me" });
		const checks = chatChecks(chat);

		for (const check of Object.values(checks)) {
			expect(check.object.resource_type).toBe("chat");
			expect(check.object.resource_id).toBe("chat-xyz");
			expect(check.object.owner_id).toBe("me");
		}
	});

	it("uses the expected RBAC actions", () => {
		const checks = chatChecks(makeChat());
		expect(checks.readChat.action).toBe("read");
		expect(checks.shareChat.action).toBe("share");
		expect(checks.updateChat.action).toBe("update");
	});

	it("re-scopes to a different owner when the chat is not mine", () => {
		const chat = makeChat({ id: "chat-xyz", owner_id: "someone-else" });
		const checks = chatChecks(chat);
		for (const check of Object.values(checks)) {
			expect(check.object.owner_id).toBe("someone-else");
		}
	});

	it("produces a stable key set matching ChatPermissions", () => {
		const keys: Array<keyof ChatPermissions> = Object.keys(
			chatChecks(makeChat()),
		) as Array<keyof ChatPermissions>;
		expect(new Set(keys)).toEqual(
			new Set<keyof ChatPermissions>(["readChat", "shareChat", "updateChat"]),
		);
	});
});
