import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { terminalWebsocketUrl } from "./terminal";

describe("terminalWebsocketUrl", () => {
	it("includes the client_session_id query parameter", async () => {
		const url = await terminalWebsocketUrl(
			undefined,
			"reconnect-token",
			"agent-id",
			undefined,
			24,
			80,
			undefined,
			undefined,
			"0123456789abcdef0123456789abcdef",
		);

		const parsed = new URL(url);
		expect(parsed.searchParams.get("client_session_id")).toBe(
			"0123456789abcdef0123456789abcdef",
		);
		expect(parsed.searchParams.get("reconnect")).toBe("reconnect-token");
	});

	it("adds a path separator after a base URL path", async () => {
		vi.spyOn(API, "issueReconnectingPTYSignedToken").mockResolvedValue({
			signed_token: "token",
		});
		const url = await terminalWebsocketUrl(
			"https://proxy.example.com/prefix",
			"reconnect-token",
			"agent-id",
			undefined,
			24,
			80,
			undefined,
			undefined,
			"session-id",
		);

		expect(new URL(url).pathname).toBe(
			"/prefix/api/v2/workspaceagents/agent-id/pty",
		);
	});
});
