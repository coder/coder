import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	ArrowLeftIcon,
	ChevronRightIcon,
	CopyIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	MonitorIcon,
	PanelLeftIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
	TerminalIcon,
	Trash2Icon,
	WandSparklesIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import {
	type DisplayWorkspaceStatusType,
	getDisplayWorkspaceStatus,
} from "#/utils/workspace";
import { parsePullRequestUrl } from "../utils/pullRequest";
import { useEmbedContext } from "./EmbedContext";
import { PrStateIcon } from "./GitPanel/GitPanel";

const statusVariantMap: Record<
	DisplayWorkspaceStatusType,
	StatusIndicatorProps["variant"]
> = {
	active: "pending",
	inactive: "inactive",
	success: "success",
	error: "failed",
	danger: "warning",
	warning: "warning",
};

interface SidebarPanelState {
	showSidebarPanel: boolean;
	onToggleSidebar: () => void;
}

interface WorkspaceActions {
	canOpenEditors: boolean;
	canOpenWorkspace: boolean;
	onOpenInEditor: (editor: "cursor" | "vscode") => void;
	onViewWorkspace: () => void;
	onOpenTerminal: () => void;
	sshCommand: string | undefined;
}

type ChatTopBarProps = {
	chatTitle?: string;
	parentChat?: TypesGen.Chat;
	panel: SidebarPanelState;
	workspace: WorkspaceActions;
	workspaceData?: TypesGen.Workspace;
	workspaceRoute?: string;
	onArchiveAgent: () => void;
	onUnarchiveAgent: () => void;
	onArchiveAndDeleteWorkspace: () => void;
	onRegenerateTitle?: () => void;
	isRegeneratingTitle?: boolean;
	isRegenerateTitleDisabled?: boolean;
	hasWorkspace?: boolean;
	isArchived?: boolean;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	diffStatusData?: ChatDiffStatus;
};

