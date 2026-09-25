import { afterEach, describe, expect, it } from "vitest";
import { randomId } from "./randomId";

describe("randomId", () => {
	const randomUUID = crypto.randomUUID;
	afterEach(() => {
		Object.defineProperty(crypto, "randomUUID", {
			value: randomUUID,
			configurable: true,
		});
	});

	it("uses randomUUID when the context has it", () => {
		expect(randomId()).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
		);
	});

	it("still produces unique ids without randomUUID", () => {
		// Plain-HTTP previews are not a secure context and lack randomUUID.
		Object.defineProperty(crypto, "randomUUID", {
			value: undefined,
			configurable: true,
		});
		const ids = new Set(Array.from({ length: 50 }, randomId));
		expect(ids.size).toBe(50);
		for (const id of ids) {
			expect(id).toMatch(/^[0-9a-f]{32}$/);
		}
	});
});
