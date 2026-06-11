import { describe, expect, it } from "vitest";
import { getActiveFile, type ScrollViewer } from "./useActiveFileTracking";

// Builds a ScrollViewer stub from a path -> top map. A top of `undefined`
// models an item the library has not measured yet.
function viewerFrom(
	tops: ReadonlyArray<readonly [string, number | undefined]>,
): ScrollViewer {
	const map = new Map(tops);
	return {
		getRenderedItems: () => tops.map(([id]) => ({ id })),
		getTopForItem: (id) => map.get(id),
	};
}

describe("getActiveFile", () => {
	it("picks the item closest to the top that has crossed it", () => {
		const viewer = viewerFrom([
			["a.ts", 0],
			["b.ts", 300],
			["c.ts", 900],
		]);
		// At scrollTop 500, b.ts (top 300) is the last file whose start has
		// scrolled past the fold; c.ts (top 900) is still below it.
		expect(getActiveFile(500, viewer)).toBe("b.ts");
	});

	it("treats an item within the threshold slack as already active", () => {
		const viewer = viewerFrom([
			["a.ts", 0],
			// 4px below the fold, inside ACTIVE_FILE_SCROLL_THRESHOLD.
			["b.ts", 104],
		]);
		expect(getActiveFile(100, viewer)).toBe("b.ts");
	});

	it("excludes items past the threshold slack", () => {
		const viewer = viewerFrom([
			["a.ts", 0],
			// 5px below the fold, just outside the threshold.
			["b.ts", 105],
		]);
		expect(getActiveFile(100, viewer)).toBe("a.ts");
	});

	it("falls back to the first rendered item when none have crossed", () => {
		const viewer = viewerFrom([
			["a.ts", 300],
			["b.ts", 400],
		]);
		expect(getActiveFile(0, viewer)).toBe("a.ts");
	});

	it("ignores unmeasured items", () => {
		const viewer = viewerFrom([
			["a.ts", -10],
			["b.ts", undefined],
		]);
		expect(getActiveFile(0, viewer)).toBe("a.ts");
	});

	it("returns undefined when nothing is rendered", () => {
		expect(getActiveFile(0, viewerFrom([]))).toBeUndefined();
	});
});
