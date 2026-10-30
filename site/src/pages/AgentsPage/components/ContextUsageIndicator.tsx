import { cn } from "cn";
import {
	BotIcon,
	CircleAlertIcon,
	FileIcon,
	PlugIcon,
	TriangleAlertIcon,
	ZapIcon,
} from "lucide-react";
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
	compactionThresholdLabel,
	compactionThresholdPercent,
	type ResolvedCompactionThreshold,
} from "../utils/compactionTriggers";
import { getPathBasename, getPathDirname } from "../utils/path";
import {
	ContextAlert,
	ContextAlertActions,
	ContextAlertDescription,
	ContextAlertTitle,
} from "./ContextAlert";
import {
	ContextResourceAccordion,
	ContextResourceAccordionContent,
	ContextResourceAccordionTrigger,
} from "./ContextResourceAccordion";
import {
	ContextResourceTree,
	ContextResourceTreeDirectory,
	ContextResourceTreeItem,
} from "./ContextResourceTree";

export type AgentContextUsage = {
	readonly usedTokens?: number;
	readonly estimated?: boolean;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	// Part of outputTokens, so usedTokens does not add it.
	readonly reasoningTokens?: number;
	readonly compactionThreshold?: ResolvedCompactionThreshold;
	// Pinned workspace-context state: the resources the chat is built from and
	// whether they have drifted from the agent's latest snapshot.
	readonly context?: ChatContext;
};

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
// A pinned resource the agent could not use, or an OK resource carrying a
// non-fatal warning, surfaced with its error so the problem is visible
// instead of silent.
type ContextIssueItem = {
	readonly name: string;
	readonly kind: ChatContextResourceKind;
	readonly outcome: ChatContextResourceStatus | "warning" | "failed";
	readonly error: string;
	readonly source: string;
};

// The outcome word shown next to an issue. Failed MCP servers carry the
// storage status "unreadable", which describes files, not connections.
const issueOutcome = (
	resource: ChatContextResource,
): ContextIssueItem["outcome"] => {
	if (resource.status === "ok") {
		return "warning";
	}
	return resource.kind === "mcp_server" ? "failed" : resource.status;
};