export const ChatTopBar: FC<ChatTopBarProps> = ({
	chatTitle,
	parentChat,
	panel,
	workspace,
	workspaceData,
	workspaceRoute,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	onRegenerateTitle,
	isRegeneratingTitle,
	isRegenerateTitleDisabled,
	hasWorkspace,
	isArchived,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	diffStatusData,
}) => {
	const { isEmbedded } = useEmbedContext();

	const prUrl = diffStatusData?.url;
	const prState = diffStatusData?.pull_request_state;
	const prDraft = diffStatusData?.pull_request_draft;
	const prTitle = diffStatusData?.pull_request_title;
	const parsedPr = parsePullRequestUrl(prUrl);
	const prNumberMatch =
		diffStatusData?.pr_number?.toString() ?? parsedPr?.number;
	const hasPR = Boolean(prState || prNumberMatch || parsedPr);

	return (
		<div className="flex shrink-0 items-center gap-2 px-4 py-1.5">
			{/* Mobile back button */}
			{!isEmbedded && (
				<Button
					asChild
					variant="subtle"
					size="icon"
					className="inline-flex h-7 w-7 min-w-0 shrink-0 md:hidden"
				>
					<Link to="/agents" aria-label="Back">
						<ArrowLeftIcon />
					</Link>
				</Button>
			)}
			{/* Desktop expand button: visible when sidebar is manually collapsed. */}
			{isSidebarCollapsed && (
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggleSidebarCollapsed}
					aria-label="Expand sidebar"
					className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
				>
					<PanelLeftIcon />
				</Button>
			)}
			{/* Title area */}
			<div className="flex min-w-0 flex-1 items-center">
				{chatTitle && (
					<div
						role="status"
						aria-live="polite"
						aria-busy={isRegeneratingTitle}
						className="flex min-w-0 items-center gap-1.5"
					>
						{parentChat && (
							<>
								<Button
									asChild
									size="sm"
									variant="subtle"
									className="h-auto max-w-[16rem] rounded-sm px-1 py-0.5 text-sm text-content-secondary shadow-none hover:bg-transparent hover:text-content-primary"
								>
									<Link to={`/agents/${parentChat.id}`}>
										<span className="truncate">{parentChat.title}</span>
									</Link>
								</Button>
								<ChevronRightIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary/70 -ml-0.5" />
							</>
						)}
						<span
							className={cn(
								"truncate text-sm text-content-primary",
								isRegeneratingTitle && "animate-pulse",
							)}
						>
							{chatTitle}
						</span>
						{isRegeneratingTitle && (
							<Spinner
								aria-label="Regenerating title"
								className="h-3.5 w-3.5 shrink-0 text-content-secondary"
								loading
							/>
						)}
					</div>
				)}
			</div>
			{/* Resource badges — workspace status and/or PR link.
				   On mobile the workspace name and PR title are hidden
				   so these collapse to compact icon-only pills, leaving
				   room for the chat title. */}
			{(workspaceData || hasPR) && (
				<div className="flex shrink-0 items-center gap-1.5">
					{workspaceData && workspaceRoute && (
						<WorkspaceBadge workspace={workspaceData} route={workspaceRoute} />
					)}
					{prUrl && hasPR && (
						<a
							href={prUrl}
							target="_blank"
							rel="noreferrer"
							className={cn(
								"inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
								panel.showSidebarPanel && "md:hidden",
							)}
						>
							<PrStateIcon
								state={prState}
								draft={prDraft}
								className="!size-3.5 shrink-0"
							/>
							<span className="truncate max-w-[120px] hidden md:inline">
								{prTitle || (prNumberMatch ? `#${prNumberMatch}` : "PR")}
							</span>
							<span className="md:hidden">
								{prNumberMatch ? prNumberMatch : "PR"}
							</span>
						</a>
					)}
				</div>
			)}
			{/* Actions area */}{" "}
			<div className="flex items-center gap-2">
				{!isEmbedded && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								className="h-7 w-7 text-content-secondary hover:text-content-primary"
								aria-label="Open agent actions"
							>
								<EllipsisIcon className="h-4 w-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent
							align="end"
							className="[&_[role=menuitem]]:text-[13px]"
						>
							<DropdownMenuItem
								disabled={!workspace.canOpenEditors}
								onSelect={() => {
									workspace.onOpenInEditor("cursor");
								}}
							>
								<ExternalLinkIcon className="h-3.5 w-3.5" />
								Open in Cursor
							</DropdownMenuItem>
							<DropdownMenuItem
								disabled={!workspace.canOpenEditors}
								onSelect={() => {
									workspace.onOpenInEditor("vscode");
								}}
							>
								<ExternalLinkIcon className="h-3.5 w-3.5" />
								Open in VS Code
							</DropdownMenuItem>
							<DropdownMenuItem
								// You can think of the web terminal as an editor if you squint.
								disabled={!workspace.canOpenEditors}
								onSelect={workspace.onOpenTerminal}
							>
								<TerminalIcon className="h-3.5 w-3.5" />
								Open Terminal
							</DropdownMenuItem>
							<DropdownMenuItem
								disabled={!workspace.sshCommand}
								onSelect={async () => {
									if (!workspace.sshCommand) return;
									try {
										await navigator.clipboard.writeText(workspace.sshCommand);
										toast.success("SSH command copied to clipboard");
									} catch {
										toast.error("Failed to copy SSH command");
									}
								}}
							>
								<CopyIcon className="h-3.5 w-3.5" />
								Copy SSH Command
							</DropdownMenuItem>
							<DropdownMenuSeparator />
							<DropdownMenuItem
								disabled={!workspace.canOpenWorkspace}
								onSelect={workspace.onViewWorkspace}
							>
								<MonitorIcon className="h-3.5 w-3.5" />
								View Workspace
							</DropdownMenuItem>
							{!isArchived && (
								<>
									<DropdownMenuSeparator />
									{onRegenerateTitle && (
										<>
											<DropdownMenuItem
												disabled={isRegenerateTitleDisabled}
												onSelect={onRegenerateTitle}
											>
												<WandSparklesIcon className="h-3.5 w-3.5" />
												Generate new title
											</DropdownMenuItem>
											<DropdownMenuSeparator />
										</>
									)}
								</>
							)}
							{isArchived ? (
								<DropdownMenuItem onSelect={onUnarchiveAgent}>
									<ArchiveRestoreIcon className="h-3.5 w-3.5" />
									Unarchive Agent
								</DropdownMenuItem>
							) : (
								<>
									<DropdownMenuItem
										className="text-content-destructive focus:text-content-destructive"
										onSelect={onArchiveAgent}
									>
										<ArchiveIcon className="h-3.5 w-3.5" />
										Archive Agent
									</DropdownMenuItem>
									{hasWorkspace && (
										<DropdownMenuItem
											className="text-content-destructive focus:text-content-destructive"
											onSelect={onArchiveAndDeleteWorkspace}
										>
											<Trash2Icon className="h-3.5 w-3.5" />
											Archive & Delete Workspace
										</DropdownMenuItem>
									)}
								</>
							)}
						</DropdownMenuContent>
					</DropdownMenu>
				)}
				{!isEmbedded && (
					<Button
						variant="subtle"
						size="icon"
						onClick={panel.onToggleSidebar}
						className="h-7 w-7 text-content-secondary hover:text-content-primary"
						aria-label="Toggle panel"
					>
						{panel.showSidebarPanel ? (
							<PanelRightCloseIcon className="h-4 w-4" />
						) : (
							<PanelRightOpenIcon className="h-4 w-4" />
						)}
					</Button>
				)}
			</div>
		</div>
	);
};

interface WorkspaceBadgeProps {
	workspace: TypesGen.Workspace;
	route: string;
}

const WorkspaceBadge: FC<WorkspaceBadgeProps> = ({ workspace, route }) => {
	const { text, type } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
	);
	const variant = statusVariantMap[workspace.health.healthy ? type : "warning"];

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Link
					to={route}
					target="_blank"
					className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary"
				>
					<MonitorIcon className="size-3.5 shrink-0" />
					<span className="hidden truncate max-w-[120px] md:inline">
						{workspace.name}
					</span>
					<StatusIndicatorDot variant={variant} size="sm" />
				</Link>
			</TooltipTrigger>
			<TooltipContent>
				{workspace.name} &mdash; {text}
			</TooltipContent>
		</Tooltip>
	);
};
