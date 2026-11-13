import { cn } from "cn";
import {
	BlocksIcon,
	BotIcon,
	CircleAlertIcon,
	FileIcon,
	TriangleAlertIcon,
	ZapIcon,
} from "lucide-react";
import { Fragment } from "react";
import type {
	ChatContext,
	ChatContextResource,
	ChatContextResourceKind,
	ChatContextResourceStatus,
	ChatContextTool,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
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
	ContextResourceServer,
	ContextResourceTool,
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

// Groups by parent directory or Agent Plugin so basename-only rows (a
// repo-root AGENTS.md and a nested one) stay distinguishable. A directory
// label with an empty name renders without a header.
type ContextGroupLabel = {
	readonly kind: "directory" | "plugin";
	readonly name: string;
};

// Normalized popover entries, sourced from the chat's pinned context
// resources.
type ContextFileItem = {
	readonly path: string;
	readonly label: ContextGroupLabel;
};
type ContextSkillItem = {
	readonly source: string;
	readonly name: string;
	readonly description?: string;
	readonly label: ContextGroupLabel;
};
type ContextPluginItem = {
	readonly name: string;
	readonly source: string;
};
// MCP configs are file-backed (shown by full path), while MCP servers are
// keyed by source, not by display name, and carry their tools.
type ContextMcpConfigItem = { readonly source: string };
type ContextMcpServerItem = {
	readonly name: string;
	readonly source: string;
	readonly label: ContextGroupLabel;
	readonly connected: boolean;
	readonly tools: readonly ChatContextTool[];
};
type ContextHarnessItem = {
	readonly slug: string;
	readonly label: ContextGroupLabel;
};
// A pinned resource that failed to load, or an OK resource whose error field
// carries a non-fatal warning.
type ContextIssueItem = {
	readonly name: string;
	readonly kind: ChatContextResourceKind;
	readonly status: ChatContextResourceStatus;
	readonly statusLabel: string;
	readonly error: string;
	readonly source: string;
};

type ContextGroup<T> = {
	readonly key: string;
	readonly label: ContextGroupLabel;
	readonly items: readonly T[];
};

type ContextPanelModel = {
	readonly fileGroups: readonly ContextGroup<ContextFileItem>[];
	readonly skillGroups: readonly ContextGroup<ContextSkillItem>[];
	readonly plugins: readonly ContextPluginItem[];
	readonly mcpConfigs: readonly ContextMcpConfigItem[];
	// Servers without a plugin come first so they never render under a
	// plugin header.
	readonly mcpServerGroups: readonly ContextGroup<ContextMcpServerItem>[];
	readonly harnessGroups: readonly ContextGroup<ContextHarnessItem>[];
	readonly issues: readonly ContextIssueItem[];
	readonly fileIssues: readonly ContextIssueItem[];
	readonly skillIssues: readonly ContextIssueItem[];
	readonly pluginIssues: readonly ContextIssueItem[];
	// MCP config issues and warnings on running servers.
	readonly mcpIssues: readonly ContextIssueItem[];
	// Servers that are not running.
	readonly mcpServerFailures: readonly ContextIssueItem[];
	readonly harnessIssues: readonly ContextIssueItem[];
	readonly hasResourceIssues: boolean;
	readonly hasResourceFailures: boolean;
};

// Human-readable label per resource kind, used in the issues list.
const RESOURCE_KIND_LABELS: Record<ChatContextResourceKind, string> = {
	instruction_file: "file",
	skill: "skill",
	mcp_config: "MCP config",
	mcp_server: "MCP server",
	acp_harness: "ACP harness",
	plugin: "plugin manifest",
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

type IndicatorTone = "default" | "warning" | "destructive";

const INDICATOR_TONE_CLASS_NAMES: Record<IndicatorTone, string> = {
	default: "text-content-secondary",
	warning: "text-content-warning",
	destructive: "text-content-destructive",
};

const getUsageTone = (
	percentUsed: number | null,
	compactionPercent: number | undefined,
): IndicatorTone => {
	if (percentUsed === null) {
		return "default";
	}
	if (percentUsed >= 95) {
		return "destructive";
	}
	if (percentUsed >= (compactionPercent ?? 100) * 0.9) {
		return "warning";
	}
	return "default";
};

const getIndicatorToneClassName = (
	percentUsed: number | null,
	compactionPercent: number | undefined,
): string =>
	INDICATOR_TONE_CLASS_NAMES[getUsageTone(percentUsed, compactionPercent)];

// Any resource issue marks the indicator, including warnings on OK rows:
// some warnings are the only report that a resource was skipped.
export const getIndicatorState = ({
	percentUsed,
	compactionPercent,
	isDirty,
	hasContextError,
	hasResourceIssues,
}: {
	percentUsed: number | null;
	compactionPercent?: number;
	isDirty: boolean;
	hasContextError: boolean;
	hasResourceIssues: boolean;
}): { tone: IndicatorTone; needsAttention: boolean } => {
	const needsAttention = isDirty || hasContextError || hasResourceIssues;
	if (hasContextError) {
		return { tone: "destructive", needsAttention };
	}
	if (needsAttention) {
		return { tone: "warning", needsAttention };
	}
	return {
		tone: getUsageTone(percentUsed, compactionPercent),
		needsAttention,
	};
};

// Merges items with equal labels even when they are not adjacent. Groups keep
// first-seen order so the popover layout stays stable across renders.
const groupByLabel = <T extends { readonly label: ContextGroupLabel }>(
	items: readonly T[],
): readonly ContextGroup<T>[] => {
	const groups: { key: string; label: ContextGroupLabel; items: T[] }[] = [];
	const byKey = new Map<string, T[]>();
	for (const item of items) {
		const key = `${item.label.kind}:${item.label.name}`;
		const existing = byKey.get(key);
		if (existing) {
			existing.push(item);
		} else {
			const bucket = [item];
			byKey.set(key, bucket);
			groups.push({ key, label: item.label, items: bucket });
		}
	}
	return groups;
};

const countGroupItems = (groups: readonly ContextGroup<unknown>[]): number =>
	groups.reduce((count, group) => count + group.items.length, 0);

const directoryGroupLabel = (source: string): ContextGroupLabel => ({
	kind: "directory",
	name: getPathDirname(source),
});

const resourceGroupLabel = (
	resource: ChatContextResource,
): ContextGroupLabel =>
	resource.plugin_name
		? { kind: "plugin", name: resource.plugin_name }
		: directoryGroupLabel(resource.source);

const groupHeaderText = (label: ContextGroupLabel): string =>
	label.kind === "plugin" ? `plugin: ${label.name}` : label.name;

// Plugin MCP servers are sourced as "<plugin>/<server>".
const mcpServerDisplayName = (resource: ChatContextResource): string =>
	resource.plugin_name
		? resource.source.slice(resource.plugin_name.length + 1)
		: resource.source;

const issueDisplayName = (resource: ChatContextResource): string => {
	const base =
		resource.kind === "mcp_server"
			? mcpServerDisplayName(resource)
			: resource.skill_name ||
				getPathBasename(resource.source) ||
				resource.source;
	if (resource.kind === "plugin") {
		// Rejected manifests carry no plugin name.
		return resource.plugin_name || base;
	}
	return resource.plugin_name ? `${resource.plugin_name}/${base}` : base;
};

// An OK row with a non-empty error appears in its section and in issues.
export const buildContextPanelModel = (
	resources: readonly ChatContextResource[],
): ContextPanelModel => {
	const okResources = resources.filter((resource) => resource.status === "ok");
	// Drop entries with no usable name or path so an empty marker never
	// renders as a blank row.
	const files: ContextFileItem[] = okResources
		.filter((resource) => resource.kind === "instruction_file")
		.map((resource) => ({
			path: resource.source,
			label: directoryGroupLabel(resource.source),
		}))
		.filter((file) => file.path.trim().length > 0);
	const skills: ContextSkillItem[] = okResources
		.filter((resource) => resource.kind === "skill")
		.map((resource) => ({
			source: resource.source,
			name: resource.skill_name || getPathBasename(resource.source),
			description: resource.skill_description,
			label: resourceGroupLabel(resource),
		}))
		.filter((skill) => skill.name.trim().length > 0);
	const plugins: ContextPluginItem[] = okResources
		.filter((resource) => resource.kind === "plugin")
		.map((resource) => ({
			name: resource.plugin_name ?? "",
			source: resource.source,
		}))
		.filter((plugin) => plugin.name.trim().length > 0);
	// MCP configs are shown by their full path so multiple .mcp.json files
	// (e.g. ~/.mcp.json and ~/project/.mcp.json) stay disambiguated.
	const mcpConfigs: ContextMcpConfigItem[] = okResources
		.filter((resource) => resource.kind === "mcp_config")
		.map((resource) => ({ source: resource.source }))
		.filter((config) => config.source.trim().length > 0);
	// Servers in any status are listed so disconnected ones stay visible.
	const mcpServers: ContextMcpServerItem[] = resources
		.filter((resource) => resource.kind === "mcp_server")
		.map((resource) => ({
			name: mcpServerDisplayName(resource),
			source: resource.source,
			label: resource.plugin_name
				? { kind: "plugin" as const, name: resource.plugin_name }
				: { kind: "directory" as const, name: "" },
			connected: resource.status === "ok",
			tools: resource.tools ?? [],
		}))
		.filter((server) => server.name.trim().length > 0);
	const harnesses: ContextHarnessItem[] = okResources
		.filter((resource) => resource.kind === "acp_harness")
		.map((resource) => ({
			slug: resource.source,
			label: { kind: "directory" as const, name: resource.source_path ?? "" },
		}))
		.filter((harness) => harness.slug.trim().length > 0);
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
	const issuesOfKind = (...kinds: ChatContextResourceKind[]) =>
		issues.filter((issue) => kinds.includes(issue.kind));

	return {
		fileGroups: groupByLabel(files),
		skillGroups: groupByLabel(skills),
		plugins,
		mcpConfigs,
		mcpServerGroups: groupByLabel([
			...mcpServers.filter((server) => server.label.kind !== "plugin"),
			...mcpServers.filter((server) => server.label.kind === "plugin"),
		]),
		harnessGroups: groupByLabel(harnesses),
		issues,
		fileIssues: issuesOfKind("instruction_file"),
		skillIssues: issuesOfKind("skill"),
		pluginIssues: issuesOfKind("plugin"),
		mcpIssues: issues.filter(
			(issue) =>
				issue.kind === "mcp_config" ||
				(issue.kind === "mcp_server" && issue.status === "ok"),
		),
		mcpServerFailures: issues.filter(
			(issue) => issue.kind === "mcp_server" && issue.status !== "ok",
		),
		harnessIssues: issuesOfKind("acp_harness"),
		hasResourceIssues: issues.length > 0,
		hasResourceFailures: issues.some((issue) => issue.status !== "ok"),
	};
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
								({RESOURCE_KIND_LABELS[issue.kind]}: {issue.statusLabel})
							</span>
						</span>
						{issue.error && <span>{issue.error}</span>}
					</span>
				))}
			</ContextAlertDescription>
		</ContextAlert>
	) : null;

