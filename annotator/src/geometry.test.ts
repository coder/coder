import { describe, expect, it } from "vitest";
import { viewportBox } from "./geometry";

const win = { innerWidth: 800, innerHeight: 600 } as Window;

describe("viewportBox", () => {
	it("expands an on-screen rect by the inset", () => {
		const box = viewportBox(new DOMRect(100, 50, 200, 40), win, 4);
		expect(box).toEqual({
			left: 96,
			top: 46,
			width: 208,
			height: 48,
			clampedTop: false,
			clampedRight: false,
		});
	});

	it("clamps an oversized rect so every edge stays visible", () => {
		const box = viewportBox(new DOMRect(-50, -120, 1000, 900), win, 4);
		expect(box).toEqual({
			left: 4,
			top: 4,
			width: 792,
			height: 592,
			clampedTop: true,
			clampedRight: true,
		});
	});
});
