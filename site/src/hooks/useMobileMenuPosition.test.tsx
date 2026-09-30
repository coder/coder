import { describe, expect, it } from "vitest";
import { getMobileMenuPosition } from "./useMobileMenuPosition";

// Layout viewport 800px tall (the hook reads it from a fixed probe); the
// composer sits at the bottom unless a case says otherwise.
const layoutViewportBottom = 800;
const composer = { left: 16, top: 600, width: 358 };

describe("getMobileMenuPosition", () => {
	it.each([
		{
			name: "keyboard closed: 8px above the composer, capped 16px below the top",
			anchor: composer,
			viewport: { offsetTop: 0, height: 800 },
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 576 },
		},
		{
			name: "keyboard covering the composer: lifted above the keyboard",
			anchor: { ...composer, top: 650 },
			viewport: { offsetTop: 0, height: 500 },
			expected: { left: 16, width: 358, bottom: 308, maxHeight: 476 },
		},
		{
			name: "visual viewport panned down: cap shrinks by the pan offset",
			anchor: composer,
			viewport: { offsetTop: 100, height: 700 },
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 476 },
		},
		{
			name: "offset larger than the space above: ignored as a settling WebKit value",
			anchor: composer,
			viewport: { offsetTop: 650, height: 800 },
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 576 },
		},
		{
			name: "composer near the top: height floors at 96px",
			anchor: { ...composer, top: 60 },
			viewport: { offsetTop: 0, height: 800 },
			expected: { left: 16, width: 358, bottom: 748, maxHeight: 96 },
		},
		{
			name: "no visual viewport: layout viewport only",
			anchor: composer,
			viewport: null,
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 576 },
		},
	])("$name", ({ anchor, viewport, expected }) => {
		expect(
			getMobileMenuPosition(anchor, layoutViewportBottom, viewport),
		).toEqual(expected);
	});
});
