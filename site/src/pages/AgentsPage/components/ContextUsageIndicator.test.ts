import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { buildContextPanelModel } from "./ContextUsageIndicator";

const resource = (
	overrides: Partial<TypesGen.ChatContextResource> &
		Pick<TypesGen.ChatContextResource, "source" | "kind">,
): TypesGen.ChatContextResource => ({
	size_bytes: 128,
	status: "ok",
	...overrides,
});

const plainSkill = resource({
	source: "/workspace/.agents/skills/deploy",
	kind: "skill",
	skill_name: "deploy",
	skill_description: "Deploy the app.",
});

const pluginSkill = resource({
	source: "/home/coder/.coder/plugins/webtools/skills/fetch",
	kind: "skill",
	skill_name: "fetch",
	plugin_name: "webtools",
});

const otherPluginSkill = resource({
	source: "/home/coder/.coder/plugins/dbtools/skills/migrate",
	kind: "skill",
	skill_name: "migrate",
	plugin_name: "dbtools",
});

describe("buildContextPanelModel", () => {
	it("groups plugin skills under the plugin and plain skills under their directory", () => {
		const model = buildContextPanelModel([
			plainSkill,
			pluginSkill,
			otherPluginSkill,
		]);

		expect(model.skillGroups).toEqual([
			{
				label: { kind: "directory", name: "/workspace/.agents/skills" },
				items: [
					{
						source: plainSkill.source,
						name: "deploy",
						description: "Deploy the app.",
						group: { kind: "directory", name: "/workspace/.agents/skills" },
					},
				],
			},
			{
				label: { kind: "plugin", name: "webtools" },
				items: [
					{
						source: pluginSkill.source,
						name: "fetch",
						description: undefined,
						group: { kind: "plugin", name: "webtools" },
					},
				],
			},
			{
				label: { kind: "plugin", name: "dbtools" },
				items: [
					{
						source: otherPluginSkill.source,
						name: "migrate",
						description: undefined,
						group: { kind: "plugin", name: "dbtools" },
					},
				],
			},
		]);
	});

	it("strips the plugin prefix from plugin MCP server names and keeps order", () => {
		const tools: TypesGen.ChatContextTool[] = [{ name: "fetch_url" }];
		const model = buildContextPanelModel([
			resource({ source: "github", kind: "mcp_server", tools: [] }),
			resource({
				source: "webtools/tools",
				kind: "mcp_server",
				plugin_name: "webtools",
				tools,
			}),
			resource({
				source: "standalone",
				kind: "mcp_server",
				plugin_name: "dbtools",
			}),
		]);

		expect(model.mcpServers).toEqual([
			{ name: "github", source: "github", pluginName: undefined, tools: [] },
			{
				name: "tools",
				source: "webtools/tools",
				pluginName: "webtools",
				tools,
			},
			{
				name: "standalone",
				source: "standalone",
				pluginName: "dbtools",
				tools: [],
			},
		]);
	});

	it("lists OK plugins by plugin name, falling back to the source basename", () => {
		const model = buildContextPanelModel([
			resource({
				source: "/home/coder/.coder/plugins/webtools",
				kind: "plugin",
				plugin_name: "webtools",
				size_bytes: 300,
			}),
			resource({
				source: "/home/coder/.coder/plugins/unnamed",
				kind: "plugin",
				size_bytes: 200,
			}),
			resource({
				source: "/home/coder/.coder/plugins/broken",
				kind: "plugin",
				plugin_name: "broken",
				status: "invalid",
				error: "manifest missing name",
			}),
		]);

		expect(model.plugins).toEqual([
			{ name: "webtools", source: "/home/coder/.coder/plugins/webtools" },
			{ name: "unnamed", source: "/home/coder/.coder/plugins/unnamed" },
		]);
		expect(model.pluginBytes).toBe(500);
	});

	it("attributes issue names to their plugin without doubling the prefix", () => {
		const model = buildContextPanelModel([
			resource({
				source: "webtools/insecure",
				kind: "mcp_server",
				plugin_name: "webtools",
				status: "unreadable",
				error: "command not found",
			}),
			resource({
				source: "/home/coder/.coder/plugins/webtools/skills/bad",
				kind: "skill",
				plugin_name: "webtools",
				status: "invalid",
				error: "missing frontmatter",
			}),
			resource({
				source: "/home/coder/.coder/plugins/broken",
				kind: "plugin",
				plugin_name: "broken",
				status: "invalid",
				error: "manifest missing name",
			}),
			resource({
				source: "/home/coder/.coder/plugins/nameless",
				kind: "plugin",
				status: "invalid",
				error: "manifest missing name",
			}),
		]);

		expect(model.issues.map((issue) => issue.name)).toEqual([
			"webtools/insecure",
			"webtools/bad",
			"broken",
			"nameless",
		]);
	});

	it("surfaces OK rows with a non-empty error as warnings", () => {
		const model = buildContextPanelModel([
			resource({
				source: "/home/coder/.coder/plugins/webtools",
				kind: "plugin",
				plugin_name: "webtools",
				error: 'unknown field "displayName" ignored',
			}),
			resource({
				source: "/workspace/AGENTS.md",
				kind: "instruction_file",
				status: "oversize",
				error: "file exceeds 64 KiB",
			}),
			resource({ ...plainSkill, error: "" }),
		]);

		expect(model.issues).toEqual([
			{
				name: "webtools",
				kind: "plugin",
				status: "ok",
				statusLabel: "warning",
				error: 'unknown field "displayName" ignored',
				source: "/home/coder/.coder/plugins/webtools",
			},
			{
				name: "AGENTS.md",
				kind: "instruction_file",
				status: "oversize",
				statusLabel: "oversize",
				error: "file exceeds 64 KiB",
				source: "/workspace/AGENTS.md",
			},
		]);
		expect(model.plugins).toEqual([
			{ name: "webtools", source: "/home/coder/.coder/plugins/webtools" },
		]);
	});

	it("leaves rows without plugin_name unchanged", () => {
		const model = buildContextPanelModel([
			resource({
				source: "/workspace/AGENTS.md",
				kind: "instruction_file",
				size_bytes: 64,
			}),
			plainSkill,
			resource({ source: "/workspace/.mcp.json", kind: "mcp_config" }),
			resource({ source: "github", kind: "mcp_server" }),
			resource({
				source: "/workspace/.agents/skills/broken",
				kind: "skill",
				status: "invalid",
				error: "missing frontmatter",
			}),
		]);

		expect(model.fileGroups).toEqual([
			{
				label: { kind: "directory", name: "/workspace" },
				items: [
					{
						path: "/workspace/AGENTS.md",
						group: { kind: "directory", name: "/workspace" },
					},
				],
			},
		]);
		expect(model.skillGroups.map((group) => group.label)).toEqual([
			{ kind: "directory", name: "/workspace/.agents/skills" },
		]);
		expect(model.mcpConfigs).toEqual([{ source: "/workspace/.mcp.json" }]);
		expect(model.mcpServers).toEqual([
			{ name: "github", source: "github", pluginName: undefined, tools: [] },
		]);
		expect(model.issues).toEqual([
			{
				name: "broken",
				kind: "skill",
				status: "invalid",
				statusLabel: "invalid",
				error: "missing frontmatter",
				source: "/workspace/.agents/skills/broken",
			},
		]);
		expect(model.plugins).toEqual([]);
		expect(model.fileBytes).toBe(64);
	});
});
