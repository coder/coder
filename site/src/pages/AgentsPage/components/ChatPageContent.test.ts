import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { workspaceSkillsFromChatContext } from "./ChatPageContent";

const skillResource = (
	name: string,
	overrides: Partial<TypesGen.ChatContextResource> = {},
): TypesGen.ChatContextResource => ({
	source: `/workspace/.agents/skills/${name}`,
	kind: "skill",
	size_bytes: 128,
	skill_name: name,
	skill_description: `${name} description`,
	status: "ok",
	...overrides,
});

const instructionResource = (): TypesGen.ChatContextResource => ({
	source: "/workspace/AGENTS.md",
	kind: "instruction_file",
	size_bytes: 64,
	status: "ok",
});

describe("workspaceSkillsFromChatContext", () => {
	it("returns undefined without pinned resources", () => {
		expect(workspaceSkillsFromChatContext(undefined)).toBeUndefined();
		expect(workspaceSkillsFromChatContext({ dirty: false })).toBeUndefined();
	});

	it("maps healthy skill resources to workspace skills", () => {
		const context: TypesGen.ChatContext = {
			dirty: false,
			resources: [
				instructionResource(),
				skillResource("reviewer"),
				skillResource("docs"),
			],
		};
		expect(workspaceSkillsFromChatContext(context)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
			{ name: "docs", description: "docs description" },
		]);
	});

	it("omits non-ok skill resources", () => {
		const context: TypesGen.ChatContext = {
			dirty: true,
			resources: [
				skillResource("reviewer"),
				skillResource("broken", { status: "unreadable", skill_name: "" }),
			],
		};
		expect(workspaceSkillsFromChatContext(context)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("returns an empty authoritative list when pinned context has no skills", () => {
		const context: TypesGen.ChatContext = {
			dirty: false,
			resources: [instructionResource()],
		};
		expect(workspaceSkillsFromChatContext(context)).toEqual([]);
	});
});
