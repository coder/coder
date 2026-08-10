import { getContentCacheKeyPrefix } from "./diffCacheKey";

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

	it("formats prefixes as content-<hex hash>-<hex length>", () => {
		expect(getContentCacheKeyPrefix("anything")).toMatch(
			/^content-[0-9a-f]+-[0-9a-f]+$/,
		);
	});
});
