import { cn } from "cn";
import {
	BlocksIcon,
	FileIcon,
	FolderIcon,
	PlugIcon,
	TriangleAlertIcon,
	WrenchIcon,
	ZapIcon,
} from "lucide-react";
import { type FC, useRef, useState } from "react";
import type {
	ChatContext,
	ChatContextResource,
	ChatContextResourceKind,
	ChatContextResourceStatus,
	ChatContextTool,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatKiB } from "#/utils/fileSize";
import { isMobileViewport } from "#/utils/mobile";
import { getPathBasename, getPathDirname } from "../utils/path";
import { SvgRingProgress } from "./SvgRingProgress";

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly estimated?: boolean;
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

// Header under which a run of files or skills is listed: either the parent
// directory or the Agent Plugin that ships them. A directory label with an
// empty name means the items have no directory component and render unlabeled.
type ContextGroupLabel = {
	readonly kind: "directory" | "plugin";
	readonly name: string;
};

// Normalized popover entries, sourced from the chat's pinned context
// resources.
type ContextFileItem = {
	readonly path: string;
	readonly group: ContextGroupLabel;
};
type ContextSkillItem = {
	readonly source: string;
	readonly name: string;
	readonly description?: string;
	readonly group: ContextGroupLabel;
};
// An OK Agent Plugin, listed by its manifest name.
type ContextPluginItem = {
	readonly name: string;
	readonly source: string;
};
// MCP configs are file-backed (shown by full path), while MCP servers are
// keyed by name and carry their tools. Servers shipped by a plugin drop the
// "<plugin>/" source prefix from the name and carry the plugin separately.
type ContextMcpConfigItem = { readonly source: string };
type ContextMcpServerItem = {
	readonly name: string;
	readonly source: string;
	readonly pluginName?: string;
	readonly tools: readonly ChatContextTool[];
};
// A pinned resource that failed to load, or an OK resource whose error field
// carries a non-fatal warning. `statusLabel` is the status for failures and
// "warning" for OK rows so the two are distinguishable in the list.
type ContextIssueItem = {
	readonly name: string;
	readonly kind: ChatContextResourceKind;
	readonly status: ChatContextResourceStatus;
	readonly statusLabel: string;
	readonly error: string;
	readonly source: string;
};

// Items that share a group label, in first-seen order.
type ContextGroup<T> = {
	readonly label: ContextGroupLabel;
	readonly items: readonly T[];
};

// Everything the popover lists for a chat's pinned resources, derived once
// from the resource array so the mapping can be tested without rendering.
type ContextPanelModel = {
	readonly fileGroups: readonly ContextGroup<ContextFileItem>[];
	readonly skillGroups: readonly ContextGroup<ContextSkillItem>[];
	readonly plugins: readonly ContextPluginItem[];
	readonly mcpConfigs: readonly ContextMcpConfigItem[];
	readonly mcpServers: readonly ContextMcpServerItem[];
	readonly issues: readonly ContextIssueItem[];
	readonly fileBytes: number;
	readonly skillBytes: number;
	readonly pluginBytes: number;
	readonly mcpBytes: number;
};

// Human-readable label per resource kind, used in the issues list.
const RESOURCE_KIND_LABELS: Record<ChatContextResourceKind, string> = {
	instruction_file: "file",
	skill: "skill",
	mcp_config: "MCP config",
	mcp_server: "MCP server",
	plugin: "plugin",
};

const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

const formatTokenCount = (value: number | undefined): string =>
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

// Sum the byte size of the OK resources in the given kinds so each popover
// section can show how much context it costs. Non-OK resources are excluded
// because they are not injected into the prompt.
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

// Dimmed "(N.N KiB)" size suffix for a section header, omitted when the
// section has no measurable size.
const SectionSize: FC<{ bytes: number }> = ({ bytes }) =>
	bytes > 0 ? (
		<span className="ml-1 font-normal text-content-secondary">
			{`(${formatKiB(bytes)})`}
		</span>
	) : null;

