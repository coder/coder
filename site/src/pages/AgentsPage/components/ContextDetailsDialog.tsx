import {
	ChevronDownIcon,
	FileIcon,
	FolderIcon,
	PlugIcon,
	TriangleAlertIcon,
	WrenchIcon,
	ZapIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { formatKiB } from "#/utils/fileSize";
import { getPathBasename } from "../utils/path";
import {
	type AgentContextUsage,
	countLabel,
	formatContextUsageLine,
	getCompressionThresholdPercent,
	normalizeContextResources,
	RESOURCE_KIND_LABELS,
	RESOURCE_STATUS_LABELS,
} from "./contextResources";

// Warning (drifted pin) or error (failed snapshot) notice with the refresh
// affordance. Shown in the compact popover footer and mirrored in the details
// dialog footer.
export const ContextSyncStatus: FC<{
	contextError: string;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ contextError, onRefreshContext, isRefreshingContext }) => {
	const hasContextError = contextError !== "";
	return (
		<div className="flex flex-col gap-1.5">
			<span
				className={cn(
					"flex items-center gap-1.5 font-medium",
					hasContextError ? "text-content-destructive" : "text-content-warning",
				)}
			>
				<TriangleAlertIcon className="size-3 shrink-0" />
				{hasContextError ? "Context error" : "Context changed"}
			</span>
			<span className="text-content-secondary">
				{hasContextError
					? contextError
					: "The workspace context changed, so this chat's context files may be outdated."}
			</span>
			{onRefreshContext && (
				<div className="flex flex-wrap gap-2">
					<Button
						size="sm"
						disabled={isRefreshingContext}
						onClick={onRefreshContext}
					>
						<Spinner loading={isRefreshingContext} />
						Refresh context
					</Button>
				</div>
			)}
		</div>
	);
};

const SectionSize: FC<{ bytes: number }> = ({ bytes }) =>
	bytes > 0 ? (
		<span className="ml-1 font-normal text-content-secondary">
			{`(${formatKiB(bytes)})`}
		</span>
	) : null;

const TreeBranch: FC<{
	label: ReactNode;
	icon?: ReactNode;
	title?: string;
	className?: string;
	children: ReactNode;
}> = ({ label, icon, title, className, children }) => (
	<Collapsible defaultOpen>
		<CollapsibleTrigger asChild>
			<button
				type="button"
				title={title}
				className={cn(
					"group flex w-full min-w-0 cursor-pointer items-center gap-1.5 rounded border-0 bg-transparent px-0.5 py-px text-left text-xs text-content-primary transition-colors hover:bg-surface-tertiary",
					className,
				)}
			>
				<ChevronDownIcon className="size-3 shrink-0 transition-transform group-data-[state=closed]:-rotate-90" />
				{icon}
				<span className="min-w-0 flex-1 truncate">{label}</span>
			</button>
		</CollapsibleTrigger>
		<CollapsibleContent className="flex flex-col gap-0.5 pl-4 pt-0.5">
			{children}
		</CollapsibleContent>
	</Collapsible>
);

const TreeLeaf: FC<{
	icon: ReactNode;
	label: string;
	title?: string;
	description?: string;
}> = ({ icon, label, title, description }) => {
	const row = (
		<div
			title={title}
			className="flex items-center gap-1.5 rounded px-0.5 py-px text-xs text-content-secondary transition-colors hover:bg-surface-tertiary"
		>
			{icon}
			<span className="truncate">{label}</span>
		</div>
	);
	if (!description) {
		return row;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div className="cursor-default">{row}</div>
			</TooltipTrigger>
			<TooltipContent side="right" sideOffset={4} className="max-w-48 text-xs">
				{description}
			</TooltipContent>
		</Tooltip>
	);
};

// Render a section's directory groups: items without a parent directory
// render as top-level leaves, while grouped items nest under a collapsible
// directory branch so resources pulled from different roots (for example a
// repo-root AGENTS.md and a nested one) stay distinguishable.
const renderDirectoryGroups = <T,>(
	groups: readonly { readonly dir: string; readonly items: readonly T[] }[],
	renderItem: (item: T) => ReactNode,
): ReactNode =>
	groups.map((group) =>
		group.dir === "" ? (
			group.items.map((item) => renderItem(item))
		) : (
			<TreeBranch
				key={group.dir}
				className="text-content-secondary"
				icon={<FolderIcon className="size-3 shrink-0" />}
				label={group.dir}
				title={group.dir}
			>
				{group.items.map((item) => renderItem(item))}
			</TreeBranch>
		),
	);

