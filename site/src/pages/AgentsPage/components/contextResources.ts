import type {
	ChatContext,
	ChatContextResource,
	ChatContextResourceKind,
	ChatContextResourceStatus,
	ChatContextTool,
} from "#/api/typesGenerated";
import { getPathBasename, getPathDirname } from "../utils/path";

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
	// Percentage (0-100) at which the context will be compacted.
	readonly compressionThreshold?: number;
	// Pinned workspace-context state: the resources the chat is built from and
	// whether they have drifted from the agent's latest snapshot.
	readonly context?: ChatContext;
}

// Normalized context entries, sourced from the chat's pinned context
// resources and shared by the compact usage popover and the details dialog.
type ContextFileItem = { readonly path: string; readonly dir: string };
type ContextSkillItem = {
	readonly source: string;
	readonly name: string;
	readonly description?: string;
	readonly dir: string;
};
// MCP configs are file-backed (shown by full path), while MCP servers are
// keyed by name and carry their tools.
type ContextMcpConfigItem = { readonly source: string };
type ContextMcpServerItem = {
	readonly name: string;
	readonly source: string;
	readonly tools: readonly ChatContextTool[];
};
// A pinned resource the agent could not use, surfaced with its error so the
// failure is visible instead of silent.
type ContextIssueItem = {
	readonly name: string;
	readonly kind: ChatContextResourceKind;
	readonly status: ChatContextResourceStatus;
	readonly error: string;
	readonly source: string;
};

// Human-readable label per resource kind, used in the issues list.
export const RESOURCE_KIND_LABELS: Record<ChatContextResourceKind, string> = {
	instruction_file: "file",
	skill: "skill",
	mcp_config: "MCP config",
	mcp_server: "MCP server",
};

const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

export const formatTokenCount = (value: number | undefined): string =>
	hasFiniteTokenValue(value) ? value.toLocaleString() : "--";

