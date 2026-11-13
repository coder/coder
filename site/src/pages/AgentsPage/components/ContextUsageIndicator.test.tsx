import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatContextDirty,
	MockChatContextResourceFailures,
} from "#/testHelpers/chatEntities";
import {
	buildContextPanelModel,
	ContextUsageIndicator,
	getIndicatorState,
} from "./ContextUsageIndicator";

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

const noLabel = { kind: "directory", name: "" } as const;

describe("buildContextPanelModel", () => {
	it("groups plugin skills under the plugin and plain skills under their directory", () => {
		const laterPluginSkill = resource({
			source: "/home/coder/.coder/plugins/webtools/skills/crawl",
			kind: "skill",
			skill_name: "crawl",
			plugin_name: "webtools",
		});
		const model = buildContextPanelModel([
			plainSkill,
			pluginSkill,
			otherPluginSkill,
			laterPluginSkill,
		]);

		expect(model.skillGroups).toEqual([
			{
				key: "directory:/workspace/.agents/skills",
				label: { kind: "directory", name: "/workspace/.agents/skills" },
				items: [
					{
						source: plainSkill.source,
						name: "deploy",
						description: "Deploy the app.",
						label: { kind: "directory", name: "/workspace/.agents/skills" },
					},
				],
			},
			{
				key: "plugin:webtools",
				label: { kind: "plugin", name: "webtools" },
				items: [
					{
						source: pluginSkill.source,
						name: "fetch",
						description: undefined,
						label: { kind: "plugin", name: "webtools" },
					},
					{
						source: laterPluginSkill.source,
						name: "crawl",
						description: undefined,
						label: { kind: "plugin", name: "webtools" },
					},
				],
			},
			{
				key: "plugin:dbtools",
				label: { kind: "plugin", name: "dbtools" },
				items: [
					{
						source: otherPluginSkill.source,
						name: "migrate",
						description: undefined,
						label: { kind: "plugin", name: "dbtools" },
					},
				],
			},
		]);
	});

	it("groups plugin MCP servers by plugin under their server name, after standalone servers", () => {
		const tools: TypesGen.ChatContextTool[] = [{ name: "fetch_url" }];
		const model = buildContextPanelModel([
			resource({
				source: "webtools/tools",
				kind: "mcp_server",
				plugin_name: "webtools",
				tools,
			}),
			resource({ source: "github", kind: "mcp_server", tools: [] }),
			resource({
				source: "webtools/insecure",
				kind: "mcp_server",
				plugin_name: "webtools",
				status: "unreadable",
				error: "command not found",
			}),
		]);

		expect(model.mcpServerGroups).toEqual([
			{
				key: "directory:",
				label: noLabel,
				items: [
					{
						name: "github",
						source: "github",
						label: noLabel,
						connected: true,
						tools: [],
					},
				],
			},
			{
				key: "plugin:webtools",
				label: { kind: "plugin", name: "webtools" },
				items: [
					{
						name: "tools",
						source: "webtools/tools",
						label: { kind: "plugin", name: "webtools" },
						connected: true,
						tools,
					},
					{
						name: "insecure",
						source: "webtools/insecure",
						label: { kind: "plugin", name: "webtools" },
						connected: false,
						tools: [],
					},
				],
			},
		]);
	});

	it("keeps warnings on running MCP servers out of the not-running list", () => {
		const model = buildContextPanelModel([
			resource({
				source: "webtools/tools",
				kind: "mcp_server",
				plugin_name: "webtools",
				error: "tool list truncated",
			}),
			resource({
				source: "github",
				kind: "mcp_server",
				status: "unreadable",
				error: "connection refused",
			}),
			resource({
				source: "/workspace/.mcp.json",
				kind: "mcp_config",
				status: "invalid",
				error: "bad JSON",
			}),
		]);

		expect(model.mcpIssues.map((issue) => issue.name)).toEqual([
			"webtools/tools",
			".mcp.json",
		]);
		expect(model.mcpServerFailures.map((issue) => issue.name)).toEqual([
			"github",
		]);
	});

	it("lists OK plugins by plugin name", () => {
		const model = buildContextPanelModel([
			resource({
				source: "/home/coder/.coder/plugins/webtools",
				kind: "plugin",
				plugin_name: "webtools",
				size_bytes: 300,
			}),
			resource({
				source: "/home/coder/.coder/plugins/broken",
				kind: "plugin",
				status: "invalid",
				error: "manifest missing name",
			}),
			resource({
				source: "/home/coder/.coder/plugins/unnamed",
				kind: "plugin",
			}),
		]);

		expect(model.plugins).toEqual([
			{ name: "webtools", source: "/home/coder/.coder/plugins/webtools" },
		]);
		expect(model.pluginIssues.map((issue) => issue.name)).toEqual(["broken"]);
	});

	it("prefixes plugin-shipped issue names with the plugin and names plugin rows alone", () => {
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
				status: "invalid",
				error: "manifest missing name",
			}),
		]);

		expect(model.issues.map((issue) => issue.name)).toEqual([
			"webtools/insecure",
			"webtools/bad",
			"broken",
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
		expect(model.hasResourceIssues).toBe(true);
		expect(model.hasResourceFailures).toBe(true);
	});

	it("counts a warning on an OK row as an issue but not a failure", () => {
		const model = buildContextPanelModel([
			resource({
				source: "/home/coder/.coder/plugins/webtools",
				kind: "plugin",
				plugin_name: "webtools",
				error: 'unknown field "displayName" ignored',
			}),
			plainSkill,
		]);

		expect(model.hasResourceIssues).toBe(true);
		expect(model.hasResourceFailures).toBe(false);
		expect(buildContextPanelModel([plainSkill])).toMatchObject({
			hasResourceIssues: false,
			hasResourceFailures: false,
		});
	});

	it("groups resources without a plugin by directory and names their issues by basename", () => {
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
				source: "claude",
				kind: "acp_harness",
				source_path: "/workspace/.coder/acp",
			}),
			resource({
				source: "/workspace/.agents/skills/broken",
				kind: "skill",
				status: "invalid",
				error: "missing frontmatter",
			}),
		]);

		expect(model.fileGroups).toEqual([
			{
				key: "directory:/workspace",
				label: { kind: "directory", name: "/workspace" },
				items: [
					{
						path: "/workspace/AGENTS.md",
						label: { kind: "directory", name: "/workspace" },
					},
				],
			},
		]);
		expect(model.skillGroups.map((group) => group.label)).toEqual([
			{ kind: "directory", name: "/workspace/.agents/skills" },
		]);
		expect(model.mcpConfigs).toEqual([{ source: "/workspace/.mcp.json" }]);
		expect(model.mcpServerGroups.map((group) => group.label)).toEqual([
			noLabel,
		]);
		expect(model.harnessGroups).toEqual([
			{
				key: "directory:/workspace/.coder/acp",
				label: { kind: "directory", name: "/workspace/.coder/acp" },
				items: [
					{
						slug: "claude",
						label: { kind: "directory", name: "/workspace/.coder/acp" },
					},
				],
			},
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
		expect(model.skillIssues).toEqual(model.issues);
		expect(model.plugins).toEqual([]);
	});
});

