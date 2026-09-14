import { getContentCacheKey } from "./diffCacheKey";

describe("getContentCacheKey", () => {
	it("returns the same key for identical text", () => {
		expect(getContentCacheKey("--- a\n+++ a\n")).toBe(
			getContentCacheKey("--- a\n+++ a\n"),
		);
	});

	it("returns different keys for different text", () => {
		expect(getContentCacheKey("--- a\n+++ a\n-x\n+y\n")).not.toBe(
			getContentCacheKey("--- a\n+++ a\n-x\n+z\n"),
		);
	});

	it("formats keys as content-<hex hash>-<hex length>", () => {
		expect(getContentCacheKey("anything")).toMatch(
			/^content-[0-9a-f]+-[0-9a-f]+$/,
		);
	});
});