const getIndicatorToneClassName = (percentUsed: number | null): string => {
	if (percentUsed === null) {
		return "text-content-secondary";
	}
	if (percentUsed >= 95) {
		return "text-content-destructive";
	}
	if (percentUsed >= 85) {
		return "text-content-warning";
	}
	return "text-content-secondary";
};

// Group items by their precomputed label, preserving first-seen order so the
// popover layout stays stable across renders. Grouping keeps resources pulled
// from different roots (for example a repo-root AGENTS.md and a nested one)
// distinguishable instead of collapsing to identical basenames.
const groupByLabel = <T extends { readonly group: ContextGroupLabel }>(
	items: readonly T[],
): readonly ContextGroup<T>[] => {
	const groups: { label: ContextGroupLabel; items: T[] }[] = [];
	const byKey = new Map<string, T[]>();
	for (const item of items) {
		const key = `${item.group.kind}:${item.group.name}`;
		const existing = byKey.get(key);
		if (existing) {
			existing.push(item);
		} else {
			const bucket = [item];
			byKey.set(key, bucket);
			groups.push({ label: item.group, items: bucket });
		}
	}
	return groups;
};

const directoryGroup = (source: string): ContextGroupLabel => ({
	kind: "directory",
	name: getPathDirname(source),
});

// Group label for a resource: the owning plugin when one is set, else the
// resource's parent directory.
const resourceGroup = (resource: ChatContextResource): ContextGroupLabel =>
	resource.plugin_name
		? { kind: "plugin", name: resource.plugin_name }
		: directoryGroup(resource.source);

// Display name for a plugin row: the manifest name, else the source basename.
const pluginDisplayName = (resource: ChatContextResource): string =>
	resource.plugin_name || getPathBasename(resource.source);

// Server name for an MCP server row. Plugin servers are sourced as
// "<plugin>/<server>"; the prefix is dropped when it matches the plugin.
const mcpServerDisplayName = (resource: ChatContextResource): string => {
	const prefix = resource.plugin_name ? `${resource.plugin_name}/` : "";
	return prefix !== "" && resource.source.startsWith(prefix)
		? resource.source.slice(prefix.length)
		: resource.source;
};

// Name shown for an issue row. Rows shipped by a plugin are attributed as
// "<plugin>/<name>" (plugin rows use the plugin name alone); the prefix is
// not repeated when the name already carries it.
const issueDisplayName = (resource: ChatContextResource): string => {
	if (resource.kind === "plugin") {
		return pluginDisplayName(resource);
	}
	const base =
		resource.skill_name || getPathBasename(resource.source) || resource.source;
	if (!resource.plugin_name) {
		return base;
	}
	const prefix = `${resource.plugin_name}/`;
	return base.startsWith(prefix) ? base : `${prefix}${base}`;
};

