import {
	getContentCacheKeyPrefix,
	getDiffCacheKeyPrefix,
} from "./diffCacheKey";

describe("getDiffCacheKeyPrefix", () => {
	it("returns the same key for the same scope and query update time", () => {
		expect(getDiffCacheKeyPrefix("chat-123", 101)).toBe(
			getDiffCacheKeyPrefix("chat-123", 101),
		);
	});

	it("changes when the query update time changes", () => {
		expect(getDiffCacheKeyPrefix("chat-123", 101)).not.toBe(
			getDiffCacheKeyPrefix("chat-123", 202),
		);
	});

	it("changes when the scope changes", () => {
		expect(getDiffCacheKeyPrefix("chat-123", 101)).not.toBe(
			getDiffCacheKeyPrefix("chat-456", 101),
		);
	});
});

describe("getContentCacheKeyPrefix", () => {
	it("returns the same prefix for identical text", () => {
		expect(getContentCacheKeyPrefix("--- a\n+++ a\n")).toBe(
			getContentCacheKeyPrefix("--- a\n+++ a\n"),
		);
	});

	it("returns different prefixes for different text", () => {
		expect(getContentCacheKeyPrefix("--- a\n+++ a\n-x\n+y\n")).not.toBe(
			getContentCacheKeyPrefix("--- a\n+++ a\n-x\n+z\n"),
		);
	});

	it("prefixes keys so they never collide with bare file names", () => {
		expect(getContentCacheKeyPrefix("anything")).toMatch(/^content-[0-9a-f]+$/);
	});
});
