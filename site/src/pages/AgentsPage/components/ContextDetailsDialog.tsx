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
	getCompactionThresholdPercent,
	normalizeContextResources,
	RESOURCE_KIND_LABELS,
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
	);
};

// Dimmed "(N.N KiB)" size suffix for a section header, omitted when the
// section has no measurable size.
const SectionSize: FC<{ bytes: number }> = ({ bytes }) =>
	bytes > 0 ? (
		<span className="ml-1 font-normal text-content-secondary">
			{`(${formatKiB(bytes)})`}
		</span>
	) : null;

// Collapsible branch of the details tree: a disclosure row with a rotating
// chevron whose children render indented one level below.
const TreeBranch: FC<{
	label: ReactNode;
	icon?: ReactNode;
	title?: string;
	// Extra classes for the disclosure row, e.g. section emphasis or tone.
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

// Leaf row of the details tree. When a description is present the row gains
// a tooltip to its right, matching the compact popover's previous behavior.
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

// Full listing of the chat's pinned context resources, opened from the
// compact context usage popover. Sections, directory groups, and MCP servers
// are collapsible and default to expanded.
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
		fileItems,
		fileGroups,
		fileBytes,
		skillItems,
		skillGroups,
		skillBytes,
		mcpConfigItems,
		mcpServerItems,
		mcpBytes,
		issueItems,
	} = normalizeContextResources(context?.resources);
	const compactionThreshold = getCompactionThresholdPercent(usage);
	const hasList =
		fileItems.length > 0 ||
		skillItems.length > 0 ||
		mcpConfigItems.length > 0 ||
		mcpServerItems.length > 0 ||
		issueItems.length > 0;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="gap-4 p-6">
				<DialogHeader className="space-y-1">
					<DialogTitle>Context details</DialogTitle>
					<DialogDescription>
						{formatContextUsageLine(usage)}
						{compactionThreshold !== null &&
							` · Compacts at ${compactionThreshold}%`}
					</DialogDescription>
				</DialogHeader>
				<div className="max-h-[60vh] overflow-y-auto">
					{hasList ? (
						<TooltipProvider delayDuration={300}>
							<div className="flex flex-col gap-1">
								{fileItems.length > 0 && (
									<TreeBranch
										className="font-medium"
										label={
											<>
												Context files
												<SectionSize bytes={fileBytes} />
											</>
										}
									>
										{fileGroups.map((group) =>
											group.dir === "" ? (
												group.items.map((file) => (
													<TreeLeaf
														key={file.path}
														icon={<FileIcon className="size-3 shrink-0" />}
														label={getPathBasename(file.path)}
														title={file.path}
													/>
												))
											) : (
												<TreeBranch
													key={group.dir}
													className="text-content-secondary"
													icon={<FolderIcon className="size-3 shrink-0" />}
													label={group.dir}
													title={group.dir}
												>
													{group.items.map((file) => (
														<TreeLeaf
															key={file.path}
															icon={<FileIcon className="size-3 shrink-0" />}
															label={getPathBasename(file.path)}
															title={file.path}
														/>
													))}
												</TreeBranch>
											),
										)}
									</TreeBranch>
								)}
								{skillItems.length > 0 && (
									<TreeBranch
										className="font-medium"
										label={
											<>
												Skills
												<SectionSize bytes={skillBytes} />
											</>
										}
									>
										{skillGroups.map((group) =>
											group.dir === "" ? (
												group.items.map((skill) => (
													<TreeLeaf
														key={skill.source}
														icon={<ZapIcon className="size-3 shrink-0" />}
														label={skill.name}
														title={skill.source}
														description={skill.description}
													/>
												))
											) : (
												<TreeBranch
													key={group.dir}
													className="text-content-secondary"
													icon={<FolderIcon className="size-3 shrink-0" />}
													label={group.dir}
													title={group.dir}
												>
													{group.items.map((skill) => (
														<TreeLeaf
															key={skill.source}
															icon={<ZapIcon className="size-3 shrink-0" />}
															label={skill.name}
															title={skill.source}
															description={skill.description}
														/>
													))}
												</TreeBranch>
											),
										)}
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
									</TreeBranch>
								)}
							</div>
						</TooltipProvider>
					) : (
						<p className="m-0 text-xs text-content-secondary">
							No context resources pinned.
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