// Map the chat's pinned resources to the lists the popover renders. OK
// resources feed the per-kind sections; non-OK resources, and OK resources
// whose error field carries a warning, feed the issues list. Entries with no
// usable name or path are dropped so empty markers never render as blank rows.
export const buildContextPanelModel = (
	resources: readonly ChatContextResource[],
): ContextPanelModel => {
	const ok = resources.filter((resource) => resource.status === "ok");
	const files: ContextFileItem[] = ok
		.filter((resource) => resource.kind === "instruction_file")
		.map((resource) => ({
			path: resource.source,
			group: directoryGroup(resource.source),
		}))
		.filter((file) => file.path.trim().length > 0);
	const skills: ContextSkillItem[] = ok
		.filter((resource) => resource.kind === "skill")
		.map((resource) => ({
			source: resource.source,
			name: resource.skill_name || getPathBasename(resource.source),
			description: resource.skill_description,
			group: resourceGroup(resource),
		}))
		.filter((skill) => skill.name.trim().length > 0);
	const plugins: ContextPluginItem[] = ok
		.filter((resource) => resource.kind === "plugin")
		.map((resource) => ({
			name: pluginDisplayName(resource),
			source: resource.source,
		}))
		.filter((plugin) => plugin.name.trim().length > 0);
	// MCP configs are shown by their full path so multiple .mcp.json files
	// (e.g. ~/.mcp.json and ~/project/.mcp.json) stay disambiguated.
	const mcpConfigs: ContextMcpConfigItem[] = ok
		.filter((resource) => resource.kind === "mcp_config")
		.map((resource) => ({ source: resource.source }))
		.filter((config) => config.source.trim().length > 0);
	const mcpServers: ContextMcpServerItem[] = ok
		.filter((resource) => resource.kind === "mcp_server")
		.map((resource) => ({
			name: mcpServerDisplayName(resource),
			source: resource.source,
			pluginName: resource.plugin_name || undefined,
			tools: resource.tools ?? [],
		}))
		.filter((server) => server.name.trim().length > 0);
	const issues: ContextIssueItem[] = resources
		.filter(
			(resource) =>
				resource.status !== "ok" || (resource.error ?? "").trim() !== "",
		)
		.map((resource) => ({
			name: issueDisplayName(resource),
			kind: resource.kind,
			status: resource.status,
			statusLabel: resource.status === "ok" ? "warning" : resource.status,
			error: resource.error ?? "",
			source: resource.source,
		}))
		.filter((issue) => issue.name.trim().length > 0);

	return {
		fileGroups: groupByLabel(files),
		skillGroups: groupByLabel(skills),
		plugins,
		mcpConfigs,
		mcpServers,
		issues,
		fileBytes: sumResourceBytes(resources, ["instruction_file"]),
		skillBytes: sumResourceBytes(resources, ["skill"]),
		pluginBytes: sumResourceBytes(resources, ["plugin"]),
		mcpBytes: sumResourceBytes(resources, ["mcp_config", "mcp_server"]),
	};
};

const RING_SIZE = 21.5;
const RING_STROKE = 2.25;

const GLYPH_HEIGHT = 11;
const GLYPH_STROKE = 1.75;
const GLYPH_BAR_LENGTH = 8.1;
const GLYPH_TOP = (RING_SIZE - GLYPH_HEIGHT) / 2;
const GLYPH_CX = RING_SIZE / 2;

const ExclamationGlyph: FC = () => (
	<svg
		width={RING_SIZE}
		height={RING_SIZE}
		viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}
		fill="none"
		aria-hidden="true"
	>
		<line
			x1={GLYPH_CX}
			y1={GLYPH_TOP + GLYPH_STROKE / 2}
			x2={GLYPH_CX}
			y2={GLYPH_TOP + GLYPH_BAR_LENGTH - GLYPH_STROKE / 2}
			stroke="currentColor"
			strokeWidth={GLYPH_STROKE}
			strokeLinecap="round"
		/>
		<circle
			cx={GLYPH_CX}
			cy={GLYPH_TOP + GLYPH_HEIGHT - GLYPH_STROKE / 2}
			r={GLYPH_STROKE / 2}
			fill="currentColor"
		/>
	</svg>
);

// Delay before the popover closes after the mouse leaves, giving
// the user time to move into the popover content.
const HOVER_CLOSE_DELAY_MS = 150;

// Dimmed header shown above a group of context resources: the parent
// directory, or "plugin: <name>" for resources shipped by an Agent Plugin.
const ContextGroupHeader: FC<{ label: ContextGroupLabel }> = ({ label }) => {
	const text = label.kind === "plugin" ? `plugin: ${label.name}` : label.name;
	return (
		<span
			className="flex items-center gap-1 text-[11px] text-content-secondary"
			title={text}
		>
			{label.kind === "plugin" ? (
				<BlocksIcon className="size-3 shrink-0" />
			) : (
				<FolderIcon className="size-3 shrink-0" />
			)}
			<span className="truncate">{text}</span>
		</span>
	);
};

