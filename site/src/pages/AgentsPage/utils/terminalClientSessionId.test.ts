import { afterEach, describe, expect, it } from "vitest";
import {
	getTerminalClientSessionId,
	resetTerminalClientSessionIds,
} from "./terminalClientSessionId";

describe("getTerminalClientSessionId", () => {
	afterEach(() => {
		resetTerminalClientSessionIds();
	});

	it("returns a stable id for the same reconnection token across calls", () => {
		const first = getTerminalClientSessionId("token-a");
		const second = getTerminalClientSessionId("token-a");
		expect(second).toBe(first);
	});

	it("returns different ids for different reconnection tokens", () => {
		expect(getTerminalClientSessionId("token-a")).not.toBe(
			getTerminalClientSessionId("token-b"),
		);
	});

	it("generates a fresh id after reset, simulating a page reload", () => {
		const before = getTerminalClientSessionId("token-a");
		resetTerminalClientSessionIds();
		expect(getTerminalClientSessionId("token-a")).not.toBe(before);
	});

	it("produces a 32-character lowercase hexadecimal id", () => {
		expect(getTerminalClientSessionId("token-a")).toMatch(/^[0-9a-f]{32}$/);
	});
});
