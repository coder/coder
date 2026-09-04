import { describe, expect, it } from "vitest";
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
});
