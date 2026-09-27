import { describe, expect, it } from "vitest";
import {
	agentSettingsSectionFromSearch,
	withAgentSettingsSection,
	withoutAgentSettingsSection,
} from "./agentSettingsSection";

describe("agentSettingsSectionFromSearch", () => {
	it("returns undefined when the param is absent", () => {
		expect(
			agentSettingsSectionFromSearch("?archived=archived"),
		).toBeUndefined();
	});

	it("returns the requested section", () => {
		expect(agentSettingsSectionFromSearch("?settings=api-keys")).toBe(
			"api-keys",
		);
	});

	it("falls back to the first section for unknown slugs", () => {
		expect(agentSettingsSectionFromSearch("?settings=nope")).toBe("general");
		expect(agentSettingsSectionFromSearch("?settings=")).toBe("general");
	});
});

describe("withAgentSettingsSection", () => {
	it("keeps the other params", () => {
		expect(withAgentSettingsSection("?archived=archived", "compaction")).toBe(
			"?archived=archived&settings=compaction",
		);
	});

	it("replaces an existing section", () => {
		expect(withAgentSettingsSection("?settings=general", "api-keys")).toBe(
			"?settings=api-keys",
		);
	});
});

describe("withoutAgentSettingsSection", () => {
	it("removes only the settings param", () => {
		expect(
			withoutAgentSettingsSection("?archived=archived&settings=general"),
		).toBe("?archived=archived");
	});

	it("returns an empty string when no params remain", () => {
		expect(withoutAgentSettingsSection("?settings=general")).toBe("");
	});
});
