import {
	FileIcon,
	PlugIcon,
	TriangleAlertIcon,
	WrenchIcon,
	ZapIcon,
} from "lucide-react";
import { type FC, useRef, useState } from "react";
import type {
	ChatContext,
	ChatContextResourceKind,
	ChatContextResourceStatus,
	ChatContextTool,
	ChatMessagePart,
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
import { isMobileViewport } from "#/utils/mobile";
import { getPathBasename } from "../utils/path";
import { ContextChangesDialog } from "./ContextChangesDialog";
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
	// Last injected context parts (AGENTS.md files and skills). Used as a
	// fallback to list the context when the chat's pinned resources have not
	// loaded yet.
	readonly lastInjectedContext?: readonly ChatMessagePart[];
	// Pinned workspace-context state: the resources the chat is built from and
	// whether they have drifted from the agent's latest snapshot.
	readonly context?: ChatContext;
}

// Normalized popover entries, sourced from either the chat's pinned context
// resources or, as a fallback, the last injected context parts.
type ContextFileItem = { readonly path: string; readonly truncated?: boolean };
type ContextSkillItem = {
	readonly name: string;
	readonly description?: string;
};
type ContextMcpItem = {
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

const RING_SIZE = 18;
const RING_STROKE = 2.5;

// Delay before the popover closes after the mouse leaves, giving
// the user time to move into the popover content.
const HOVER_CLOSE_DELAY_MS = 150;

export const ContextUsageIndicator: FC<{
	usage: AgentContextUsage | null;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, onRefreshContext, isRefreshingContext }) => {
	const [open, setOpen] = useState(false);
	const [changesOpen, setChangesOpen] = useState(false);
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
	const changes = context?.changes ?? [];
	const pinnedResources = context?.resources;

	// Drive the listed context from the chat's pinned resources, falling back
	// to the last injected context parts while the pin has not loaded.
	const usePinned = (pinnedResources?.length ?? 0) > 0;
	const fileItems: readonly ContextFileItem[] = (
		usePinned
			? (pinnedResources ?? [])
					.filter(
						(resource) =>
							resource.kind === "instruction_file" && resource.status === "ok",
					)
					.map((resource) => ({ path: resource.source }))
			: (usage?.lastInjectedContext ?? [])
					.filter((part) => part.type === "context-file")
					.map((part) => ({
						path: part.context_file_path,
						truncated: part.context_file_truncated,
					}))
	)
		// Drop entries with no usable path. The injected-context fallback can
		// carry an empty context-file marker, which would otherwise render as a
		// nameless "Context files" row.
		.filter((file) => file.path.trim().length > 0);
	const skillItems: readonly ContextSkillItem[] = (
		usePinned
			? (pinnedResources ?? [])
					.filter(
						(resource) => resource.kind === "skill" && resource.status === "ok",
					)
					.map((resource) => ({
						name: resource.skill_name || getPathBasename(resource.source),
						description: resource.skill_description,
					}))
			: (usage?.lastInjectedContext ?? [])
					.filter((part) => part.type === "skill")
					.map((part) => ({
						name: part.skill_name,
						description: part.skill_description,
					}))
	)
		// Drop entries with no usable name so an empty skill marker never renders
		// as a blank row.
		.filter((skill) => skill.name.trim().length > 0);
	// MCP configs/servers are only ever surfaced from the chat's pinned
	// resources; there is no injected-context fallback for them. An MCP server's
	// source is its server name, while an MCP config's source is its file path.
	const mcpItems: readonly ContextMcpItem[] = (
		usePinned
			? (pinnedResources ?? [])
					.filter(
						(resource) =>
							(resource.kind === "mcp_config" ||
								resource.kind === "mcp_server") &&
							resource.status === "ok",
					)
					.map((resource) => ({
						name:
							resource.kind === "mcp_server"
								? resource.source
								: getPathBasename(resource.source),
						source: resource.source,
						tools: resource.tools ?? [],
					}))
			: []
	)
		// Drop entries with no usable name so an empty MCP marker never renders as
		// a blank row.
		.filter((mcp) => mcp.name.trim().length > 0);
	// Pinned resources the agent could not use (invalid skill, unreadable or
	// oversize file) are surfaced as issues with their error so the failure is
	// visible rather than a silent omission. Pinned-only; the injected-context
	// fallback has no status.
	const issueItems: readonly ContextIssueItem[] = (
		usePinned ? (pinnedResources ?? []) : []
	)
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
		mcpItems.length > 0 ||
		issueItems.length > 0;

	const ariaLabel = hasPercent
		? `Context usage ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.${isDirty ? " Context changed." : ""}`
		: isDirty
			? "Context usage. Context changed."
			: "Context usage";

	const openChanges = () => {
		setChangesOpen(true);
		setOpen(false);
	};

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
								Context files
							</span>
							{fileItems.map((file) => (
								<div key={file.path} className="flex items-center gap-1.5">
									<FileIcon className="size-3 shrink-0" />
									<span className="truncate" title={file.path}>
										{getPathBasename(file.path)}
									</span>
									{file.truncated && (
										<span className="shrink-0 text-content-warning">
											(truncated)
										</span>
									)}
								</div>
							))}
						</div>
					)}
					{skillItems.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">Skills</span>
							<TooltipProvider delayDuration={300}>
								{skillItems.map((skill) => {
									const row = (
										<div className="flex items-center gap-1.5 rounded px-0.5 py-px transition-colors hover:bg-surface-tertiary">
											<ZapIcon className="size-3 shrink-0" />
											<span className="truncate">{skill.name}</span>
										</div>
									);
									if (!skill.description) {
										return <div key={skill.name}>{row}</div>;
									}
									return (
										<Tooltip key={skill.name}>
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
							</TooltipProvider>
						</div>
					)}
					{mcpItems.length > 0 && (
						<div className="flex flex-col gap-1">
							<span className="font-medium text-content-primary">MCP</span>
							<TooltipProvider delayDuration={300}>
								{mcpItems.map((mcp) => (
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
					<div className="flex flex-wrap gap-2">
						{changes.length > 0 && (
							<Button size="sm" variant="outline" onClick={openChanges}>
								View changes
							</Button>
						)}
						{onRefreshContext && (
							<Button
								size="sm"
								disabled={isRefreshingContext}
								onClick={() => onRefreshContext()}
							>
								<Spinner loading={isRefreshingContext} />
								Refresh context
							</Button>
						)}
					</div>
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

	const changesDialog = (
		<ContextChangesDialog
			open={changesOpen}
			onOpenChange={setChangesOpen}
			changes={changes}
			onRefreshContext={onRefreshContext}
			isRefreshingContext={isRefreshingContext}
		/>
	);

	// On mobile, a tap toggles the popover. On desktop, hover opens
	// it like a dropdown menu and skill descriptions appear as
	// nested tooltips to the right (same pattern as ModelSelector).
	if (isMobileViewport()) {
		return (
			<>
				<Popover>
					<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
					<PopoverContent
						side="top"
						className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto max-w-72 px-3 py-2"
					>
						{panelContent}
					</PopoverContent>
				</Popover>
				{changesDialog}
			</>
		);
	}

	return (
		<>
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
			{changesDialog}
		</>
	);
};
