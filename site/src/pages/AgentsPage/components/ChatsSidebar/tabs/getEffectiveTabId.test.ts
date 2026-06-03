import { describe, expect, it } from "vitest";
import { getEffectiveTabId } from "./getEffectiveTabId";

describe("getEffectiveTabId", () => {
	it("returns the active tab id when it matches a known tab", () => {
		expect(
			getEffectiveTabId(["git", "terminal", "debug"], "debug", undefined),
		).toBe("debug");
	});

	it("falls back to the first tab when the active id is unknown", () => {
		expect(getEffectiveTabId(["git", "terminal"], "missing", undefined)).toBe(
			"git",
		);
	});

	it("falls back to the first tab when no active id is set", () => {
		expect(getEffectiveTabId(["git", "terminal"], null, undefined)).toBe("git");
	});

	it("resolves to desktop when it is the active id and desktopChatId is set", () => {
		expect(getEffectiveTabId(["git"], "desktop", "desktop-123")).toBe(
			"desktop",
		);
	});

	it("returns desktop when the tab list is empty but desktop is available", () => {
		expect(getEffectiveTabId([], null, "desktop-123")).toBe("desktop");
	});

	it("returns null when no tabs are available", () => {
		expect(getEffectiveTabId([], null, undefined)).toBeNull();
	});

	it("ignores an unknown active id when only desktop is available", () => {
		expect(getEffectiveTabId([], "git", "desktop-123")).toBe("desktop");
	});
});
