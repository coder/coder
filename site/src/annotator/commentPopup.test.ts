import { describe, expect, it } from "vitest";
import { popupPosition } from "./commentPopup";

const viewport = { innerWidth: 1000, innerHeight: 600 };

describe("popupPosition", () => {
	it("sits just below the target, kept inside the viewport", () => {
		expect(popupPosition(new DOMRect(900, 100, 50, 20), viewport)).toEqual({
			left: 1000 - 384 - 8,
			top: 128,
		});
	});

	it("flips above the target when it would not fit below", () => {
		expect(popupPosition(new DOMRect(20, 500, 50, 20), viewport)).toEqual({
			left: 20,
			top: 500 - 220 - 8,
		});
	});
});