// Human-readable label per resource kind, used in the issues list.
const RESOURCE_KIND_LABELS: Record<ChatContextResourceKind, string> = {
	instruction_file: "file",
	skill: "skill",
	mcp_config: "MCP config",
	mcp_server: "MCP server",
	acp_harness: "ACP harness",
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

const getIndicatorToneClassName = (
	percentUsed: number | null,
	compactionPercent: number | undefined,
): string => {
	if (percentUsed === null) {
		return "text-content-secondary";
	}
	if (percentUsed >= 95) {
		return "text-content-destructive";
	}
	if (percentUsed >= (compactionPercent ?? 100) * 0.9) {
		return "text-content-warning";
	}
	return "text-content-secondary";
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

const ContextResourceIssues: React.FC<{
	issues: readonly ContextIssueItem[];
}> = ({ issues }) =>
	issues.length > 0 ? (
		<ContextAlert severity="warning" variant="panel">
			<ContextAlertTitle>Issues</ContextAlertTitle>
			<ContextAlertDescription>
				{issues.map((issue) => (
					<span
						key={`${issue.kind}:${issue.source}`}
						className="flex flex-col gap-1"
						title={issue.source}
					>
						<span className="truncate">
							{issue.name}{" "}
							<span className="text-content-secondary">
								({RESOURCE_KIND_LABELS[issue.kind]}: {issue.outcome})
							</span>
						</span>
						{issue.error && (
							<span className="wrap-anywhere">{issue.error}</span>
						)}
					</span>
				))}
			</ContextAlertDescription>
		</ContextAlert>
	) : null;

const ContextIssueIndicator: React.FC<{ count: number }> = ({ count }) =>
	count > 0 ? (
		<span className="ml-1 inline-flex align-middle text-content-warning">
			<TriangleAlertIcon aria-hidden="true" className="size-3" />
			<span className="sr-only">
				{count} {count === 1 ? "issue" : "issues"}
			</span>
		</span>
	) : null;

const ContextHarnessSection: React.FC<{
	groups: readonly DirectoryGroup<{ slug: string; dir: string }>[];
	issues: readonly ContextIssueItem[];
}> = ({ groups, issues }) => (
	<ContextResourceAccordion>
		<ContextResourceAccordionTrigger>
			ACP
			<ContextIssueIndicator count={issues.length} />
		</ContextResourceAccordionTrigger>
		<ContextResourceAccordionContent>
			{groups.length > 0 && (
				<ContextResourceTree>
					{groups.map((group) => (
						<ContextResourceTreeDirectory key={group.dir} dir={group.dir}>
							{group.items.map((harness) => (
								<ContextResourceTreeItem
									key={harness.slug}
									icon={group.dir === "" ? BotIcon : undefined}
									name={harness.slug}
									title={harness.slug}
								/>
							))}
						</ContextResourceTreeDirectory>
					))}
				</ContextResourceTree>
			)}
			<ContextResourceIssues issues={issues} />
		</ContextResourceAccordionContent>
	</ContextResourceAccordion>
);

export const ContextUsageIndicator: React.FC<{
	usage: AgentContextUsage;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, onRefreshContext, isRefreshingContext }) => {
	const usedTokens = hasFiniteTokenValue(usage.usedTokens)
		? usage.usedTokens
		: undefined;
	const contextLimitTokens = hasFiniteTokenValue(usage.contextLimitTokens)
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
		usage.usedTokens,
		usage.contextLimitTokens,
		usage.inputTokens,
		usage.outputTokens,
		usage.cacheReadTokens,
		usage.cacheCreationTokens,
		usage.reasoningTokens,
	].some(hasFiniteTokenValue);
	const percentLabel =
		percentUsed === null ? "--" : `${Math.round(percentUsed)}%`;
	const compactionLabel =
		usage.compactionThreshold && contextLimitTokens !== undefined
			? compactionThresholdLabel(usage.compactionThreshold, contextLimitTokens)
			: undefined;
	const compactionPercent =
		usage.compactionThreshold && contextLimitTokens !== undefined
			? compactionThresholdPercent(
					usage.compactionThreshold,
					contextLimitTokens,
				)
			: undefined;
	const clampedPercent = hasPercent
		? Math.min(Math.max(percentUsed, 0), 100)
		: 0;

	const context = usage.context;
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
	const hasMcpResources =
		mcpConfigItems.length > 0 || mcpServerItems.length > 0;
	const mcpServerCount = (pinnedResources ?? []).filter(
		(resource) => resource.kind === "mcp_server",
	).length;
	const mcpDiscovery = context?.mcp_discovery;
	const isMcpStale = mcpDiscovery?.stale ?? false;
	const isMcpPending = mcpDiscovery?.phase === "pending";
	const isMcpComplete = mcpDiscovery?.phase === "complete";
	let mcpDiscoveryNote = "";
	if (isMcpStale) {
		mcpDiscoveryNote =
			"Discovered by a previous agent run. Tools are withheld until your next message picks up the current agent's discovery.";
	} else if (isMcpPending) {
		mcpDiscoveryNote =
			"Discovery initializing. Servers may appear on a later turn.";
	} else if (isMcpComplete) {
		mcpDiscoveryNote =
			mcpServerCount === 0
				? "Discovery complete. No MCP servers discovered."
				: "Discovery complete.";
	}
	const hasMcp = hasMcpResources || mcpDiscoveryNote !== "";
	const harnessItems = (pinnedResources ?? [])
		.filter(
			(resource) => resource.kind === "acp_harness" && resource.status === "ok",
		)
		.map((resource) => ({
			slug: resource.source,
			dir: resource.source_path ?? "",
		}))
		.filter((harness) => harness.slug.trim().length > 0);
	// Pinned resources the agent could not use (invalid skill, unreadable or
	// oversize file) are surfaced as issues with their error so the failure is
	// visible rather than a silent omission. An OK row may also carry a
	// non-fatal warning (for example an MCP server still serving the tools of
	// its previous connection); it stays listed as usable and the warning is
	// surfaced here.
	const issueItems: readonly ContextIssueItem[] = (pinnedResources ?? [])
		.filter(
			(resource) => resource.status !== "ok" || (resource.error ?? "") !== "",
		)
		.map(
			(resource): ContextIssueItem => ({
				name:
					resource.skill_name ||
					getPathBasename(resource.source) ||
					resource.source,
				kind: resource.kind,
				outcome: issueOutcome(resource),
				error: resource.error ?? "",
				source: resource.source,
			}),
		)
		.filter((issue) => issue.name.trim().length > 0);
	const fileIssues = issueItems.filter(
		(issue) => issue.kind === "instruction_file",
	);
	const skillIssues = issueItems.filter((issue) => issue.kind === "skill");
	const mcpIssues = issueItems.filter(
		(issue) => issue.kind === "mcp_config" || issue.kind === "mcp_server",
	);
	const harnessIssues = issueItems.filter(
		(issue) => issue.kind === "acp_harness",
	);

	const hasContextList =
		fileItems.length > 0 ||
		skillItems.length > 0 ||
		hasMcp ||
		harnessItems.length > 0 ||
		issueItems.length > 0;

	const hasResourceFailures = issueItems.some(
		(issue) => issue.outcome !== "warning",
	);
	const hasResourceWarnings = issueItems.some(
		(issue) => issue.outcome === "warning",
	);
	const hasResourceIssues = issueItems.length > 0;
	const needsAttention =
		isDirty || hasContextError || hasResourceIssues || isMcpStale;
	const AttentionIcon = hasContextError ? CircleAlertIcon : TriangleAlertIcon;
	const toneClassName = hasContextError
		? "text-content-destructive"
		: isDirty || hasResourceIssues || isMcpStale
			? "text-content-warning"
			: getIndicatorToneClassName(percentUsed, compactionPercent);

	// Group files and skills by directory so every context root is labeled,
	// keeping resources pulled from different directories distinguishable.
	const fileGroups = groupByDirectory(fileItems);
	const skillGroups = groupByDirectory(skillItems);

	const statusNotes = [
		hasContextError ? "Context error." : "",
		isDirty ? "Context changed." : "",
		hasResourceFailures ? "Some context resources failed to load." : "",
		hasResourceWarnings ? "Some context resources reported warnings." : "",
		isMcpStale ? "Workspace MCP tools are from a previous agent run." : "",
	].filter((note) => note !== "");
	const statusNote = statusNotes.length > 0 ? ` ${statusNotes.join(" ")}` : "";
	let ariaLabel = "Context usage";
	if (hasPercent) {
		const label = usage.estimated ? "Estimated context usage" : "Context usage";
		ariaLabel = `${label} ${percentLabel}. ${formatTokenCount(usedTokens)} of ${formatTokenCount(contextLimitTokens)} tokens used.${statusNote}`;
	} else if (statusNote !== "") {
		ariaLabel = `Context usage.${statusNote}`;
	}

	const triggerButton = (
		<button
			type="button"
			aria-label={ariaLabel}
			className="inline-flex h-7 shrink-0 items-center gap-2 rounded-md border-none bg-transparent px-1 text-xs outline-hidden transition-colors hover:bg-surface-secondary/60 focus-visible:ring-2 focus-visible:ring-content-link"
		>
			<span
				aria-hidden="true"
				className="relative h-2 w-10 rounded-xs bg-surface-tertiary"
			>
				<span
					className={cn(
						"block h-full rounded-xs bg-current",
						clampedPercent > 0 && "min-w-1",
						getIndicatorToneClassName(percentUsed, compactionPercent),
					)}
					style={{ width: `${clampedPercent}%` }}
				/>
				{compactionPercent !== undefined && (
					<span
						className="absolute -top-0.5 h-3 w-0.5 -translate-x-1/2 bg-content-primary"
						style={{ left: `${compactionPercent}%` }}
					/>
				)}
			</span>
			<span className="text-content-secondary tabular-nums">
				{percentLabel}
			</span>
			{needsAttention && (
				<AttentionIcon
					aria-hidden="true"
					className={cn("size-3 shrink-0", toneClassName)}
				/>
			)}
		</button>
	);

	return (
		<Popover>
			<PopoverTrigger asChild>{triggerButton}</PopoverTrigger>
			<PopoverContent
				side="top"
				align="end"
				className="mobile-full-width-dropdown w-76 text-xs pb-0.5 font-normal"
			>
				<div className="text-xs text-content-primary p-3">
					<div className="text-sm font-semibold">Context</div>
					{hasReportedUsage ? (
						hasPercent ? (
							<div className="flex gap-2 items-center pt-2">
								<div
									className="flex-1 bg-surface-tertiary rounded-xs h-2 relative"
									style={{
										"--data-compactsAt": compactionPercent,
										"--data-percentUsed": clampedPercent,
									}}
								>
									<div
										className={cn(
											"h-2 w-[calc(var(--data-percentUsed)*1%)] bg-current rounded-xs",
											clampedPercent > 0 && "min-w-1",
											getIndicatorToneClassName(percentUsed, compactionPercent),
										)}
									/>
									{compactionPercent !== undefined && (
										<div className="absolute top-0 left-[calc(var(--data-compactsAt)*1%)] h-2 w-0.5 -translate-x-1/2 bg-content-primary" />
									)}
								</div>
								<span
									className={getIndicatorToneClassName(
										percentUsed,
										compactionPercent,
									)}
								>
									{usage.estimated && "~"}
									{percentLabel}
								</span>
								<span className="text-content-secondary">
									({formatTokenCountCompact(usedTokens)}/
									{formatTokenCountCompact(contextLimitTokens)})
								</span>
							</div>
						) : (
							<div
								role="status"
								className="mt-2 rounded-lg border border-dashed border-border px-3 py-4 text-center"
							>
								<h3 className="m-0 text-xs font-semibold text-content-primary">
									Context usage unavailable.
								</h3>
							</div>
						)
					) : (
						<div
							role="status"
							className="mt-2 rounded-lg border border-dashed border-border px-3 py-4 text-center"
						>
							<h3 className="m-0 text-xs font-semibold text-content-primary">
								Usage not available yet
							</h3>
							<p className="mb-0 mt-2 text-xs font-normal text-content-secondary max-w-48 mx-auto leading-5">
								Context usage will appear after sending a message.
							</p>
						</div>
					)}
					{hasPercent && compactionLabel !== undefined && (
						<div className="mt-2 text-content-secondary">{compactionLabel}</div>
					)}
					{hasPercent && usage.estimated && (
						<ContextAlert
							severity="info"
							variant="panel"
							className="mt-2 font-normal"
						>
							<ContextAlertTitle className="font-normal">
								Estimated usage
							</ContextAlertTitle>
							<ContextAlertDescription>
								Based on the compacted summary only, excluding other prompt
								content and tools. Replaced by measured usage after the next
								response.
							</ContextAlertDescription>
						</ContextAlert>
					)}
				</div>
				{hasContextList && (
					<div className="flex flex-col text-content-secondary px-1.5">
						{(fileItems.length > 0 || fileIssues.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${fileItems.length} ${fileItems.length === 1 ? "file" : "files"}`}
								>
									Context files
									<ContextIssueIndicator count={fileIssues.length} />
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent>
									{fileItems.length > 0 && (
										<ContextResourceTree>
											{fileGroups.map((group) => (
												<ContextResourceTreeDirectory
													key={group.dir}
													dir={group.dir}
												>
													{group.items.map((file) => (
														<ContextResourceTreeItem
															key={file.path}
															icon={group.dir === "" ? FileIcon : undefined}
															name={getPathBasename(file.path)}
															title={file.path}
														/>
													))}
												</ContextResourceTreeDirectory>
											))}
										</ContextResourceTree>
									)}
									<ContextResourceIssues issues={fileIssues} />
								</ContextResourceAccordionContent>
							</ContextResourceAccordion>
						)}
						{(skillItems.length > 0 || skillIssues.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${skillItems.length} available`}
								>
									Skills
									<ContextIssueIndicator count={skillIssues.length} />
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent>
									{skillItems.length > 0 && (
										<ContextResourceTree>
											{skillGroups.map((group) => (
												<ContextResourceTreeDirectory
													key={group.dir}
													dir={group.dir}
												>
													{group.items.map((skill) => (
														<ContextResourceTreeItem
															key={skill.source}
															icon={group.dir === "" ? ZapIcon : undefined}
															name={skill.name}
															description={skill.description}
															className="text-content-primary"
														/>
													))}
												</ContextResourceTreeDirectory>
											))}
										</ContextResourceTree>
									)}
									<ContextResourceIssues issues={skillIssues} />
								</ContextResourceAccordionContent>
							</ContextResourceAccordion>
						)}
						{(hasMcp || mcpIssues.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${mcpServerItems.length} connected`}
								>
									MCP Servers
									<ContextIssueIndicator count={mcpIssues.length} />
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent>
									{mcpDiscoveryNote !== "" && (
										<span
											className={cn(
												"flex items-center gap-1.5",
												isMcpStale && "text-content-warning",
											)}
										>
											{isMcpStale && (
												<TriangleAlertIcon className="size-3 shrink-0" />
											)}
											{isMcpPending && (
												<Spinner loading className="size-3! shrink-0" />
											)}
											<span>{mcpDiscoveryNote}</span>
										</span>
									)}
									{hasMcpResources && (
										<ContextResourceTree>
											{mcpConfigItems.map((config) => (
												<ContextResourceTreeItem
													key={config.source}
													icon={FileIcon}
													name={config.source}
													title={config.source}
												/>
											))}
											{mcpServerItems.map((mcp) => (
												<ContextResourceTreeItem
													key={mcp.source}
													icon={PlugIcon}
													name={mcp.name}
													title={mcp.source}
												>
													{mcp.tools.length > 0 ? (
														mcp.tools.map((tool) => (
															<ContextResourceTreeItem
																key={tool.name}
																name={tool.name}
																description={tool.description}
															/>
														))
													) : (
														<span className="text-content-secondary">
															(no tools)
														</span>
													)}
												</ContextResourceTreeItem>
											))}
										</ContextResourceTree>
									)}
									<ContextResourceIssues issues={mcpIssues} />
								</ContextResourceAccordionContent>
							</ContextResourceAccordion>
						)}
						{(harnessItems.length > 0 || harnessIssues.length > 0) && (
							<ContextHarnessSection
								groups={groupByDirectory(harnessItems)}
								issues={harnessIssues}
							/>
						)}
					</div>
				)}
				{(isDirty || hasContextError) && (
					<ContextAlert
						severity={hasContextError ? "error" : "warning"}
						variant="panel"
						className="mx-3 mb-3 mt-2"
					>
						<ContextAlertTitle>
							{hasContextError ? "Context error" : "Context changed"}
						</ContextAlertTitle>
						<ContextAlertDescription>
							{hasContextError
								? contextError
								: "The workspace context changed since this chat was pinned."}
							{onRefreshContext && (
								<ContextAlertActions>
									<Button
										size="xs"
										disabled={isRefreshingContext}
										onClick={() => onRefreshContext()}
										variant="outline"
									>
										<Spinner size="sm" loading={isRefreshingContext} />
										Refresh context
									</Button>
								</ContextAlertActions>
							)}
						</ContextAlertDescription>
					</ContextAlert>
				)}
			</PopoverContent>
		</Popover>
	);
};