describe("getIndicatorState", () => {
	const calm = {
		percentUsed: 10,
		isDirty: false,
		hasContextError: false,
		hasResourceIssues: false,
	};

	it("leaves the indicator unmarked without issues", () => {
		expect(getIndicatorState(calm)).toEqual({
			tone: "default",
			needsAttention: false,
		});
	});

	it("marks the indicator for any resource issue, warnings included", () => {
		expect(getIndicatorState({ ...calm, hasResourceIssues: true })).toEqual({
			tone: "warning",
			needsAttention: true,
		});
	});

	it("marks drift as a warning and a context error as destructive", () => {
		expect(getIndicatorState({ ...calm, isDirty: true })).toEqual({
			tone: "warning",
			needsAttention: true,
		});
		expect(
			getIndicatorState({
				...calm,
				hasContextError: true,
				hasResourceIssues: true,
			}),
		).toEqual({ tone: "destructive", needsAttention: true });
	});

	it("tones by usage relative to the compaction point without marking the indicator", () => {
		expect(getIndicatorState({ ...calm, percentUsed: 89 })).toEqual({
			tone: "default",
			needsAttention: false,
		});
		expect(getIndicatorState({ ...calm, percentUsed: 90 })).toEqual({
			tone: "warning",
			needsAttention: false,
		});
		expect(
			getIndicatorState({ ...calm, percentUsed: 63, compactionPercent: 70 }),
		).toEqual({ tone: "warning", needsAttention: false });
		expect(getIndicatorState({ ...calm, percentUsed: 95 })).toEqual({
			tone: "destructive",
			needsAttention: false,
		});
		expect(getIndicatorState({ ...calm, percentUsed: null })).toEqual({
			tone: "default",
			needsAttention: false,
		});
	});
});