export const ContextDetailsDialog: FC<{
	usage: AgentContextUsage;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}> = ({ usage, open, onOpenChange, onRefreshContext, isRefreshingContext }) => {
	const context = usage.context;
	const isDirty = context?.dirty ?? false;
	const contextError = context?.error ?? "";
	const {
		fileGroups,
		fileBytes,
		skillGroups,
		skillBytes,
		mcpConfigItems,
		mcpServerItems,
		mcpBytes,
		issueItems,
		hasResources,
	} = normalizeContextResources(context?.resources);
	const compressionThreshold = getCompressionThresholdPercent(usage);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="gap-4 p-6">
				<DialogHeader className="space-y-1">
					<DialogTitle>Context details</DialogTitle>
					<DialogDescription>
						{formatContextUsageLine(usage)}
						{compressionThreshold !== null &&
							` · Compacts at ${compressionThreshold}%`}
					</DialogDescription>
				</DialogHeader>
				<div className="max-h-[60vh] overflow-y-auto">
					{hasResources ? (
						<TooltipProvider delayDuration={300}>
							<div className="flex flex-col gap-1">
								{fileGroups.length > 0 && (
									<TreeBranch
										className="font-medium"
										label={
											<>
												Context files
												<SectionSize bytes={fileBytes} />
											</>
										}
									>
										{renderDirectoryGroups(fileGroups, (file) => (
											<TreeLeaf
												key={file.path}
												icon={<FileIcon className="size-3 shrink-0" />}
												label={getPathBasename(file.path)}
												title={file.path}
											/>
										))}
									</TreeBranch>
								)}
								{skillGroups.length > 0 && (
									<TreeBranch
										className="font-medium"
										label={
											<>
												Skills
												<SectionSize bytes={skillBytes} />
											</>
										}
									>
										{renderDirectoryGroups(skillGroups, (skill) => (
											<TreeLeaf
												key={skill.source}
												icon={<ZapIcon className="size-3 shrink-0" />}
												label={skill.name}
												title={skill.source}
												description={skill.description}
											/>
										))}
									</TreeBranch>
								)}
								{(mcpConfigItems.length > 0 || mcpServerItems.length > 0) && (
									<TreeBranch
										className="font-medium"
										label={
											<>
												MCP
												<SectionSize bytes={mcpBytes} />
											</>
										}
									>
										{mcpConfigItems.map((config) => (
											<TreeLeaf
												key={config.source}
												icon={<FileIcon className="size-3 shrink-0" />}
												label={config.source}
												title={config.source}
											/>
										))}
										{mcpServerItems.map((server) =>
											server.tools.length === 0 ? (
												<TreeLeaf
													key={server.source}
													icon={<PlugIcon className="size-3 shrink-0" />}
													label={server.name}
													title={server.source}
												/>
											) : (
												<TreeBranch
													key={server.source}
													className="text-content-secondary"
													icon={<PlugIcon className="size-3 shrink-0" />}
													title={server.source}
													label={
														<>
															{server.name}
															<span className="ml-1 text-content-secondary">
																({countLabel(server.tools.length, "tool")})
															</span>
														</>
													}
												>
													{server.tools.map((tool) => (
														<TreeLeaf
															key={tool.name}
															icon={<WrenchIcon className="size-3 shrink-0" />}
															label={tool.name}
															description={tool.description}
														/>
													))}
												</TreeBranch>
											),
										)}
									</TreeBranch>
								)}
								{issueItems.length > 0 && (
									<TreeBranch
										className="font-medium text-content-warning"
										icon={<TriangleAlertIcon className="size-3 shrink-0" />}
										label="Issues"
									>
										{issueItems.map((issue) => (
											<div
												key={issue.source}
												className="flex flex-col px-0.5 py-px text-xs"
												title={issue.source}
											>
												<span className="truncate text-content-primary">
													{`${issue.name} `}
													<span className="text-content-secondary">
														{`(${RESOURCE_KIND_LABELS[issue.kind]}: ${RESOURCE_STATUS_LABELS[issue.status]})`}
													</span>
												</span>
												{issue.error && (
													<span className="text-content-secondary">
														{issue.error}
													</span>
												)}
											</div>
										))}
									</TreeBranch>
								)}
							</div>
						</TooltipProvider>
					) : (
						<p className="m-0 text-xs text-content-secondary">
							No context resources.
						</p>
					)}
				</div>
				{(isDirty || contextError !== "") && (
					<div className="border-0 border-t border-solid border-border-default pt-3 text-xs">
						<ContextSyncStatus
							contextError={contextError}
							onRefreshContext={onRefreshContext}
							isRefreshingContext={isRefreshingContext}
						/>
					</div>
				)}
			</DialogContent>
		</Dialog>
	);
};
