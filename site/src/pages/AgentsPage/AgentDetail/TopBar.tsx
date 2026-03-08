import type { ChatDiffStatusResponse } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
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
} from "lucide-react";
import type { FC } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { DiffStatsInline } from "../DiffStats";

interface SidebarPanelState {
	hasDiffStatus: boolean;
	diffStatus: ChatDiffStatusResponse | undefined;
	hasGitRepos: boolean;
	gitRepoCount: number;
	gitRepositories: ReadonlyMap<string, TypesGen.WorkspaceAgentRepoChanges>;
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

interface AgentDetailTopBarProps {
	chatTitle?: string;
	parentChat?: TypesGen.Chat;
	onOpenParentChat: (chatId: string) => void;
	diff: SidebarPanelState;
	workspace: WorkspaceActions;
	onArchiveAgent: () => void;
	onUnarchiveAgent: () => void;
	onArchiveAndDeleteWorkspace: () => void;
	hasWorkspace?: boolean;
	isArchived?: boolean;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
}

export const AgentDetailTopBar: FC<AgentDetailTopBarProps> = ({
	chatTitle,
	parentChat,
	onOpenParentChat,
	diff,
	workspace,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	hasWorkspace,
	isArchived,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}) => {
	const navigate = useNavigate();

	return (
		<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
			{/* Mobile back button */}
			<Button
				variant="subtle"
				size="icon"
				onClick={() => navigate("/agents")}
				aria-label="Back"
				className="inline-flex h-7 w-7 min-w-0 shrink-0 md:hidden"
			>
				<ArrowLeftIcon />
			</Button>
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
					<div className="flex min-w-0 items-center gap-1.5">
						{parentChat && (
							<>
								<Button
									size="sm"
									variant="subtle"
									className="h-auto max-w-[16rem] rounded-sm px-1 py-0.5 text-xs text-content-secondary shadow-none hover:bg-transparent hover:text-content-primary"
									onClick={() => onOpenParentChat(parentChat.id)}
								>
									<span className="truncate">{parentChat.title}</span>
								</Button>
								<ChevronRightIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary/70" />
							</>
						)}
						<span className="truncate text-sm text-content-primary">
							{chatTitle}
						</span>
						{diff.hasDiffStatus &&
							diff.diffStatus &&
							!diff.showSidebarPanel && (
								<span className="ml-3">
									<DiffStatsInline
										status={diff.diffStatus}
										onClick={diff.onToggleSidebar}
									/>
								</span>
							)}
						{isArchived && (
							<span className="shrink-0 rounded bg-surface-tertiary px-1.5 py-0.5 text-xs text-content-secondary">
								Archived
							</span>
						)}
					</div>
				)}
			</div>
			{/* Actions area */}
			<div className="flex items-center gap-2">
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
					<DropdownMenuContent align="end">
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
						<DropdownMenuSeparator />
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
				{(diff.hasDiffStatus || diff.hasGitRepos) && (
					<Button
						variant="subtle"
						size="icon"
						onClick={diff.onToggleSidebar}
						className="h-7 w-7 text-content-secondary hover:text-content-primary"
						aria-label="Toggle files changed"
					>
						{diff.showSidebarPanel ? (
							<PanelRightCloseIcon className="h-4 w-4" />
						) : (
							<PanelRightOpenIcon className="h-4 w-4" />
						)}
					</Button>
				)}
			</div>{" "}
		</div>
	);
};
