import { describe, expect, it } from "vitest";
import { generateSessionId } from "./sessionId";

describe("generateSessionId", () => {
	it("returns a 32-character lowercase hex string", () => {
		expect(generateSessionId()).toMatch(/^[0-9a-f]{32}$/);
	});

	it("returns a different value on each call", () => {
		expect(generateSessionId()).not.toEqual(generateSessionId());
	});
});