const formatTokenCountCompact = (value: number | undefined): string => {
	if (!hasFiniteTokenValue(value)) {
		return "--";
	}
	if (value >= 1_000_000) {
		const m = value / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1).replace(/\.0$/, "")}M`;
	}
	if (value >= 1_000) {
		const k = value / 1_000;
		return `${Number.isInteger(k) ? k : k.toFixed(1).replace(/\.0$/, "")}K`;
	}
	return String(value);
};

// Percent (0-100) of the context window consumed, or null when either the
// used tokens or the limit is unknown.
export const getPercentUsed = (
	usage: AgentContextUsage | null,
): number | null => {
	const used = usage?.usedTokens;
	const limit = usage?.contextLimitTokens;
	if (!hasFiniteTokenValue(used) || !hasFiniteTokenValue(limit) || limit <= 0) {
		return null;
	}
	return (used / limit) * 100;
};

// One-line usage summary shown in both the compact popover and the details
// dialog header.
export const formatContextUsageLine = (
	usage: AgentContextUsage | null,
): string => {
	const percent = getPercentUsed(usage);
	if (usage === null || percent === null) {
		return "Context usage unavailable";
	}
	return `${Math.round(percent)}% - ${formatTokenCountCompact(usage.usedTokens)} / ${formatTokenCountCompact(usage.contextLimitTokens)} context used`;
};

// Compaction threshold percent to display, or null when it is unset or the
// usage percent itself is unknown.
export const getCompactionThresholdPercent = (
	usage: AgentContextUsage | null,
): number | null => {
	if (
		usage === null ||
		usage.compressionThreshold === undefined ||
		usage.compressionThreshold <= 0 ||
		getPercentUsed(usage) === null
	) {
		return null;
	}
	return usage.compressionThreshold;
};

// "1 skill" / "3 skills" count labels for the compact popover summary lines
// and the per-server tool counts in the details dialog.
export const countLabel = (count: number, noun: string): string =>
	`${count} ${noun}${count === 1 ? "" : "s"}`;

// Sum the byte size of the OK resources in the given kinds so each section
// can show how much context it costs. Non-OK resources are excluded because
// they are not injected into the prompt.
const sumResourceBytes = (
	resources: readonly ChatContextResource[],
	kinds: readonly ChatContextResourceKind[],
): number =>
	resources.reduce(
		(total, resource) =>
			resource.status === "ok" && kinds.includes(resource.kind)
				? total + (resource.size_bytes ?? 0)
				: total,
		0,
	);

// A set of context resources that share a parent directory. Lists are grouped
// by directory so resources pulled from different roots (for example a
// repo-root AGENTS.md and a nested one) stay distinguishable instead of
// collapsing to identical basenames.
type DirectoryGroup<T> = {
	readonly dir: string;
	readonly items: readonly T[];
};

// Group items by their precomputed dir, preserving first-seen order so the
// list layout stays stable across renders.
const groupByDirectory = <T extends { readonly dir: string }>(
	items: readonly T[],
): readonly DirectoryGroup<T>[] => {
	const byDir = new Map<string, T[]>();
	for (const item of items) {
		const existing = byDir.get(item.dir);
		if (existing) {
			existing.push(item);
		} else {
			byDir.set(item.dir, [item]);
		}
	}
	return [...byDir.entries()].map(([dir, items]) => ({ dir, items }));
};

interface NormalizedContextResources {
	readonly fileItems: readonly ContextFileItem[];
	readonly fileGroups: readonly DirectoryGroup<ContextFileItem>[];
	readonly fileBytes: number;
	readonly skillItems: readonly ContextSkillItem[];
	readonly skillGroups: readonly DirectoryGroup<ContextSkillItem>[];
	readonly skillBytes: number;
	readonly mcpConfigItems: readonly ContextMcpConfigItem[];
	readonly mcpServerItems: readonly ContextMcpServerItem[];
	readonly mcpToolCount: number;
	readonly mcpBytes: number;
	readonly issueItems: readonly ContextIssueItem[];
	// Whether any entry (including issues) exists to show in a full listing.
	readonly hasResources: boolean;
}

// Normalize the chat's pinned context resources into the display entries the
// compact popover (counts) and the details dialog (full tree) are built from.
export const normalizeContextResources = (
	resources: readonly ChatContextResource[] | undefined,
): NormalizedContextResources => {
	const pinned = resources ?? [];
	const fileItems: readonly ContextFileItem[] = pinned
		.filter(
			(resource) =>
				resource.kind === "instruction_file" && resource.status === "ok",
		)
		.map((resource) => ({
			path: resource.source,
			dir: getPathDirname(resource.source),
		}))
		// Drop entries with no usable path or name (here and below) so an empty
		// marker never renders as a blank row.
		.filter((file) => file.path.trim().length > 0);
	const skillItems: readonly ContextSkillItem[] = pinned
		.filter((resource) => resource.kind === "skill" && resource.status === "ok")
		.map((resource) => ({
			source: resource.source,
			name: resource.skill_name || getPathBasename(resource.source),
			description: resource.skill_description,
			dir: getPathDirname(resource.source),
		}))
		.filter((skill) => skill.name.trim().length > 0);
	// MCP configs are shown by their full path so multiple .mcp.json files
	// (e.g. ~/.mcp.json and ~/project/.mcp.json) stay disambiguated; servers
	// are keyed by name and carry their tools.
	const mcpConfigItems: readonly ContextMcpConfigItem[] = pinned
		.filter(
			(resource) => resource.kind === "mcp_config" && resource.status === "ok",
		)
		.map((resource) => ({ source: resource.source }))
		.filter((config) => config.source.trim().length > 0);
	const mcpServerItems: readonly ContextMcpServerItem[] = pinned
		.filter(
			(resource) => resource.kind === "mcp_server" && resource.status === "ok",
		)
		.map((resource) => ({
			name: resource.source,
			source: resource.source,
			tools: resource.tools ?? [],
		}))
		.filter((server) => server.name.trim().length > 0);
	// Pinned resources the agent could not use (invalid skill, unreadable or
	// oversize file) are surfaced as issues with their error so the failure is
	// visible rather than a silent omission.
	const issueItems: readonly ContextIssueItem[] = pinned
		.filter((resource) => resource.status !== "ok")
		.map((resource) => ({
			name:
				resource.skill_name ||
				getPathBasename(resource.source) ||
				resource.source,
			kind: resource.kind,
			status: resource.status,
			error: resource.error ?? "",
			source: resource.source,
		}))
		.filter((issue) => issue.name.trim().length > 0);

	return {
		fileItems,
		fileGroups: groupByDirectory(fileItems),
		fileBytes: sumResourceBytes(pinned, ["instruction_file"]),
		skillItems,
		skillGroups: groupByDirectory(skillItems),
		skillBytes: sumResourceBytes(pinned, ["skill"]),
		mcpConfigItems,
		mcpServerItems,
		mcpToolCount: mcpServerItems.reduce(
			(total, server) => total + server.tools.length,
			0,
		),
		mcpBytes: sumResourceBytes(pinned, ["mcp_config", "mcp_server"]),
		issueItems,
		hasResources:
			fileItems.length > 0 ||
			skillItems.length > 0 ||
			mcpConfigItems.length > 0 ||
			mcpServerItems.length > 0 ||
			issueItems.length > 0,
	};
};
