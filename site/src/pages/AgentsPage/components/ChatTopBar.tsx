import {
	ArrowLeftIcon,
	ChevronRightIcon,
	EllipsisVerticalIcon,
	PanelLeftIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
	Share2Icon,
	UsersIcon,
} from "lucide-react";
import { type FC, Fragment, type ReactNode, useState } from "react";
import { Link, useLocation } from "react-router";
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
import { Popover, PopoverTrigger } from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";
import { parsePullRequestUrl } from "../utils/pullRequest";
import { ChatActionsMenuItems } from "./ChatActionsMenuItems";
import { useEmbedContext } from "./EmbedContext";
import { PrStateIcon } from "./GitPanel/GitPanel";

interface SidebarPanelState {
	showSidebarPanel: boolean;
	onToggleSidebar: () => void;
}

type ChatSharingTopBarButtonProps = {
	renderChatSharingContent: (open: boolean) => ReactNode;
};

type ChatTopBarProps = {
	chatTitle?: string;
	parentChat?: TypesGen.Chat;
	panel: SidebarPanelState;
	onArchiveAgent: () => void;
	onUnarchiveAgent: () => void;
	onArchiveAndDeleteWorkspace: () => void;
	onPinAgent?: () => void;
	onUnpinAgent?: () => void;
	onOpenRenameDialog?: () => void;
	hasWorkspace?: boolean;
	isArchived?: boolean;
	isArchiving?: boolean;
	isChildChat?: boolean;
	isPinned?: boolean;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	diffStatusData?: ChatDiffStatus;
	isSharedChat?: boolean;
	renderChatSharingContent?: (open: boolean) => ReactNode;
};

const ChatSharingTopBarButton: FC<ChatSharingTopBarButtonProps> = ({
	renderChatSharingContent,
}) => {
	const [isChatSharingOpen, setIsChatSharingOpen] = useState(false);
	const [contentGeneration, setContentGeneration] = useState(0);

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen) {
			setContentGeneration((generation) => generation + 1);
		}

		setIsChatSharingOpen(nextOpen);
	};

	return (
		<Popover open={isChatSharingOpen} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					className="size-7 text-content-secondary hover:text-content-primary"
					aria-label="Share chat"
				>
					<Share2Icon className="size-4" />
				</Button>
			</PopoverTrigger>
			<Fragment key={contentGeneration}>
				{renderChatSharingContent(isChatSharingOpen)}
			</Fragment>
		</Popover>
	);
};

export const ChatTopBar: FC<ChatTopBarProps> = ({
	chatTitle,
	parentChat,
	panel,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	onPinAgent,
	onUnpinAgent,
	onOpenRenameDialog,
	hasWorkspace = false,
	isArchived = false,
	isArchiving = false,
	isChildChat = false,
	isPinned = false,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	diffStatusData,
	isSharedChat,
	renderChatSharingContent,
}) => {
	const { isEmbedded } = useEmbedContext();
	const location = useLocation();

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
					className="inline-flex size-7 min-w-0 shrink-0 sm:hidden"
				>
					<Link
						to={{ pathname: "/agents", search: location.search }}
						aria-label="Back"
					>
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
					className="hidden size-7 min-w-0 shrink-0 sm:inline-flex"
				>
					<PanelLeftIcon />
				</Button>
			)}
			{/* Title area */}
			<div className="flex min-w-0 flex-1 items-center gap-1.5">
				{chatTitle && (
					<div
						role="status"
						aria-live="polite"
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
									<Link
										to={{
											pathname: `/agents/${parentChat.id}`,
											search: location.search,
										}}
									>
										<span className="truncate">{parentChat.title}</span>
									</Link>
								</Button>
								<ChevronRightIcon className="size-3.5 shrink-0 text-content-secondary/70 -ml-0.5" />
							</>
						)}
						<span className="truncate text-sm text-content-primary">
							{chatTitle}
						</span>
						{isSharedChat && (
							<UsersIcon
								className="size-3.5 shrink-0 text-content-secondary"
								aria-label="Shared chat"
							/>
						)}
					</div>
				)}
				{/* Actions menu sits inline with the title so it tracks the title's right edge.
				   Suppressed when there is no chat to act on (loading and not-found views). */}
				{!isEmbedded && chatTitle && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								className="size-7 shrink-0 text-content-secondary hover:text-content-primary"
								aria-label="Open agent actions"
							>
								<EllipsisVerticalIcon className="size-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent
							align="start"
							className="mobile-full-width-dropdown mobile-full-width-dropdown-top [&_[role=menuitem]]:text-[13px]"
						>
							<ChatActionsMenuItems
								isArchived={isArchived}
								isPinned={isPinned}
								isChildChat={isChildChat}
								hasWorkspace={hasWorkspace}
								isArchiving={isArchiving}
								onPinAgent={onPinAgent}
								onUnpinAgent={onUnpinAgent}
								onArchiveAgent={onArchiveAgent}
								onUnarchiveAgent={onUnarchiveAgent}
								onArchiveAndDeleteWorkspace={onArchiveAndDeleteWorkspace}
								onOpenRenameDialog={onOpenRenameDialog}
								Item={DropdownMenuItem}
								Separator={DropdownMenuSeparator}
							/>
						</DropdownMenuContent>
					</DropdownMenu>
				)}
			</div>
			{/* PR link. On mobile: icon + number; on desktop: icon + title.
			   Hidden on desktop when the sidebar panel is open
			   (which already shows PR info). */}
			{prUrl && hasPR && (
				<a
					href={prUrl}
					target="_blank"
					rel="noreferrer"
					className={cn(
						"inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
						panel.showSidebarPanel && "lg:hidden",
					)}
				>
					<PrStateIcon
						state={prState}
						draft={prDraft}
						className="!size-3.5 shrink-0"
					/>
					<span className="truncate max-w-[120px] hidden sm:inline">
						{prTitle || (prNumberMatch ? `#${prNumberMatch}` : "PR")}
					</span>
					<span className="sm:hidden">
						{prNumberMatch ? prNumberMatch : "PR"}
					</span>
				</a>
			)}
			{/* Actions area */}
			<div className="flex items-center gap-2">
				{!isEmbedded && renderChatSharingContent && (
					<ChatSharingTopBarButton
						renderChatSharingContent={renderChatSharingContent}
					/>
				)}
				{!isEmbedded && (
					<Button
						variant="subtle"
						size="icon"
						onClick={panel.onToggleSidebar}
						className="size-7 text-content-secondary hover:text-content-primary"
						aria-label="Toggle panel"
					>
						{panel.showSidebarPanel ? (
							<PanelRightCloseIcon className="size-4" />
						) : (
							<PanelRightOpenIcon className="size-4" />
						)}
					</Button>
				)}
			</div>
		</div>
	);
};
