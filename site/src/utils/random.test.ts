import { describe, expect, it } from "vitest";
import { generateSessionId } from "#/utils/random";

describe("generateSessionId", () => {
	it("is 32 characters long", () => {
		const id = generateSessionId();
		expect(id.length).toBe(32);
	});

	it("outputs a hexadecimal string", () => {
		const id = generateSessionId();
		const hexRegex = /^[\da-f]+$/;
		expect(hexRegex.test(id)).toBe(true);
	});

	it("generates unique values across calls", () => {
		const numValues = 1000;
		const ids = new Set(
			Array.from({ length: numValues }, () => generateSessionId()),
		);
		expect(ids.size).toBe(numValues);
	});
});
