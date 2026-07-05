import { describe, expect, it } from "vitest";
import type { ChatContextResource } from "#/api/typesGenerated";
import {
	countLabel,
	formatContextUsageLine,
	getCompactionThresholdPercent,
	getPercentUsed,
	normalizeContextResources,
} from "./contextResources";

const resources: ChatContextResource[] = [
	{
		source: "/home/coder/AGENTS.md",
		kind: "instruction_file",
		size_bytes: 200,
		status: "ok",
	},
	{
		source: "/home/coder/site/AGENTS.md",
		kind: "instruction_file",
		size_bytes: 300,
		status: "ok",
	},
	{
		source: "/home/coder/.coder/skills/deploy",
		kind: "skill",
		size_bytes: 100,
		status: "ok",
		skill_name: "deploy",
		skill_description: "Deploy the app.",
	},
	{
		source: "/home/coder/.agents/skills/review",
		kind: "skill",
		size_bytes: 50,
		status: "ok",
	},
	{
		source: "/home/coder/.mcp.json",
		kind: "mcp_config",
		size_bytes: 150,
		status: "ok",
	},
	{
		source: "github",
		kind: "mcp_server",
		size_bytes: 400,
		status: "ok",
		tools: [
			{ name: "search_issues", description: "Search issues." },
			{ name: "create_issue" },
		],
	},
	{
		source: "/home/coder/test/.agents/skills/moo",
		kind: "skill",
		size_bytes: 75,
		status: "invalid",
		error: "name mismatch",
	},
	{
		source: "/home/coder/huge.md",
		kind: "instruction_file",
		size_bytes: 9000,
		status: "oversize",
	},
];

describe("normalizeContextResources", () => {
	it("returns empty entries for undefined resources", () => {
		const normalized = normalizeContextResources(undefined);
		expect(normalized.fileItems).toEqual([]);
		expect(normalized.fileGroups).toEqual([]);
		expect(normalized.fileBytes).toBe(0);
		expect(normalized.skillItems).toEqual([]);
		expect(normalized.mcpConfigItems).toEqual([]);
		expect(normalized.mcpServerItems).toEqual([]);
		expect(normalized.mcpToolCount).toBe(0);
		expect(normalized.issueItems).toEqual([]);
	});

	it("normalizes files, skills, and MCP entries from ok resources only", () => {
		const normalized = normalizeContextResources(resources);

		expect(normalized.fileItems).toEqual([
			{ path: "/home/coder/AGENTS.md", dir: "/home/coder" },
			{ path: "/home/coder/site/AGENTS.md", dir: "/home/coder/site" },
		]);
		// Skill names fall back to the source basename when unnamed.
		expect(normalized.skillItems.map((skill) => skill.name)).toEqual([
			"deploy",
			"review",
		]);
		expect(normalized.mcpConfigItems).toEqual([
			{ source: "/home/coder/.mcp.json" },
		]);
		expect(normalized.mcpServerItems).toEqual([
			{
				name: "github",
				source: "github",
				tools: [
					{ name: "search_issues", description: "Search issues." },
					{ name: "create_issue" },
				],
			},
		]);
		expect(normalized.mcpToolCount).toBe(2);
	});

	it("groups files and skills by directory in first-seen order", () => {
		const normalized = normalizeContextResources(resources);
		expect(normalized.fileGroups).toEqual([
			{
				dir: "/home/coder",
				items: [{ path: "/home/coder/AGENTS.md", dir: "/home/coder" }],
			},
			{
				dir: "/home/coder/site",
				items: [
					{ path: "/home/coder/site/AGENTS.md", dir: "/home/coder/site" },
				],
			},
		]);
		expect(normalized.skillGroups.map((group) => group.dir)).toEqual([
			"/home/coder/.coder/skills",
			"/home/coder/.agents/skills",
		]);
	});

	it("sums bytes per section from ok resources only", () => {
		const normalized = normalizeContextResources(resources);
		// The too_large instruction file is not injected, so it does not count.
		expect(normalized.fileBytes).toBe(500);
		// The invalid skill does not count either.
		expect(normalized.skillBytes).toBe(150);
		// MCP bytes cover configs and servers.
		expect(normalized.mcpBytes).toBe(550);
	});

	it("surfaces non-ok resources as issues", () => {
		const normalized = normalizeContextResources(resources);
		expect(normalized.issueItems).toEqual([
			{
				name: "moo",
				kind: "skill",
				status: "invalid",
				error: "name mismatch",
				source: "/home/coder/test/.agents/skills/moo",
			},
			{
				name: "huge.md",
				kind: "instruction_file",
				status: "oversize",
				error: "",
				source: "/home/coder/huge.md",
			},
		]);
	});

	it("drops entries without a usable name or path", () => {
		const normalized = normalizeContextResources([
			{ source: " ", kind: "instruction_file", size_bytes: 0, status: "ok" },
			{ source: "", kind: "skill", size_bytes: 0, status: "ok" },
			{ source: "", kind: "mcp_config", size_bytes: 0, status: "ok" },
			{ source: " ", kind: "mcp_server", size_bytes: 0, status: "ok" },
		]);
		expect(normalized.fileItems).toEqual([]);
		expect(normalized.skillItems).toEqual([]);
		expect(normalized.mcpConfigItems).toEqual([]);
		expect(normalized.mcpServerItems).toEqual([]);
	});
});

describe("usage formatting", () => {
	it("formats the usage line from used and limit tokens", () => {
		expect(
			formatContextUsageLine({
				usedTokens: 12_000,
				contextLimitTokens: 200_000,
			}),
		).toBe("6% - 12K / 200K context used");
	});

	it("falls back when usage is unknown", () => {
		expect(formatContextUsageLine(null)).toBe("Context usage unavailable");
		expect(formatContextUsageLine({ usedTokens: 12_000 })).toBe(
			"Context usage unavailable",
		);
	});

	it("computes the percent used", () => {
		expect(
			getPercentUsed({ usedTokens: 50_000, contextLimitTokens: 200_000 }),
		).toBe(25);
		expect(getPercentUsed({ usedTokens: 50_000 })).toBeNull();
		expect(
			getPercentUsed({ usedTokens: 50_000, contextLimitTokens: 0 }),
		).toBeNull();
	});

	it("only reports a compaction threshold alongside a known usage percent", () => {
		expect(
			getCompactionThresholdPercent({
				usedTokens: 12_000,
				contextLimitTokens: 200_000,
				compressionThreshold: 80,
			}),
		).toBe(80);
		expect(
			getCompactionThresholdPercent({
				usedTokens: 12_000,
				compressionThreshold: 80,
			}),
		).toBeNull();
		expect(
			getCompactionThresholdPercent({
				usedTokens: 12_000,
				contextLimitTokens: 200_000,
				compressionThreshold: 0,
			}),
		).toBeNull();
	});

	it("pluralizes count labels", () => {
		expect(countLabel(1, "skill")).toBe("1 skill");
		expect(countLabel(3, "skill")).toBe("3 skills");
	});
});
