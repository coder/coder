import { cn } from "cn";
import {
	ChevronDownIcon,
	FileIcon,
	FolderIcon,
	PlugIcon,
	TriangleAlertIcon,
	WrenchIcon,
} from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import type {
	ChatContext,
	ChatContextResource,
	ChatContextResourceKind,
	ChatContextTool,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { getPathBasename, getPathDirname } from "../utils/path";

export interface AgentContextUsage {
	readonly usedTokens?: number;
	readonly estimated?: boolean;
	readonly contextLimitTokens?: number;
	readonly inputTokens?: number;
	readonly outputTokens?: number;
	readonly cacheReadTokens?: number;
	readonly cacheCreationTokens?: number;
	readonly reasoningTokens?: number;
	readonly compressionThreshold?: number;
	readonly context?: ChatContext;
}

type DirectoryItem = {
	readonly source: string;
	readonly name: string;
	readonly description?: string;
	readonly dir: string;
};

type MCPServerItem = {
	readonly source: string;
	readonly name: string;
	readonly tools: readonly ChatContextTool[];
	readonly error?: string;
	readonly connected: boolean;
};

type DirectoryGroup = {
	readonly dir: string;
	readonly items: readonly DirectoryItem[];
};

const hasFiniteTokenValue = (value: number | undefined): value is number =>
	typeof value === "number" && Number.isFinite(value) && value >= 0;

const formatTokenCountCompact = (value: number | undefined): string => {
	if (!hasFiniteTokenValue(value)) {
		return "--";
	}
	if (value >= 1_000_000) {
		const millions = value / 1_000_000;
		return `${Number.isInteger(millions) ? millions : millions.toFixed(1).replace(/\.0$/, "")}M`;
	}
	if (value >= 1_000) {
		const thousands = value / 1_000;
		return `${Number.isInteger(thousands) ? thousands : thousands.toFixed(1).replace(/\.0$/, "")}K`;
	}
	return String(value);
};

const groupByDirectory = (
	items: readonly DirectoryItem[],
): DirectoryGroup[] => {
	const groups = new Map<string, DirectoryItem[]>();
	for (const item of items) {
		const group = groups.get(item.dir);
		if (group) {
			group.push(item);
		} else {
			groups.set(item.dir, [item]);
		}
	}
	return [...groups].map(([dir, groupItems]) => ({ dir, items: groupItems }));
};

const resourceItems = (
	resources: readonly ChatContextResource[],
	kind: ChatContextResourceKind,
): DirectoryItem[] =>
	resources
		.filter((resource) => resource.kind === kind && resource.status === "ok")
		.map((resource) => ({
			source: resource.source,
			name:
				kind === "skill"
					? resource.skill_name || getPathBasename(resource.source)
					: getPathBasename(resource.source),
			description: resource.skill_description,
			dir: getPathDirname(resource.source),
		}))
		.filter((item) => item.name.trim() !== "");

const resourceIssues = (
	resources: readonly ChatContextResource[],
	kinds: readonly ChatContextResourceKind[],
): ChatContextResource[] =>
	resources.filter(
		(resource) => kinds.includes(resource.kind) && resource.status !== "ok",
	);

const DirectoryLabel: FC<{ dir: string }> = ({ dir }) => (
	<div className="flex min-w-0 items-center gap-2 text-sm text-content-secondary">
		<FolderIcon className="size-4 shrink-0" />
		<span className="truncate" title={dir}>
			{dir}
		</span>
	</div>
);

const DirectoryList: FC<{ groups: readonly DirectoryGroup[] }> = ({
	groups,
}) => (
	<div className="flex flex-col gap-4">
		{groups.map((group) => (
			<div key={group.dir} className="flex flex-col gap-2">
				{group.dir !== "" && <DirectoryLabel dir={group.dir} />}
				<div className={cn("flex flex-col gap-2", group.dir !== "" && "ml-6")}>
					{group.items.map((item) => (
						<div
							key={item.source}
							className="min-w-0 text-sm text-content-secondary"
						>
							<div className="flex items-center gap-2">
								<FileIcon className="size-4 shrink-0" />
								<span className="truncate" title={item.source}>
									{item.name}
								</span>
							</div>
							{item.description && (
								<p className="m-0 ml-6 mt-1 text-xs text-content-secondary">
									{item.description}
								</p>
							)}
						</div>
					))}
				</div>
			</div>
		))}
	</div>
);

const IssueList: FC<{ issues: readonly ChatContextResource[] }> = ({
	issues,
}) =>
	issues.length > 0 ? (
		<div className="flex flex-col gap-2 rounded-md border border-border-warning bg-surface-orange p-3 text-xs">
			{issues.map((issue) => (
				<div key={`${issue.kind}:${issue.source}`}>
					<div className="font-medium text-content-warning">
						{getPathBasename(issue.source) || issue.source}
					</div>
					<div className="text-content-secondary">
						{issue.error ||
							`This ${issue.kind.replace("_", " ")} is ${issue.status}.`}
					</div>
				</div>
			))}
		</div>
	) : null;

interface SummarySectionProps {
	label: string;
	meta: ReactNode;
	disabled?: boolean;
	warning?: boolean;
	children: ReactNode;
}

const SummarySection: FC<SummarySectionProps> = ({
	label,
	meta,
	disabled = false,
	warning = false,
	children,
}) => {
	const [open, setOpen] = useState(false);
	return (
		<Collapsible open={open} onOpenChange={setOpen} disabled={disabled}>
			<CollapsibleTrigger asChild>
				<button
					type="button"
					disabled={disabled}
					className="flex min-h-14 w-full items-center gap-3 border-0 bg-transparent px-4 py-3 text-left text-sm text-content-primary enabled:cursor-pointer disabled:cursor-default disabled:text-content-disabled"
				>
					<span className="font-semibold">{label}</span>
					<span className="ml-auto flex min-w-0 items-center gap-3 text-content-secondary">
						{warning && (
							<>
								<TriangleAlertIcon className="size-4 shrink-0 text-content-warning" />
								<span className="sr-only">Warning</span>
							</>
						)}
						<span className="truncate">{meta}</span>
						{!disabled && (
							<ChevronDownIcon
								className={cn(
									"size-4 shrink-0 transition-transform",
									open && "rotate-180",
								)}
							/>
						)}
					</span>
				</button>
			</CollapsibleTrigger>
			<CollapsibleContent className="px-4 pb-5">{children}</CollapsibleContent>
		</Collapsible>
	);
};

export const ChatSummaryResources: FC<{
	usage: AgentContextUsage | null;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, onRefreshContext, isRefreshingContext }) => {
	const resources = usage?.context?.resources ?? [];
	const files = resourceItems(resources, "instruction_file");
	const skills = resourceItems(resources, "skill");
	const fileIssues = resourceIssues(resources, ["instruction_file"]);
	const skillIssues = resourceIssues(resources, ["skill"]);
	const mcpIssues = resourceIssues(resources, ["mcp_config", "mcp_server"]);
	const mcpConfigs = resources.filter(
		(resource) => resource.kind === "mcp_config" && resource.status === "ok",
	);
	const mcpServers: MCPServerItem[] = resources
		.filter((resource) => resource.kind === "mcp_server")
		.map((resource) => ({
			source: resource.source,
			name: resource.source,
			tools: resource.tools ?? [],
			error: resource.error,
			connected: resource.status === "ok",
		}));
	const connectedMCPServers = mcpServers.filter((server) => server.connected);

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
			: undefined;
	const roundedPercent =
		percentUsed === undefined ? undefined : Math.round(percentUsed);
	const contextSeverity =
		percentUsed !== undefined && percentUsed >= 95
			? "exceeded"
			: percentUsed !== undefined && percentUsed >= 85
				? "warning"
				: "normal";
	const contextToneClassName =
		contextSeverity === "exceeded"
			? "text-content-destructive"
			: contextSeverity === "warning"
				? "text-content-warning"
				: "text-content-secondary";
	const contextWarning =
		Boolean(usage?.context?.dirty) ||
		Boolean(usage?.context?.error) ||
		fileIssues.length > 0;
	const hasContext =
		files.length > 0 ||
		fileIssues.length > 0 ||
		percentUsed !== undefined ||
		Boolean(usage?.context?.dirty) ||
		Boolean(usage?.context?.error);
	const hasSkills = skills.length > 0 || skillIssues.length > 0;
	const hasMCP =
		mcpConfigs.length > 0 || mcpServers.length > 0 || mcpIssues.length > 0;

	return (
		<div className="-mx-4 mt-4 divide-y divide-border-default border-y border-border-default">
			<SummarySection
				label="Context"
				disabled={!hasContext}
				warning={contextWarning}
				meta={
					<span className="flex items-center gap-3">
						{percentUsed !== undefined && (
							<>
								<UsageBar
									percent={percentUsed}
									severity={contextSeverity}
									ariaLabel={`${roundedPercent}% of context used`}
									className="hidden w-28 sm:block"
								/>
								<span className={contextToneClassName}>{roundedPercent}%</span>
							</>
						)}
						<span>
							{files.length} {files.length === 1 ? "file" : "files"}
						</span>
					</span>
				}
			>
				<div className="flex flex-col gap-4">
					{percentUsed !== undefined && (
						<div className="flex flex-col gap-2">
							<div className="flex items-center gap-3">
								<UsageBar
									percent={percentUsed}
									severity={contextSeverity}
									ariaLabel={`${roundedPercent}% of context used`}
									className="flex-1"
								/>
								<span className={cn("text-sm", contextToneClassName)}>
									{roundedPercent}%
								</span>
								<span className="text-sm text-content-secondary">
									({formatTokenCountCompact(usedTokens)} /{" "}
									{formatTokenCountCompact(contextLimitTokens)})
								</span>
							</div>
							{usage?.compressionThreshold !== undefined &&
								usage.compressionThreshold > 0 && (
									<span className="text-xs text-content-secondary">
										Compacts at {usage.compressionThreshold}%
									</span>
								)}
							{usage?.estimated && (
								<span className="text-xs text-content-secondary">
									Estimated until the next response reports measured usage.
								</span>
							)}
						</div>
					)}
					{files.length > 0 && (
						<DirectoryList groups={groupByDirectory(files)} />
					)}
					<IssueList issues={fileIssues} />
					{(usage?.context?.dirty || usage?.context?.error) && (
						<div className="flex flex-col gap-2 rounded-md border border-border-warning bg-surface-orange p-3 text-xs">
							<div className="font-medium text-content-warning">
								{usage.context.error ? "Context error" : "Context changed"}
							</div>
							<div className="text-content-secondary">
								{usage.context.error ||
									"The workspace context changed since this chat was pinned."}
							</div>
							{onRefreshContext && (
								<Button
									size="xs"
									className="self-start"
									disabled={isRefreshingContext}
									onClick={onRefreshContext}
								>
									<Spinner size="sm" loading={isRefreshingContext} />
									Refresh context
								</Button>
							)}
						</div>
					)}
				</div>
			</SummarySection>

			<SummarySection
				label="Skills"
				disabled={!hasSkills}
				warning={skillIssues.length > 0}
				meta={`${skills.length} available`}
			>
				<div className="flex flex-col gap-4">
					{skills.length > 0 && (
						<DirectoryList groups={groupByDirectory(skills)} />
					)}
					<IssueList issues={skillIssues} />
				</div>
			</SummarySection>

			<SummarySection
				label="MCP servers"
				disabled={!hasMCP}
				warning={
					mcpIssues.length > 0 || connectedMCPServers.length < mcpServers.length
				}
				meta={`${connectedMCPServers.length}${mcpServers.length > 0 ? ` of ${mcpServers.length}` : ""} connected`}
			>
				<div className="flex flex-col gap-4">
					{mcpConfigs.map((config) => (
						<div
							key={config.source}
							className="flex items-center gap-2 text-sm text-content-secondary"
						>
							<FileIcon className="size-4 shrink-0" />
							<span className="truncate" title={config.source}>
								{config.source}
							</span>
						</div>
					))}
					{mcpServers.map((server) => (
						<div key={server.source} className="flex flex-col gap-2">
							<div className="flex items-center gap-2 text-sm">
								<PlugIcon
									className={cn(
										"size-4 shrink-0",
										server.connected
											? "text-content-secondary"
											: "text-content-warning",
									)}
								/>
								<span
									className={
										server.connected
											? "text-content-primary"
											: "text-content-warning"
									}
								>
									{server.name}
								</span>
								<span className="ml-auto text-content-secondary">
									{server.connected ? "Connected" : "Unreachable"}
								</span>
							</div>
							{server.error && (
								<div className="ml-6 rounded-md border border-border-warning bg-surface-orange p-3 text-xs text-content-secondary">
									{server.error}
								</div>
							)}
							{server.tools.length > 0 && (
								<div className="ml-6 flex flex-col gap-2">
									{server.tools.map((tool) => (
										<div
											key={tool.name}
											className="flex items-center gap-2 text-sm text-content-secondary"
										>
											<WrenchIcon className="size-4 shrink-0" />
											<span>{tool.name}</span>
										</div>
									))}
								</div>
							)}
						</div>
					))}
					<IssueList
						issues={mcpIssues.filter((issue) => issue.kind === "mcp_config")}
					/>
				</div>
			</SummarySection>
		</div>
	);
};
