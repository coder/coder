import { describe, expect, it } from "vitest";
import type { ChatContextResource } from "#/api/typesGenerated";
import { MockChatContextClean } from "#/testHelpers/chatEntities";
import { getContextInventory, groupContextResources } from "./chatDetails";

describe("getContextInventory", () => {
	it("counts usable files and skills independently, retaining rejected and nameless resources as issues", () => {
		const result = getContextInventory({
			...MockChatContextClean,
			resources: [
				...(MockChatContextClean.resources ?? []),
				{ kind: "instruction_file", source: " ", status: "ok", size_bytes: 0 },
				{
					kind: "skill",
					source: "",
					skill_name: " ",
					status: "ok",
					size_bytes: 0,
				},
				{
					kind: "skill",
					source: "/project/skill",
					status: "excluded",
					size_bytes: 1,
				},
			],
		});
		expect(result.files).toHaveLength(1);
		expect(result.skills).toHaveLength(1);
		expect(result.issues).toHaveLength(4);
	});
	it("counts zero-tool and failed MCP servers, not configs or tools", () => {
		const result = getContextInventory({
			dirty: false,
			resources: [
				{
					kind: "mcp_config",
					source: "/project/.mcp.json",
					status: "ok",
					size_bytes: 100,
				},
				{
					kind: "mcp_server",
					source: "empty",
					status: "ok",
					size_bytes: 0,
					tools: [],
				},
				{
					kind: "mcp_server",
					source: "failed",
					status: "unreadable",
					size_bytes: 0,
					error: "Connection refused",
				},
			],
		});
		expect(result.servers).toHaveLength(2);
		expect(result.connectedServers).toBe(1);
		expect(result.servers[1]?.resource.error).toBe("Connection refused");
	});
	it("distinguishes missing inventory from a known empty inventory", () => {
		expect(getContextInventory(undefined).known).toBe(false);
		expect(getContextInventory({ dirty: false }).known).toBe(false);
		expect(getContextInventory({ dirty: false, resources: [] }).known).toBe(
			true,
		);
	});
	it("preserves first-seen directory order and identical basenames from separate roots", () => {
		const resources: ChatContextResource[] = [
			"/b/AGENTS.md",
			"/a/AGENTS.md",
			"/b/README.md",
		].map((source) => ({
			source,
			kind: "instruction_file",
			status: "ok",
			size_bytes: 1,
		}));
		const groups = groupContextResources(
			getContextInventory({ dirty: false, resources }).files,
		);
		expect(groups.map(({ dir }) => dir)).toEqual(["/b", "/a"]);
		expect(groups[0]?.items.map(({ resource }) => resource.source)).toEqual([
			"/b/AGENTS.md",
			"/b/README.md",
		]);
	});
});
