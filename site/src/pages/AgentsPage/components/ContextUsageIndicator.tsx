import {
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
import { cn } from "#/utils/cn";
import { formatKiB } from "#/utils/fileSize";
import { isMobileViewport } from "#/utils/mobile";
import { getPathBasename, getPathDirname } from "../utils/path";
import { SvgRingProgress } from "./SvgRingProgress";

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

// Normalized popover entries, sourced from the chat's pinned context
// resources.
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
const RESOURCE_KIND_LABELS: Record<ChatContextResourceKind, string> = {
	instruction_file: "file",
	skill: "skill",
	mcp_config: "MCP config",
	mcp_server: "MCP server",
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
		return "text-content-secondary/60";
	}
	if (percentUsed >= 95) {
		return "text-content-destructive";
	}
	if (percentUsed >= 85) {
		return "text-content-warning";
	}
	return "text-content-secondary/60";
};

// A set of context resources that share a parent directory. Lists are grouped
// by directory so resources pulled from different roots (for example a
// repo-root AGENTS.md and a nested one) stay distinguishable instead of
// collapsing to identical basenames.
type DirectoryGroup<T> = {
	readonly dir: string;
	readonly items: readonly T[];
};

// Group items by their precomputed dir, preserving first-seen order so the
// popover layout stays stable across renders.
const groupByDirectory = <T extends { readonly dir: string }>(
	items: readonly T[],
): readonly DirectoryGroup<T>[] => {
	const order: string[] = [];
	const byDir = new Map<string, T[]>();
	for (const item of items) {
		const existing = byDir.get(item.dir);
		if (existing) {
			existing.push(item);
		} else {
			byDir.set(item.dir, [item]);
			order.push(item.dir);
		}
	}
	return order.map((dir) => ({ dir, items: byDir.get(dir) ?? [] }));
};

const RING_SIZE = 18;
const RING_STROKE = 2.5;

// Delay before the popover closes after the mouse leaves, giving
// the user time to move into the popover content.
const HOVER_CLOSE_DELAY_MS = 150;