type ContextMcpServerIssuesProps = {
	issues: readonly ContextIssueItem[];
	onManageServers?: () => void;
};

const ContextMcpServerIssues: React.FC<ContextMcpServerIssuesProps> = ({
	issues,
	onManageServers,
}) =>
	issues.length > 0 ? (
		<ContextAlert
			severity="warning"
			variant="panel"
			className="gap-y-1 leading-4.5"
		>
			<ContextAlertTitle className="font-normal leading-4.5">
				{issues.length} {issues.length === 1 ? "server" : "servers"} not
				running:
			</ContextAlertTitle>
			<ContextAlertDescription className="gap-1 break-words">
				<span>{issues.map((issue) => issue.name).join(", ")}</span>
				{onManageServers && (
					<Link
						asChild
						size="sm"
						showExternalIcon={false}
						className="w-fit border-0 bg-transparent pl-0 font-normal leading-4.5"
					>
						<button type="button" onClick={onManageServers}>
							Manage servers
						</button>
					</Link>
				)}
			</ContextAlertDescription>
		</ContextAlert>
	) : null;

const ContextHarnessSection: React.FC<{
	groups: readonly ContextGroup<ContextHarnessItem>[];
	issues: readonly ContextIssueItem[];
}> = ({ groups, issues }) => (
	<ContextResourceAccordion>
		<ContextResourceAccordionTrigger
			detail={`${countGroupItems(groups)} available`}
		>
			ACP
		</ContextResourceAccordionTrigger>
		<ContextResourceAccordionContent>
			{groups.length > 0 && (
				<ContextResourceTree>
					{groups.map((group) => (
						<ContextResourceTreeDirectory
							key={group.key}
							dir={group.label.name}
						>
							{group.items.map((harness) => (
								<ContextResourceTreeItem
									key={harness.slug}
									icon={group.label.name === "" ? BotIcon : undefined}
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
	onManageServers?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, onRefreshContext, onManageServers, isRefreshingContext }) => {
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
	const {
		fileGroups,
		skillGroups,
		plugins,
		mcpConfigs,
		mcpServerGroups,
		harnessGroups,
		issues,
		fileIssues,
		skillIssues,
		pluginIssues,
		mcpIssues,
		mcpServerFailures,
		harnessIssues,
		hasResourceIssues,
		hasResourceFailures,
	} = buildContextPanelModel(context?.resources ?? []);
	const fileCount = countGroupItems(fileGroups);
	const skillCount = countGroupItems(skillGroups);
	const mcpServers = mcpServerGroups.flatMap((group) => group.items);
	const hasMcp = mcpConfigs.length > 0 || mcpServers.length > 0;

	const hasContextList =
		fileGroups.length > 0 ||
		skillGroups.length > 0 ||
		plugins.length > 0 ||
		hasMcp ||
		harnessGroups.length > 0 ||
		issues.length > 0;

	const { tone, needsAttention } = getIndicatorState({
		percentUsed,
		compactionPercent,
		isDirty,
		hasContextError,
		hasResourceIssues,
	});
	const AttentionIcon = hasContextError ? CircleAlertIcon : TriangleAlertIcon;
	const toneClassName = INDICATOR_TONE_CLASS_NAMES[tone];

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
				className="mobile-full-width-dropdown w-76 text-xs pb-1 font-normal"
			>
				<div className="text-xs text-content-primary px-4 pt-2 pb-3">
					<div className="pt-1 text-sm font-semibold leading-6">Context</div>
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
								className="mt-2 rounded-lg border border-dashed px-3 py-4 text-center"
							>
								<h3 className="m-0 text-xs font-semibold text-content-primary">
									Context usage unavailable.
								</h3>
							</div>
						)
					) : (
						<div
							role="status"
							className="mt-2 rounded-lg border border-dashed px-3 py-4 text-center"
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
					<div className="flex flex-col text-content-secondary">
						{(fileCount > 0 || fileIssues.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${fileCount} ${fileCount === 1 ? "file" : "files"}`}
								>
									Context files
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent>
									{fileCount > 0 && (
										<ContextResourceTree>
											{fileGroups.map((group) => (
												<ContextResourceTreeDirectory
													key={group.key}
													dir={group.label.name}
												>
													{group.items.map((file) => (
														<ContextResourceTreeItem
															key={file.path}
															icon={
																group.label.name === "" ? FileIcon : undefined
															}
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
						{(plugins.length > 0 || pluginIssues.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${plugins.length} available`}
								>
									Plugins
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent>
									{plugins.length > 0 && (
										<ContextResourceTree>
											{plugins.map((plugin) => (
												<ContextResourceTreeItem
													key={plugin.source}
													icon={BlocksIcon}
													name={plugin.name}
													title={plugin.source}
												/>
											))}
										</ContextResourceTree>
									)}
									<ContextResourceIssues issues={pluginIssues} />
								</ContextResourceAccordionContent>
							</ContextResourceAccordion>
						)}
						{(skillCount > 0 || skillIssues.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${skillCount} available`}
								>
									Skills
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent>
									{skillCount > 0 && (
										<ContextResourceTree>
											{skillGroups.map((group) => (
												<ContextResourceTreeDirectory
													key={group.key}
													dir={groupHeaderText(group.label)}
													icon={
														group.label.kind === "plugin"
															? BlocksIcon
															: undefined
													}
												>
													{group.items.map((skill) => (
														<ContextResourceTreeItem
															key={skill.source}
															icon={
																group.label.name === "" ? ZapIcon : undefined
															}
															name={skill.name}
															description={skill.description}
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
						{(hasMcp ||
							mcpIssues.length > 0 ||
							mcpServerFailures.length > 0) && (
							<ContextResourceAccordion>
								<ContextResourceAccordionTrigger
									detail={`${mcpServers.filter((server) => server.connected).length} of ${mcpServers.length} connected`}
								>
									MCP Servers
								</ContextResourceAccordionTrigger>
								<ContextResourceAccordionContent className="gap-2">
									{hasMcp && (
										<ContextResourceTree>
											{mcpConfigs.map((config) => (
												<ContextResourceTreeItem
													key={config.source}
													icon={FileIcon}
													name={config.source}
													title={config.source}
													appearance="path"
												/>
											))}
											{mcpServerGroups.map((group) => (
												<Fragment key={group.key}>
													{group.label.kind === "plugin" && (
														<ContextResourceTreeItem
															icon={BlocksIcon}
															name={groupHeaderText(group.label)}
															title={groupHeaderText(group.label)}
															appearance="path"
														/>
													)}
													{group.items.map((mcp) => (
														<ContextResourceServer
															key={mcp.source}
															name={mcp.name}
															title={mcp.source}
															connected={mcp.connected}
														>
															{mcp.tools.length > 0 &&
																mcp.tools.map((tool) => (
																	<ContextResourceTool
																		key={tool.name}
																		name={tool.name}
																		description={tool.description}
																	/>
																))}
														</ContextResourceServer>
													))}
												</Fragment>
											))}
										</ContextResourceTree>
									)}
									<ContextResourceIssues issues={mcpIssues} />
									<ContextMcpServerIssues
										issues={mcpServerFailures}
										onManageServers={onManageServers}
									/>
								</ContextResourceAccordionContent>
							</ContextResourceAccordion>
						)}
						{(harnessGroups.length > 0 || harnessIssues.length > 0) && (
							<ContextHarnessSection
								groups={harnessGroups}
								issues={harnessIssues}
							/>
						)}
					</div>
				)}
				{(isDirty || hasContextError) && (
					<div className="px-4 py-3">
						<ContextAlert
							severity={hasContextError ? "error" : "warning"}
							variant="panel"
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
					</div>
				)}
			</PopoverContent>
		</Popover>
	);
};
