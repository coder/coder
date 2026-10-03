import { afterEach, describe, expect, it, vi } from "vitest";
import {
	AGENTS_MAIN_PANEL_MIN_WIDTH,
	clampLeftSidebarWidth,
	getLeftSidebarMaxWidth,
	LEFT_SIDEBAR_MIN_WIDTH,
} from "./sidebarWidth";

const setViewportWidth = (width: number) => {
	vi.stubGlobal("innerWidth", width);
};

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("getLeftSidebarMaxWidth", () => {
	it("reserves room for the main panel when the viewport shrinks", () => {
		setViewportWidth(720);

		expect(getLeftSidebarMaxWidth()).toBe(720 - AGENTS_MAIN_PANEL_MIN_WIDTH);
		expect(clampLeftSidebarWidth(660)).toBe(720 - AGENTS_MAIN_PANEL_MIN_WIDTH);
	});

	it("never clamps below the left sidebar minimum", () => {
		setViewportWidth(560);

		expect(getLeftSidebarMaxWidth()).toBe(LEFT_SIDEBAR_MIN_WIDTH);
		expect(clampLeftSidebarWidth(660)).toBe(LEFT_SIDEBAR_MIN_WIDTH);
	});

	it("keeps the fixed max width on wide viewports", () => {
		setViewportWidth(1440);

		expect(getLeftSidebarMaxWidth()).toBe(660);
	});
});
