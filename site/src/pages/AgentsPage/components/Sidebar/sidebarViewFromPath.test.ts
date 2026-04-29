import { describe, expect, it } from "vitest";
import { isSettingsView, sidebarViewFromPath } from "./AgentsSidebar";

describe("sidebarViewFromPath", () => {
	it("returns chats for the agents index", () => {
		expect(sidebarViewFromPath("/agents")).toEqual({ panel: "chats" });
	});

	it("returns analytics for the analytics route", () => {
		expect(sidebarViewFromPath("/agents/analytics")).toEqual({
			panel: "analytics",
		});
	});

	it("returns chats for non-settings agent routes", () => {
		expect(sidebarViewFromPath("/agents/some-uuid")).toEqual({
			panel: "chats",
		});
	});

	it("returns the settings index for /agents/settings", () => {
		expect(sidebarViewFromPath("/agents/settings")).toEqual({
			panel: "settings",
			section: undefined,
		});
	});

	it("returns the general settings section", () => {
		expect(sidebarViewFromPath("/agents/settings/general")).toEqual({
			panel: "settings",
			section: "general",
		});
	});

	it("returns the compaction settings section", () => {
		expect(sidebarViewFromPath("/agents/settings/compaction")).toEqual({
			panel: "settings",
			section: "compaction",
		});
	});

	it("returns the api keys settings section", () => {
		expect(sidebarViewFromPath("/agents/settings/api-keys")).toEqual({
			panel: "settings",
			section: "api-keys",
		});
	});

	it("returns the lifecycle admin settings section", () => {
		expect(sidebarViewFromPath("/agents/settings/lifecycle")).toEqual({
			panel: "settings-admin",
			section: "lifecycle",
		});
	});

	it("returns the providers admin settings section", () => {
		expect(sidebarViewFromPath("/agents/settings/providers")).toEqual({
			panel: "settings-admin",
			section: "providers",
		});
	});

	it("normalizes the admin index route to an undefined section", () => {
		expect(sidebarViewFromPath("/agents/settings/admin")).toEqual({
			panel: "settings-admin",
			section: undefined,
		});
	});

	it("returns the instructions admin settings section", () => {
		expect(sidebarViewFromPath("/agents/settings/instructions")).toEqual({
			panel: "settings-admin",
			section: "instructions",
		});
	});

	it("falls through unknown settings slugs to the user settings panel", () => {
		expect(sidebarViewFromPath("/agents/settings/unknown-slug")).toEqual({
			panel: "settings",
			section: "unknown-slug",
		});
	});

	it("falls back to chats for unrelated routes", () => {
		expect(sidebarViewFromPath("/workspaces")).toEqual({
			panel: "chats",
		});
	});
});

describe("isSettingsView", () => {
	it("returns true for the user settings panel", () => {
		expect(isSettingsView({ panel: "settings", section: undefined })).toBe(
			true,
		);
	});

	it("returns true for the admin settings panel", () => {
		expect(
			isSettingsView({ panel: "settings-admin", section: "providers" }),
		).toBe(true);
	});

	it("returns false for chats", () => {
		expect(isSettingsView({ panel: "chats" })).toBe(false);
	});

	it("returns false for analytics", () => {
		expect(isSettingsView({ panel: "analytics" })).toBe(false);
	});
});
