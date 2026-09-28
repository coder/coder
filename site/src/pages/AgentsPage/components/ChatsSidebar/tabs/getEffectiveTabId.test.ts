import { describe, expect, it } from "vitest";
import { getEffectiveTabId } from "./getEffectiveTabId";

describe("getEffectiveTabId", () => {
	it("returns the active tab id when it matches a known tab", () => {
		expect(getEffectiveTabId(["git", "terminal", "debug"], "debug")).toBe(
			"debug",
		);
	});

	it("falls back to the first tab when the active id is unknown", () => {
		expect(getEffectiveTabId(["git", "terminal"], "missing")).toBe("git");
	});

	it("falls back to the first tab when no active id is set", () => {
		expect(getEffectiveTabId(["git", "terminal"], null)).toBe("git");
	});

	it("resolves to desktop when it is the active id and desktop is available", () => {
		expect(getEffectiveTabId(["git", "desktop"], "desktop")).toBe("desktop");
	});

	it("ignores a hidden singleton tab that is still the stored selection", () => {
		expect(getEffectiveTabId(["summary", "git"], "desktop")).toBe("summary");
	});

	it("returns null when no tabs are available", () => {
		expect(getEffectiveTabId([], null)).toBeNull();
	});

	it("returns null when the active id has no matching tab", () => {
		expect(getEffectiveTabId([], "git")).toBeNull();
	});
});
