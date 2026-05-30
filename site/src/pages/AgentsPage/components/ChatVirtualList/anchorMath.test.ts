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
	it("adds the offset delta so the anchor stays put", () => {
		expect(correctedScrollTop(200, 100, 140)).toBe(240);
	});
	it("returns the same scrollTop when nothing moved", () => {
		expect(correctedScrollTop(200, 100, 100)).toBe(200);
	});
});
