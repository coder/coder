// Pure layout math for the windowing renderer. Kept free of DOM and React so it
// can be unit tested and reused by the container without re-rendering.

// cumulativeOffsets returns N+1 entries: the running top offset of each item,
// with the final entry holding the total height.
export function cumulativeOffsets(heights: readonly number[]): number[] {
	const offsets = new Array<number>(heights.length + 1);
	offsets[0] = 0;
	for (let i = 0; i < heights.length; i++) {
		offsets[i + 1] = offsets[i] + heights[i];
	}
	return offsets;
}

type WindowInput = {
	offsets: readonly number[];
	scrollTop: number;
	viewportHeight: number;
	overscan: number;
};

type Window = {
	start: number;
	end: number;
	topPad: number;
	bottomPad: number;
};

// computeWindow returns the inclusive [start, end] item range to render plus the
// spacer heights that reserve the unrendered ranges. The range covers the
// viewport expanded by `overscan` on each side. An empty list yields an empty
// range (end = -1) and zero pads.
export function computeWindow({
	offsets,
	scrollTop,
	viewportHeight,
	overscan,
}: WindowInput): Window {
	const count = offsets.length - 1;
	const total = offsets[count];
	if (count <= 0) {
		return { start: 0, end: -1, topPad: 0, bottomPad: 0 };
	}

	const windowTop = scrollTop - overscan;
	const windowBottom = scrollTop + viewportHeight + overscan;

	// First item whose bottom edge is past the window top.
	let start = 0;
	while (start < count - 1 && offsets[start + 1] <= windowTop) {
		start++;
	}

	// Last item whose top edge is before the window bottom.
	let end = start;
	while (end < count - 1 && offsets[end + 1] < windowBottom) {
		end++;
	}

	return {
		start,
		end,
		topPad: offsets[start],
		bottomPad: total - offsets[end + 1],
	};
}