describe("ContextUsageIndicator", () => {
	it("opens server management from an MCP server failure", async () => {
		const user = userEvent.setup();
		const onManageServers = vi.fn();
		render(
			<ContextUsageIndicator
				usage={{ context: MockChatContextResourceFailures }}
				onManageServers={onManageServers}
			/>,
		);
		await user.click(screen.getByRole("button", { name: /Context usage/ }));
		await user.click(screen.getByRole("button", { name: /MCP Servers/ }));
		await user.click(screen.getByRole("button", { name: "Manage servers" }));
		expect(onManageServers).toHaveBeenCalledTimes(1);
	});

	it("refreshes context after opening the popover with a click", async () => {
		const user = userEvent.setup();
		const onRefreshContext = vi.fn();
		render(
			<ContextUsageIndicator
				usage={{
					usedTokens: 12_000,
					contextLimitTokens: 200_000,
					context: MockChatContextDirty,
				}}
				onRefreshContext={onRefreshContext}
			/>,
		);
		await user.click(screen.getByRole("button", { name: /Context usage/ }));
		await user.click(screen.getByRole("button", { name: "Refresh context" }));
		expect(onRefreshContext).toHaveBeenCalledTimes(1);
	});

	it("notes warnings in the accessible name when no resource failed", () => {
		render(
			<ContextUsageIndicator
				usage={{
					usedTokens: 12_000,
					contextLimitTokens: 200_000,
					context: {
						dirty: false,
						resources: [
							resource({
								source: "/home/coder/.coder/plugins/webtools",
								kind: "plugin",
								plugin_name: "webtools",
								error: 'unknown field "displayName" ignored',
							}),
						],
					},
				}}
			/>,
		);

		const button = screen.getByRole("button");
		expect(button).toHaveAccessibleName(
			/Some context resources have warnings\./,
		);
		expect(button).not.toHaveAccessibleName(/failed to load/);
	});

	it("announces failed resources in the accessible name", () => {
		render(
			<ContextUsageIndicator
				usage={{
					usedTokens: 12_000,
					contextLimitTokens: 200_000,
					context: {
						dirty: false,
						resources: [
							resource({
								source: "/home/coder/.coder/plugins/broken",
								kind: "plugin",
								status: "invalid",
								error: "manifest missing name",
							}),
						],
					},
				}}
			/>,
		);

		const button = screen.getByRole("button");
		expect(button).toHaveAccessibleName(/failed to load/);
	});

	it("shows plugin warnings in the Plugins section", async () => {
		const user = userEvent.setup();
		render(
			<ContextUsageIndicator
				usage={{
					context: {
						dirty: false,
						resources: [
							resource({
								source: "/home/coder/.coder/plugins/webtools",
								kind: "plugin",
								plugin_name: "webtools",
								error: 'unknown field "displayName" ignored',
							}),
						],
					},
				}}
			/>,
		);
		await user.click(screen.getByRole("button", { name: /Context usage/ }));
		await user.click(screen.getByRole("button", { name: /Plugins/ }));
		expect(screen.getByText("(plugin manifest: warning)")).toBeVisible();
		expect(
			screen.getByText('unknown field "displayName" ignored'),
		).toBeVisible();
	});
});
