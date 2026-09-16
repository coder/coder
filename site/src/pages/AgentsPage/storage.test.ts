import { beforeEach, describe, expect, it } from "vitest";
import { rightPanelWidthAtom } from "#/pages/AgentsPage/atoms";

describe("rightPanelWidthAtom", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("decodes fractional widths written by pre-upgrade builds", () => {
		localStorage.setItem("agents.right-panel-width", "479.5");
		expect(rightPanelWidthAtom.get()).toBe(480);
	});

	it("falls back to the default for non-numeric values", () => {
		localStorage.setItem("agents.right-panel-width", "wide");
		expect(rightPanelWidthAtom.get()).toBeNull();
	});
});
