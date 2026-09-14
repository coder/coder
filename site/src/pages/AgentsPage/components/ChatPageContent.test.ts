import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	workspaceMCPServersFromChat,
	workspaceSkillsFromChat,
} from "./ChatPageContent";

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

const chatWithContext = (
	context: TypesGen.ChatContext | undefined,
): TypesGen.Chat => ({ ...MockChat, context });

describe("workspaceSkillsFromChat", () => {
	it("returns undefined while the chat detail is unresolved", () => {
		expect(workspaceSkillsFromChat(undefined)).toBeUndefined();
	});

	it("returns an empty authoritative list for a resolved unpinned chat", () => {
		expect(workspaceSkillsFromChat(chatWithContext(undefined))).toEqual([]);
		expect(workspaceSkillsFromChat(chatWithContext({ dirty: false }))).toEqual(
			[],
		);
	});

	it("maps healthy skill resources to workspace skills", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				instructionResource(),
				skillResource("reviewer"),
				skillResource("docs"),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
			{ name: "docs", description: "docs description" },
		]);
	});

	it("keeps the first resource for duplicate skill names, matching read_skill", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				skillResource("reviewer", {
					source: "/workspace/.agents/skills/reviewer",
				}),
				skillResource("reviewer", {
					source: "/workspace/other/skills/reviewer",
					skill_description: "shadowed duplicate",
				}),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("omits non-ok skill resources", () => {
		const chat = chatWithContext({
			dirty: true,
			resources: [
				skillResource("reviewer"),
				skillResource("broken", { status: "unreadable", skill_name: "" }),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("returns an empty authoritative list when pinned context has no skills", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [instructionResource()],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([]);
	});
});

const mcpServerResource = (
	source: string,
	toolNames: string[],
	status: TypesGen.ChatContextResource["status"] = "ok",
): TypesGen.ChatContextResource => ({
	source,
	kind: "mcp_server",
	size_bytes: 32,
	status,
	tools: toolNames.map((name) => ({ name, description: "" })),
});

describe("workspaceMCPServersFromChat", () => {
	it("lists ok mcp_server resources sorted by name with tool counts", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				instructionResource(),
				mcpServerResource("linear", ["list_issues"]),
				mcpServerResource("github", ["create_issue", "search"]),
				mcpServerResource("broken", [], "invalid"),
			],
		});
		expect(workspaceMCPServersFromChat(undefined)).toEqual([]);
		expect(workspaceMCPServersFromChat(chat)).toEqual([
			{ name: "github", toolCount: 2 },
			{ name: "linear", toolCount: 1 },
		]);
	});
});
