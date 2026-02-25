import type { ChatDiffStatusResponse } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import {
	ArchiveIcon,
	ChevronRightIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	MonitorIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
} from "lucide-react";
import type { FC, RefObject } from "react";
import { createPortal } from "react-dom";
import { FilesChangedPanel } from "../FilesChangedPanel";

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
		<div
			role="button"
			tabIndex={0}
			onClick={onToggle}
			onKeyDown={(event) => {
				if (event.key === "Enter" || event.key === " ") {
					onToggle();
				}
			}}
			className="flex cursor-pointer items-center gap-3 px-2 py-1 text-content-secondary transition-colors hover:text-content-primary"
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
		</div>
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

type AgentDetailTopBarPortalsProps = {
	topBarTitleRef?: RefObject<HTMLDivElement | null>;
	topBarActionsRef?: RefObject<HTMLDivElement | null>;
	rightPanelRef?: RefObject<HTMLDivElement | null>;
	chatTitle?: string;
	parentChat?: TypesGen.Chat;
	onOpenParentChat: (chatId: string) => void;
	diff: DiffPanelState;
	workspace: WorkspaceActions;
	onArchiveAgent: () => void;
	shouldShowDiffPanel: boolean;
	agentId: string;
};

export const AgentDetailTopBarPortals: FC<AgentDetailTopBarPortalsProps> = ({
	topBarTitleRef,
	topBarActionsRef,
	rightPanelRef,
	chatTitle,
	parentChat,
	onOpenParentChat,
	diff,
	workspace,
	onArchiveAgent,
	shouldShowDiffPanel,
	agentId,
}) => {
	return (
		<>
			{chatTitle &&
				topBarTitleRef?.current &&
				createPortal(
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
					</div>,
					topBarTitleRef.current,
				)}
			{diff.hasDiffStatus &&
				diff.diffStatus &&
				topBarActionsRef?.current &&
				createPortal(
					<DiffStatsBadge
						status={diff.diffStatus}
						isOpen={diff.showDiffPanel}
						onToggle={diff.onToggleFilesChanged}
					/>,
					topBarActionsRef.current,
				)}
			{topBarActionsRef?.current &&
				createPortal(
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
					</DropdownMenu>,
					topBarActionsRef.current,
				)}
			{shouldShowDiffPanel &&
				rightPanelRef?.current &&
				createPortal(
					<FilesChangedPanel chatId={agentId} />,
					rightPanelRef.current,
				)}
		</>
	);
};
