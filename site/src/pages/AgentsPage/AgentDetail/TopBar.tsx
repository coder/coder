import type { ChatDiffStatusResponse } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { useAuthenticated } from "hooks";
import {
	ArchiveIcon,
	ArrowLeftIcon,
	ChevronRightIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	MonitorIcon,
	PanelLeftIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
} from "lucide-react";
import { UserDropdown } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useNavigate } from "react-router";
import { WebPushButton } from "../WebPushButton";

interface DiffStatsBadgeProps {
	status: ChatDiffStatusResponse;
	isOpen: boolean;
	onToggle: () => void;
}

const DiffStatsBadge: FC<DiffStatsBadgeProps> = ({
	status,
	isOpen,
	onToggle,
}) => {
	const additions = status.additions ?? 0;
	const deletions = status.deletions ?? 0;

	return (
		<Button
			variant="subtle"
			onClick={onToggle}
			className="gap-3 px-2 py-1 text-content-secondary hover:text-content-primary"
		>
			<span className="font-mono text-sm font-semibold text-content-success">
				+{additions}
			</span>
			<span className="font-mono text-sm font-semibold text-content-destructive">
				−{deletions}
			</span>
			{isOpen ? (
				<PanelRightCloseIcon className="h-4 w-4" />
			) : (
				<PanelRightOpenIcon className="h-4 w-4" />
			)}
		</Button>
	);
};

interface DiffPanelState {
	hasDiffStatus: boolean;
	diffStatus: ChatDiffStatusResponse | undefined;
	showDiffPanel: boolean;
	onToggleFilesChanged: () => void;
}

interface WorkspaceActions {
	canOpenEditors: boolean;
	canOpenWorkspace: boolean;
	onOpenInEditor: (editor: "cursor" | "vscode") => void;
	onViewWorkspace: () => void;
}

type AgentDetailTopBarProps = {
	chatTitle?: string;
	parentChat?: TypesGen.Chat;
	onOpenParentChat: (chatId: string) => void;
	diff: DiffPanelState;
	workspace: WorkspaceActions;
	onArchiveAgent: () => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
};

export const AgentDetailTopBar: FC<AgentDetailTopBarProps> = ({
	chatTitle,
	parentChat,
	onOpenParentChat,
	diff,
	workspace,
	onArchiveAgent,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}) => {
	const navigate = useNavigate();
	const { user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();

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
					</div>
				)}
			</div>
			{/* Actions area */}
			<div className="flex items-center gap-2">
				{diff.hasDiffStatus && diff.diffStatus && (
					<DiffStatsBadge
						status={diff.diffStatus}
						isOpen={diff.showDiffPanel}
						onToggle={diff.onToggleFilesChanged}
					/>
				)}
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
							disabled={!workspace.canOpenWorkspace}
							onSelect={workspace.onViewWorkspace}
						>
							<MonitorIcon className="h-3.5 w-3.5" />
							View Workspace
						</DropdownMenuItem>
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onSelect={onArchiveAgent}
						>
							<ArchiveIcon className="h-3.5 w-3.5" />
							Archive Agent
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				<WebPushButton />
			</div>
			<div className="flex items-center [&_span]:!rounded-full [&_span]:!size-8 [&_span]:!text-xs">
				<UserDropdown
					user={user}
					buildInfo={buildInfo}
					supportLinks={
						appearance.support_links?.filter(
							(link) => link.location !== "navbar",
						) ?? []
					}
					onSignOut={signOut}
				/>
			</div>
		</div>
	);
};
