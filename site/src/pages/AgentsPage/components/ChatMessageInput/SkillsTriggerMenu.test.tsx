import { describe, expect, it } from "vitest";
import {
	createPinnedSkillMenuItem,
	createSkillMenuItem,
	createSkillMenuItems,
} from "./SkillsTriggerMenu";

describe("createPinnedSkillMenuItem", () => {
	it("qualifies a skill with a plugin name as /plugin/<plugin>/<name>", () => {
		const item = createPinnedSkillMenuItem({
			name: "deploy",
			description: "Deploy via acme",
			pluginName: "acme",
		});
		expect(item.source).toBe("plugin");
		expect(item.pluginName).toBe("acme");
		expect(item.triggerText).toBe("/plugin/acme/deploy");
		expect(item.altTriggerText).toBe("/plugin/acme/deploy");
	});

	it("treats a skill without a plugin name as a workspace skill", () => {
		const item = createPinnedSkillMenuItem({ name: "deploy", description: "" });
		expect(item.source).toBe("workspace");
		expect(item.pluginName).toBeUndefined();
		expect(item.triggerText).toBe("/workspace/deploy");
	});
});

describe("createSkillMenuItem", () => {
	it("qualifies workspace skills as /workspace/<name>", () => {
		const item = createSkillMenuItem("workspace", {
			name: "deploy",
			description: "",
		});
		expect(item.triggerText).toBe("/workspace/deploy");
		expect(item.altTriggerText).toBe("/workspace/deploy");
	});

	it("keeps personal skills bare unless asked to qualify", () => {
		const skill = { name: "deploy", description: "" };
		expect(createSkillMenuItem("personal", skill).triggerText).toBe("/deploy");
		expect(createSkillMenuItem("personal", skill, true).triggerText).toBe(
			"/personal/deploy",
		);
	});
});

describe("createSkillMenuItems", () => {
	const personal = [{ name: "reviewer", description: "" }];

	it("qualifies a personal skill that shares a plugin skill's name", () => {
		const items = createSkillMenuItems(
			personal,
			[{ name: "reviewer", description: "", pluginName: "acme" }],
			true,
		);
		expect(items.personal.map((item) => item.triggerText)).toEqual([
			"/personal/reviewer",
		]);
		expect(items.workspace.map((item) => item.triggerText)).toEqual([
			"/plugin/acme/reviewer",
		]);
	});

	it("keeps a personal trigger bare when no workspace skill shares its name", () => {
		const items = createSkillMenuItems(
			personal,
			[{ name: "deploy", description: "", pluginName: "acme" }],
			true,
		);
		expect(items.personal.map((item) => item.triggerText)).toEqual([
			"/reviewer",
		]);
	});

	it("qualifies personal triggers while workspace skills are unknown", () => {
		const items = createSkillMenuItems(personal, [], false);
		expect(items.personal.map((item) => item.triggerText)).toEqual([
			"/personal/reviewer",
		]);
	});
});