// Dimmed directory header shown above a group of context resources when a
// section spans more than one directory.
const ContextDirLabel: FC<{ dir: string }> = ({ dir }) => (
	<span
		className="flex items-center gap-1 text-[11px] text-content-secondary"
		title={dir}
	>
		<FolderIcon className="size-3 shrink-0" />
		<span className="truncate">{dir}</span>
	</span>
);

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
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 100;
	const toneClassName = getIndicatorToneClassName(percentUsed);

	const context = usage?.context;
	const isDirty = context?.dirty ?? false;
	const contextError = context?.error ?? "";
	const hasContextError = contextError !== "";
	const pinnedResources = context?.resources;

	// Drive the listed context from the chat's pinned resources.
	const fileItems: readonly ContextFileItem[] = (pinnedResources ?? [])
		.filter(
			(resource) =>
				resource.kind === "instruction_file" && resource.status === "ok",
		)
		.map((resource) => ({
			path: resource.source,
			dir: getPathDirname(resource.source),
		}))
		// Drop entries with no usable path so an empty marker never renders as a
		// nameless "Context files" row.
		.filter((file) => file.path.trim().length > 0);
	const skillItems: readonly ContextSkillItem[] = (pinnedResources ?? [])
		.filter((resource) => resource.kind === "skill" && resource.status === "ok")
		.map((resource) => ({
			source: resource.source,
			name: resource.skill_name || getPathBasename(resource.source),
			description: resource.skill_description,
			dir: getPathDirname(resource.source),
		}))
		// Drop entries with no usable name so an empty skill marker never renders
		// as a blank row.
		.filter((skill) => skill.name.trim().length > 0);
	// MCP configs are shown by their full path so multiple .mcp.json files
	// (e.g. ~/.mcp.json and ~/project/.mcp.json) stay disambiguated; servers
	// are keyed by name and carry their tools.
	const mcpConfigItems: readonly ContextMcpConfigItem[] = (
		pinnedResources ?? []
	)
		.filter(
			(resource) => resource.kind === "mcp_config" && resource.status === "ok",
		)
		.map((resource) => ({ source: resource.source }))
		.filter((config) => config.source.trim().length > 0);
	const mcpServerItems: readonly ContextMcpServerItem[] = (
		pinnedResources ?? []
	)
		.filter(
			(resource) => resource.kind === "mcp_server" && resource.status === "ok",
		)
		.map((resource) => ({
			name: resource.source,
			source: resource.source,
			tools: resource.tools ?? [],
		}))
		// Drop entries with no usable name so an empty MCP marker never renders as
		// a blank row.
		.filter((server) => server.name.trim().length > 0);
	const hasMcp = mcpConfigItems.length > 0 || mcpServerItems.length > 0;
	// Pinned resources the agent could not use (invalid skill, unreadable or
	// oversize file) are surfaced as issues with their error so the failure is
	// visible rather than a silent omission.
	const issueItems: readonly ContextIssueItem[] = (pinnedResources ?? [])
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
	const hasContextList =
		fileItems.length > 0 ||
		skillItems.length > 0 ||
		hasMcp ||
		issueItems.length > 0;
	const fileBytes = sumResourceBytes(pinnedResources ?? [], [
		"instruction_file",
	]);
	const skillBytes = sumResourceBytes(pinnedResources ?? [], ["skill"]);
	const mcpBytes = sumResourceBytes(pinnedResources ?? [], [
		"mcp_config",
		"mcp_server",
	]);

	// Group files and skills by directory so every context root is labeled,
	// keeping resources pulled from different directories distinguishable.
	const fileGroups = groupByDirectory(fileItems);
	const skillGroups = groupByDirectory(skillItems);

	const ariaLabel = hasPercent
		? `Context usage ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.${isDirty ? " Context changed." : ""}`
		: isDirty
			? "Context usage. Context changed."
			: "Context usage";

	const panelContent = (
		<div className="text-xs text-content-primary">
			{hasPercent
				? `${percentLabel} - ${formatTokenCountCompact(usedTokens)} / ${formatTokenCountCompact(contextLimitTokens)} context used`
				: "Context usage unavailable"}
			{hasPercent &&
				usage?.compressionThreshold !== undefined &&
				usage.compressionThreshold > 0 && (
					<div className="mt-1 text-content-secondary">
						{`Compacts at ${usage.compressionThreshold}%`}
					</div>
				)}
			{hasContextList && (
				<div
					className={cn(
						"flex flex-col gap-2 text-content-secondary",
						hasPercent && "mt-2",
					)}
				>
					{fileItems.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								<span>Context files</span>
								<SectionSize bytes={fileBytes} />
							</span>
							{fileGroups.map((group) => (
								<div key={group.dir} className="flex flex-col gap-1">
									{group.dir !== "" && <ContextDirLabel dir={group.dir} />}
									<div
										className={cn(
											"flex flex-col",
											group.dir !== "" ? "ml-3.5 gap-0.5" : "gap-1",
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
					{skillItems.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">
								<span>Skills</span>
								<SectionSize bytes={skillBytes} />
							</span>
							<TooltipProvider delayDuration={300}>
								{skillGroups.map((group) => (
									<div key={group.dir} className="flex flex-col gap-1">
										{group.dir !== "" && <ContextDirLabel dir={group.dir} />}
										<div
											className={cn(
												"flex flex-col",
												group.dir !== "" ? "ml-3.5 gap-0.5" : "gap-1",
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
								{mcpConfigItems.map((config) => (
									<div
										key={config.source}
										className="flex items-center gap-1.5"
										title={config.source}
									>
										<FileIcon className="size-3 shrink-0" />
										<span className="truncate">{config.source}</span>
									</div>
								))}
								{mcpServerItems.map((mcp) => (
									<div key={mcp.source} className="flex flex-col gap-0.5">
										<div
											className="flex items-center gap-1.5"
											title={mcp.source}
										>
											<PlugIcon className="size-3 shrink-0" />
											<span className="truncate">{mcp.name}</span>
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
					{issueItems.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="flex items-center gap-1.5 font-medium text-content-warning">
								<TriangleAlertIcon className="size-3 shrink-0" />
								Issues
							</span>
							{issueItems.map((issue) => (
								<div
									key={issue.source}
									className="flex flex-col"
									title={issue.source}
								>
									<span className="truncate">
										{issue.name}{" "}
										<span className="text-content-secondary">
											({RESOURCE_KIND_LABELS[issue.kind]}: {issue.status})
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
								size="sm"
								disabled={isRefreshingContext}
								onClick={() => onRefreshContext()}
							>
								<Spinner loading={isRefreshingContext} />
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
			className="relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-none bg-transparent p-0 outline-none transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link/40"
		>
			<SvgRingProgress
				size={RING_SIZE}
				strokeWidth={RING_STROKE}
				percent={clampedPercent}
				trackClassName="stroke-content-secondary/25"
				progressClassName="stroke-current"
				className={cn("size-icon-sm", toneClassName)}
			/>
			{(isDirty || hasContextError) && (
				<TriangleAlertIcon
					aria-hidden
					className={cn(
						"absolute -right-0.5 -top-0.5 size-3",
						hasContextError
							? "text-content-destructive"
							: "text-content-warning",
					)}
				/>
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
				<div onMouseEnter={handleMouseEnter} onMouseLeave={scheduleClose}>
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
