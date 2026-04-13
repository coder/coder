import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useParsedDiffFiles } from "./useParsedDiffFiles";

const sampleDiff = [
	"diff --git a/src/main.ts b/src/main.ts",
	"index abc1234..def5678 100644",
	"--- a/src/main.ts",
	"+++ b/src/main.ts",
	"@@ -1,3 +1,3 @@",
	" const x = 1;",
	"-const y = 2;",
	"+const y = 42;",
	" const z = 3;",
].join("\n");

describe("useParsedDiffFiles", () => {
	it("returns the same parsed file reference when inputs are unchanged", () => {
		const { result, rerender } = renderHook(
			({ diff, cacheKeyPrefix }: { diff?: string; cacheKeyPrefix?: string }) =>
				useParsedDiffFiles(diff, cacheKeyPrefix),
			{
				initialProps: {
					diff: sampleDiff,
					cacheKeyPrefix: "chat-1",
				},
			},
		);

		const firstResult = result.current;
		rerender({ diff: sampleDiff, cacheKeyPrefix: "chat-1" });

		expect(result.current).toBe(firstResult);
		expect(result.current[0]?.name).toBe("src/main.ts");
	});

	it("re-parses when the cache key changes", () => {
		const { result, rerender } = renderHook(
			({ diff, cacheKeyPrefix }: { diff?: string; cacheKeyPrefix?: string }) =>
				useParsedDiffFiles(diff, cacheKeyPrefix),
			{
				initialProps: {
					diff: sampleDiff,
					cacheKeyPrefix: "chat-1",
				},
			},
		);

		const firstResult = result.current;
		rerender({ diff: sampleDiff, cacheKeyPrefix: "chat-2" });

		expect(result.current).not.toBe(firstResult);
		expect(result.current[0]?.cacheKey).not.toBe(firstResult[0]?.cacheKey);
	});

	it("returns an empty result for invalid patches", () => {
		const { result } = renderHook(() => useParsedDiffFiles("not a diff"));
		expect(result.current).toEqual([]);
	});
});
