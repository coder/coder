import { getDiffCacheKeyPrefix } from "./diffCacheKey";

describe("getDiffCacheKeyPrefix", () => {
	it("returns the same key for the same scope and diff contents", () => {
		const diff = "diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n";

		expect(getDiffCacheKeyPrefix("chat-123", diff)).toBe(
			getDiffCacheKeyPrefix("chat-123", diff),
		);
	});

	it("changes when the diff contents change", () => {
		const original = "diff --git a/a.ts b/a.ts\n-old\n+new\n";
		const updated = "diff --git a/a.ts b/a.ts\n-old\n+newer\n";

		expect(getDiffCacheKeyPrefix("chat-123", original)).not.toBe(
			getDiffCacheKeyPrefix("chat-123", updated),
		);
	});

	it("changes when the scope changes", () => {
		const diff = "diff --git a/a.ts b/a.ts\n-old\n+new\n";

		expect(getDiffCacheKeyPrefix("chat-123", diff)).not.toBe(
			getDiffCacheKeyPrefix("chat-456", diff),
		);
	});
});