export const ContextUsageIndicator: FC<{
	usage: AgentContextUsage | null;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, onRefreshContext, isRefreshingContext }) => {
	const [open, setOpen] = useState(false);
	const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const cancelClose = () => {
		if (closeTimerRef.current) {
			clearTimeout(closeTimerRef.current);
			closeTimerRef.current = null;
		}
	};

	const scheduleClose = () => {
		cancelClose();
		closeTimerRef.current = setTimeout(() => {
			setOpen(false);
			closeTimerRef.current = null;
		}, HOVER_CLOSE_DELAY_MS);
	};

	const handleMouseEnter = () => {
		cancelClose();
		setOpen(true);
	};

	const usedTokens = hasFiniteTokenValue(usage?.usedTokens)
		? usage.usedTokens
		: undefined;
	const contextLimitTokens = hasFiniteTokenValue(usage?.contextLimitTokens)
		? usage.contextLimitTokens
		: undefined;
	const percentUsed =
		usedTokens !== undefined &&
		contextLimitTokens !== undefined &&
		contextLimitTokens > 0
			? (usedTokens / contextLimitTokens) * 100
			: null;
	const hasPercent = percentUsed !== null;
	// Providers may report usage without token counts. Only a chat with no
	// reported usage at all should promise numbers after the next message.
	const hasReportedUsage = [
		usage?.usedTokens,
		usage?.contextLimitTokens,
		usage?.inputTokens,
		usage?.outputTokens,
		usage?.cacheReadTokens,
		usage?.cacheCreationTokens,
		usage?.reasoningTokens,
	].some(hasFiniteTokenValue);
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 0;

	const context = usage?.context;
	const isDirty = context?.dirty ?? false;
	const contextError = context?.error ?? "";
	const hasContextError = contextError !== "";
	const {
		fileGroups,
		skillGroups,
		plugins,
		mcpConfigs,
		mcpServers,
		issues,
		fileBytes,
		skillBytes,
		pluginBytes,
		mcpBytes,
	} = buildContextPanelModel(context?.resources ?? []);
	const hasMcp = mcpConfigs.length > 0 || mcpServers.length > 0;
	const hasContextList =
		fileGroups.length > 0 ||
		skillGroups.length > 0 ||
		plugins.length > 0 ||
		hasMcp ||
		issues.length > 0;

	const hasResourceIssues = issues.length > 0;
	const hasResourceFailures = issues.some((issue) => issue.status !== "ok");
	const needsAttention = isDirty || hasContextError || hasResourceIssues;
	const toneClassName = hasContextError
		? "text-content-destructive"
		: isDirty || hasResourceIssues
			? "text-content-warning"
			: getIndicatorToneClassName(percentUsed);

	const statusNotes = [
		hasContextError ? "Context error." : "",
		isDirty ? "Context changed." : "",
		hasResourceFailures ? "Some context resources failed to load." : "",
		hasResourceIssues && !hasResourceFailures
			? "Some context resources have warnings."
			: "",
	].filter((note) => note !== "");
	const statusNote = statusNotes.length > 0 ? ` ${statusNotes.join(" ")}` : "";
	let ariaLabel = "Context usage";
	if (hasPercent) {
		const label = usage?.estimated
			? "Estimated context usage"
			: "Context usage";
		ariaLabel = `${label} ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.${statusNote}`;
	} else if (statusNote !== "") {
		ariaLabel = `Context usage.${statusNote}`;
	}

	let usageLabel = "Context usage will appear after sending a message.";
	if (hasPercent) {
		const prefix = usage?.estimated ? "Estimated: " : "";
		usageLabel = `${prefix}${percentLabel} - ${formatTokenCountCompact(usedTokens)} / ${formatTokenCountCompact(contextLimitTokens)} context used`;
	} else if (hasReportedUsage) {
		usageLabel = "Context usage unavailable";
	}

	const panelContent = (
		<div className="text-xs text-content-primary">
			{usageLabel}
			{hasPercent && usage?.estimated && (
				<div className="mt-1 max-w-64 text-content-secondary">
					Based on the compacted summary only, excluding other prompt content
					and tools. Replaced by measured usage after the next response.
				</div>
			)}
			{hasPercent &&
				usage?.compressionThreshold !== undefined &&
				usage.compressionThreshold > 0 && (
					<div className="mt-1 text-content-secondary">
						{`Compacts at ${usage.compressionThreshold}%`}
					</div>
				)}
			{hasContextList && (
				<div className="mt-2 flex flex-col gap-2 text-content-secondary">
					{fileGroups.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								<span>Context files</span>
								<SectionSize bytes={fileBytes} />
							</span>
							{fileGroups.map((group) => (
								<div
									key={`${group.label.kind}:${group.label.name}`}
									className="flex flex-col gap-1"
								>
									{group.label.name !== "" && (
										<ContextGroupHeader label={group.label} />
									)}
									<div
										className={cn(
											"flex flex-col",
											group.label.name !== "" ? "ml-3.5 gap-0.5" : "gap-1",
										)}
									>
										{group.items.map((file) => (
											<div
												key={file.path}
												className="flex items-center gap-1.5"
											>
												<FileIcon className="size-3 shrink-0" />
												<span className="truncate" title={file.path}>
													{getPathBasename(file.path)}
												</span>
											</div>
										))}
									</div>
								</div>
							))}
						</div>
					)}
					{plugins.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								<span>Plugins</span>
								<SectionSize bytes={pluginBytes} />
							</span>
							{plugins.map((plugin) => (
								<div
									key={plugin.source}
									className="flex items-center gap-1.5"
									title={plugin.source}
								>
									<BlocksIcon className="size-3 shrink-0" />
									<span className="truncate">{plugin.name}</span>
								</div>
							))}
						</div>
					)}
					{skillGroups.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								<span>Skills</span>
								<SectionSize bytes={skillBytes} />
							</span>
							<TooltipProvider delayDuration={300}>
								{skillGroups.map((group) => (
									<div
										key={`${group.label.kind}:${group.label.name}`}
										className="flex flex-col gap-1"
									>
										{group.label.name !== "" && (
											<ContextGroupHeader label={group.label} />
										)}
										<div
											className={cn(
												"flex flex-col",
												group.label.name !== "" ? "ml-3.5 gap-0.5" : "gap-1",
											)}
										>
											{group.items.map((skill) => {
												const row = (
													<div className="flex items-center gap-1.5 rounded px-0.5 py-px transition-colors hover:bg-surface-tertiary">
														<ZapIcon className="size-3 shrink-0" />
														<span className="truncate">{skill.name}</span>
													</div>
												);
												if (!skill.description) {
													return <div key={skill.source}>{row}</div>;
												}
												return (
													<Tooltip key={skill.source}>
														<TooltipTrigger asChild>
															<div className="cursor-default">{row}</div>
														</TooltipTrigger>
														<TooltipContent
															side="right"
															sideOffset={4}
															className="max-w-48 text-xs"
														>
															{skill.description}
														</TooltipContent>
													</Tooltip>
												);
											})}
										</div>
									</div>
								))}
							</TooltipProvider>
						</div>
					)}
					{hasMcp && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								<span>MCP</span>
								<SectionSize bytes={mcpBytes} />
							</span>
							<TooltipProvider delayDuration={300}>
								{mcpConfigs.map((config) => (
									<div
										key={config.source}
										className="flex items-center gap-1.5"
										title={config.source}
									>
										<FileIcon className="size-3 shrink-0" />
										<span className="truncate">{config.source}</span>
									</div>
								))}
								{mcpServers.map((mcp) => (
									<div key={mcp.source} className="flex flex-col gap-0.5">
										<div
											className="flex items-center gap-1.5"
											title={mcp.source}
										>
											<PlugIcon className="size-3 shrink-0" />
											<span className="truncate">{mcp.name}</span>
											{mcp.pluginName && (
												<span className="shrink-0 text-[11px] text-content-secondary">
													{`plugin: ${mcp.pluginName}`}
												</span>
											)}
										</div>
										{mcp.tools.length > 0 && (
											<div className="ml-4 flex flex-col gap-0.5">
												{mcp.tools.map((tool) => {
													const row = (
														<div className="flex items-center gap-1.5 rounded px-0.5 py-px text-content-secondary transition-colors hover:bg-surface-tertiary">
															<WrenchIcon className="size-3 shrink-0" />
															<span className="truncate">{tool.name}</span>
														</div>
													);
													if (!tool.description) {
														return <div key={tool.name}>{row}</div>;
													}
													return (
														<Tooltip key={tool.name}>
															<TooltipTrigger asChild>
																<div className="cursor-default">{row}</div>
															</TooltipTrigger>
															<TooltipContent
																side="right"
																sideOffset={4}
																className="max-w-48 text-xs"
															>
																{tool.description}
															</TooltipContent>
														</Tooltip>
													);
												})}
											</div>
										)}
									</div>
								))}
							</TooltipProvider>
						</div>
					)}
					{issues.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="flex items-center gap-1.5 font-medium text-content-warning">
								<TriangleAlertIcon className="size-3 shrink-0" />
								Issues
							</span>
							{issues.map((issue) => (
								<div
									key={`${issue.kind}:${issue.source}`}
									className="flex flex-col"
									title={issue.source}
								>
									<span className="truncate">
										{issue.name}{" "}
										<span className="text-content-secondary">
											({RESOURCE_KIND_LABELS[issue.kind]}: {issue.statusLabel})
										</span>
									</span>
									{issue.error && (
										<span className="text-content-secondary">
											{issue.error}
										</span>
									)}
								</div>
							))}
						</div>
					)}
				</div>
			)}
			{(isDirty || hasContextError) && (
				<div className="mt-2 flex flex-col gap-1.5 border-0 border-t border-solid border-border-default pt-2">
					{hasContextError ? (
						<span className="flex items-center gap-1.5 font-medium text-content-destructive">
							<TriangleAlertIcon className="size-3 shrink-0" />
							Context error
						</span>
					) : (
						<span className="flex items-center gap-1.5 font-medium text-content-warning">
							<TriangleAlertIcon className="size-3 shrink-0" />
							Context changed
						</span>
					)}
					{hasContextError ? (
						<span className="text-content-secondary">{contextError}</span>
					) : (
						<span className="text-content-secondary">
							The workspace context changed since this chat was pinned.
						</span>
					)}
					{onRefreshContext && (
						<div className="flex flex-wrap gap-2">
							<Button
								size="xs"
								disabled={isRefreshingContext}
								onClick={() => onRefreshContext()}
							>
								<Spinner size="sm" loading={isRefreshingContext} />
								Refresh context
							</Button>
						</div>
					)}
				</div>
			)}
		</div>
	);

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaLabel}
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-hidden transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<SvgRingProgress
				size={RING_SIZE}
				strokeWidth={RING_STROKE}
				percent={clampedPercent}
				trackClassName="stroke-border"
				progressClassName="stroke-current"
				className={toneClassName}
			/>
			{needsAttention && (
				<span
					aria-hidden="true"
					className={cn(
						"absolute inset-0 flex items-center justify-center",
						toneClassName,
					)}
				>
					<ExclamationGlyph />
				</span>
			)}
		</button>
	);

	// On mobile, a tap toggles the popover. On desktop, hover opens
	// it like a dropdown menu and skill descriptions appear as
	// nested tooltips to the right.
	if (isMobileViewport()) {
		return (
			<Popover>
				<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
				<PopoverContent
					side="top"
					className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto max-w-72 px-3 py-2"
				>
					{panelContent}
				</PopoverContent>
			</Popover>
		);
	}

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<div
					className="flex"
					onMouseEnter={handleMouseEnter}
					onMouseLeave={scheduleClose}
				>
					{triggerButton}
				</div>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				className="w-auto max-w-72 px-3 py-2"
				onMouseEnter={cancelClose}
				onMouseLeave={scheduleClose}
				// Prevent the popover from stealing focus, which would
				// interfere with the chat input.
				onOpenAutoFocus={(e) => e.preventDefault()}
			>
				{panelContent}
			</PopoverContent>
		</Popover>
	);
};
