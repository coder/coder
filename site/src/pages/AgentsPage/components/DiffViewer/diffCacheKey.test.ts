import { getDiffCacheKeyPrefix } from "./diffCacheKey";

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
