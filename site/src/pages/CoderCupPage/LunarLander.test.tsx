import { render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LunarLander } from "./LunarLander";

vi.mock("./roster", () => ({
	ROSTER: [
		{ name: "Nova Moonfern", role: "Community Member", isCommunity: true },
		{ name: "Employee", role: "Community Team Lead" },
	],
}));

vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");

afterEach(() => {
	vi.restoreAllMocks();
});

it("carries community membership from the roster into sidebar and suit drawing", () => {
	vi.spyOn(Math, "random").mockReturnValue(0.5);
	vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
		new DOMRect(0, 0, 1200, 800),
	);
	const frames: FrameRequestCallback[] = [];
	vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
		frames.push(callback);
		return frames.length;
	});
	vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
	const context = document.createElement("canvas").getContext("2d");
	if (!context) throw new Error("Canvas context unavailable");
	vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(context);
	const fillText = vi.spyOn(context, "fillText");
	const suitColors = new Set<string | CanvasGradient | CanvasPattern>();
	vi.spyOn(context, "stroke").mockImplementation(() => {
		suitColors.add(context.strokeStyle);
	});

	const { unmount } = render(<LunarLander />);
	// Advance past the intro, then draw a normal game frame.
	for (const timestamp of [1, 8001, 8017]) {
		const frame = frames.shift();
		if (!frame) throw new Error("Animation frame not scheduled");
		frame(timestamp);
	}

	expect(fillText).toHaveBeenCalledWith(
		"Nova Moonfern (community)",
		expect.any(Number),
		expect.any(Number),
	);
	expect(fillText).toHaveBeenCalledWith(
		"Employee",
		expect.any(Number),
		expect.any(Number),
	);
	expect(suitColors).toContain("#666666");
	expect(suitColors).toContain("#888888");
	unmount();
});
