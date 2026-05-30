import { describe, expect, it } from "vitest";
import { computeWindow, cumulativeOffsets } from "./windowMath";

describe("cumulativeOffsets", () => {
	it("returns N+1 offsets with the total last", () => {
		expect(cumulativeOffsets([100, 200, 50])).toEqual([0, 100, 300, 350]);
	});
	it("returns [0] for an empty list", () => {
		expect(cumulativeOffsets([])).toEqual([0]);
	});
});

describe("computeWindow", () => {
	const overscan = 1000;

	it("renders the whole list when it fits the window", () => {
		const offsets = cumulativeOffsets([100, 200, 50]);
		expect(
			computeWindow({ offsets, scrollTop: 0, viewportHeight: 1000, overscan }),
		).toEqual({ start: 0, end: 2, topPad: 0, bottomPad: 0 });
	});

	it("windows the middle of a long list with overscan", () => {
		// 100 items of 100px each, total 10000.
		const offsets = cumulativeOffsets(Array.from({ length: 100 }, () => 100));
		const win = computeWindow({
			offsets,
			scrollTop: 5000,
			viewportHeight: 500,
			overscan,
		});
		// windowTop = 4000, windowBottom = 6500.
		expect(win.start).toBe(40);
		expect(win.end).toBe(64);
		expect(win.topPad).toBe(4000);
		expect(win.bottomPad).toBe(10000 - 6500);
	});

	it("clamps the top: scrollTop 0 gives start 0 and no top pad", () => {
		const offsets = cumulativeOffsets(Array.from({ length: 100 }, () => 100));
		const win = computeWindow({
			offsets,
			scrollTop: 0,
			viewportHeight: 500,
			overscan,
		});
		expect(win.start).toBe(0);
		expect(win.topPad).toBe(0);
	});

	it("clamps the bottom: at max scroll, end is last and no bottom pad", () => {
		const offsets = cumulativeOffsets(Array.from({ length: 100 }, () => 100));
		const win = computeWindow({
			offsets,
			scrollTop: 10000 - 500,
			viewportHeight: 500,
			overscan,
		});
		expect(win.end).toBe(99);
		expect(win.bottomPad).toBe(0);
	});

	it("returns an empty window for an empty list", () => {
		expect(
			computeWindow({
				offsets: [0],
				scrollTop: 0,
				viewportHeight: 500,
				overscan,
			}),
		).toEqual({ start: 0, end: -1, topPad: 0, bottomPad: 0 });
	});
});
