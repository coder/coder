import { describe, expect, it } from "vitest";
import { createHeightCache } from "./heightCache";

describe("createHeightCache", () => {
	it("uses a provided seed when a kind has no samples", () => {
		const cache = createHeightCache({ assistant: 200 });
		expect(cache.estimate("a1", "assistant")).toBe(200);
	});

	it("falls back to a built-in seed when none is provided", () => {
		const cache = createHeightCache();
		expect(cache.estimate("u1", "user")).toBe(80);
	});

	it("averages recorded heights per kind", () => {
		const cache = createHeightCache();
		cache.record("a1", "assistant", 100);
		cache.record("a2", "assistant", 300);
		expect(cache.estimate("a3", "assistant")).toBe(200);
	});

	it("returns the measured height for a known id via get and estimate", () => {
		const cache = createHeightCache();
		cache.record("a1", "assistant", 512);
		expect(cache.get("a1")).toBe(512);
		expect(cache.estimate("a1", "assistant")).toBe(512);
	});

	it("updates the average when an id is re-recorded, without double counting", () => {
		const cache = createHeightCache();
		cache.record("a1", "assistant", 100);
		cache.record("a1", "assistant", 200);
		expect(cache.estimate("a2", "assistant")).toBe(200);
	});

	it("returns undefined from get for an unknown id", () => {
		const cache = createHeightCache();
		expect(cache.get("nope")).toBeUndefined();
	});

	it("keeps averages per kind independent", () => {
		const cache = createHeightCache();
		cache.record("u1", "user", 60);
		cache.record("a1", "assistant", 400);
		expect(cache.estimate("u2", "user")).toBe(60);
		expect(cache.estimate("a2", "assistant")).toBe(400);
	});
});
