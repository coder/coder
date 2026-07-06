import type {
	ChatContext,
	ChatContextResource,
	ChatContextResourceKind,
	ChatContextResourceStatus,
	ChatContextTool,
} from "#/api/typesGenerated";
import { formatTokenCount as formatTokenCountBase } from "#/utils/analytics";
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

export const RESOURCE_KIND_LABELS: Record<ChatContextResourceKind, string> = {
	instruction_file: "file",
	skill: "skill",
	mcp_config: "MCP config",
	mcp_server: "MCP server",
};

export const RESOURCE_STATUS_LABELS: Record<ChatContextResourceStatus, string> =
	{
		ok: "ok",
		excluded: "excluded",
		invalid: "invalid",
		oversize: "too large",
		unreadable: "unreadable",
	};

// Explanations shown when the backend omits a per-resource error message, so
// an issue row never renders without a diagnosis.
const STATUS_EXPLANATIONS: Partial<Record<ChatContextResourceStatus, string>> =
	{
		oversize: "File exceeds the context size limit.",
		unreadable: "File could not be read.",
		excluded: "Resource was excluded by configuration.",
		invalid: "Resource configuration is invalid.",
	};

const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

// Adds undefined handling on top of the shared token formatter.
export const formatTokenCount = (value: number | undefined): string =>
	hasFiniteTokenValue(value) ? formatTokenCountBase(value) : "--";

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

// Null when either the used tokens or the limit is unknown.
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

export const formatContextUsageLine = (
	usage: AgentContextUsage | null,
): string => {
	const percent = getPercentUsed(usage);
	if (usage === null || percent === null) {
		return "Context usage unavailable";
	}
	return `${Math.round(percent)}% - ${formatTokenCountCompact(usage.usedTokens)} / ${formatTokenCountCompact(usage.contextLimitTokens)} context used`;
};

// Null when the threshold is unset or the usage percent itself is unknown.
export const getCompressionThresholdPercent = (
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

export const countLabel = (count: number, noun: string): string =>
	`${count} ${noun}${count === 1 ? "" : "s"}`;

// Non-OK resources are excluded because they are not injected into the
// prompt.
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

// Grouping by directory keeps resources pulled from different roots (for
// example a repo-root AGENTS.md and a nested one) distinguishable instead of
// collapsing to identical basenames.
type DirectoryGroup<T> = {
	readonly dir: string;
	readonly items: readonly T[];
};

// Manual grouping because Map.groupBy is unavailable in the browserslist
// targets (Safari 16, Chrome 110). Map iteration preserves first-seen order.
const groupByDirectory = <T extends { readonly dir: string }>(
	items: readonly T[],
): readonly DirectoryGroup<T>[] => {
	const byDir = new Map<string, T[]>();
	for (const item of items) {
		const group = byDir.get(item.dir);
		if (group) {
			group.push(item);
		} else {
			byDir.set(item.dir, [item]);
		}
	}
	return [...byDir].map(([dir, items]) => ({ dir, items }));
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
	// Full paths disambiguate multiple .mcp.json files.
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
	const issueItems: readonly ContextIssueItem[] = pinned
		.filter((resource) => resource.status !== "ok")
		.map((resource) => ({
			name:
				resource.skill_name ||
				getPathBasename(resource.source) ||
				resource.source,
			kind: resource.kind,
			status: resource.status,
			error: resource.error || STATUS_EXPLANATIONS[resource.status] || "",
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
