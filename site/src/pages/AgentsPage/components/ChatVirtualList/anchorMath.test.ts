import { describe, expect, it } from "vitest";
import { correctedScrollTop, isAtBottom } from "./anchorMath";

describe("isAtBottom", () => {
	it("is true at the exact bottom", () => {
		expect(isAtBottom(880, 1000, 120, 16)).toBe(true);
	});
	it("is true within the threshold", () => {
		expect(isAtBottom(870, 1000, 120, 16)).toBe(true);
	});
	it("is false above the threshold", () => {
		expect(isAtBottom(800, 1000, 120, 16)).toBe(false);
	});
});

describe("correctedScrollTop", () => {
	// Capture and restore at the same scroll position: a pure content change.
	it("adds the content-position delta so the anchor stays put", () => {
		// scrollTop 200, anchor offset 100 (content top 300); content above grew 40
		// so the anchor's content top is now 340.
		expect(correctedScrollTop(200, 300, 340)).toBe(240);
	});
	it("returns the same scrollTop when nothing moved", () => {
		expect(correctedScrollTop(200, 300, 300)).toBe(200);
	});
	// The Safari instability: the user scrolled between capture and restore. The
	// anchor's content top is unchanged, so there must be no correction.
	it("does not snap back when only a scroll happened between capture and restore", () => {
		// Captured at scrollTop 250, offset 50 (content top 300). Scrolled to 200
		// with no content change: content top is still 300.
		expect(correctedScrollTop(200, 300, 300)).toBe(200);
	});
	it("corrects only the content shift when a scroll and a content change coincide", () => {
		// Captured content top 300. Scrolled to 200, then content above grew 40, so
		// the anchor's content top is now 340. Only the 40 is corrected.
		expect(correctedScrollTop(200, 300, 340)).toBe(240);
	});
});
