import { cn } from "cn";
import {
	ArrowLeftIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	EllipsisVerticalIcon,
	PanelLeftIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
	Share2Icon,
	UsersIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { Link, useLocation, useOutletContext } from "react-router";
import { checkAuthorization } from "#/api/queries/authCheck";
import { chat as chatById } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Popover, PopoverTrigger } from "#/components/Popover/Popover";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { parsePullRequestUrl } from "../utils/pullRequest";
import {
	ChatActionsMenuItems,
	chatFamilyAllowsArchive,
	chatHasMenuActions,
} from "./ChatActionsMenuItems";
import { getParentChatID } from "./ChatConversation/chatHelpers";
import { ChatSharingPopoverContent } from "./ChatSharingPopover";
import { useEmbedContext } from "./EmbedContext";
import { PrStateIcon } from "./GitPanel/GitPanel";

interface SidebarPanelState {
	showSidebarPanel: boolean;
	onToggleSidebar: () => void;
}

type ChatSharingTopBarButtonProps = {
	chatId: string;
	organizationId: string;
};

type ChatTopBarProps = {
	chat?: TypesGen.Chat;
	liveChatStatus?: TypesGen.ChatStatus | null;
	panel: SidebarPanelState;
};

const ChatSharingTopBarButton: FC<ChatSharingTopBarButtonProps> = ({
	chatId,
	organizationId,
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
			<ChatSharingPopoverContent
				key={contentGeneration}
				chatId={chatId}
				organizationId={organizationId}
				open={isChatSharingOpen}
			/>
		</Popover>
	);
};

export const ChatTopBar: FC<ChatTopBarProps> = ({
	chat,
	liveChatStatus,
	panel,
}) => {
	const { isEmbedded } = useEmbedContext();
	const location = useLocation();
	const parentChatID = getParentChatID(chat);
	const parentChatQuery = useQuery({
		...chatById(parentChatID ?? ""),
		enabled: Boolean(parentChatID),
	});
	const parentChat = parentChatQuery.data;
	const isRootChat = chat !== undefined && parentChatID === undefined;
	const chatAuthorizationChecks: TypesGen.AuthorizationRequest["checks"] = {};
	if (chat !== undefined && isRootChat) {
		chatAuthorizationChecks.canShareChat = {
			object: {
				resource_type: "chat",
				owner_id: chat.owner_id,
				organization_id: chat.organization_id,
			},
			action: "share",
		};
	}
	const chatAuthorizationQuery = useQuery({
		...checkAuthorization({ checks: chatAuthorizationChecks }),
		enabled: Object.keys(chatAuthorizationChecks).length > 0,
	});
	const canShareChat =
		isRootChat && Boolean(chatAuthorizationQuery.data?.canShareChat);
	const {
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestArchiveAndDeleteWorkspace,
		requestPinAgent,
		requestUnpinAgent,
		onOpenRenameDialog,
		isArchiving = false,
		archivingChatId,
		activeChatChildren,
	} = useOutletContext<AgentsPageOutletContext | undefined>() ?? {};

	const chatTitle = chat?.title;
	const isArchived = chat?.archived ?? false;
	const isSharedChat = chat?.shared;
	const hasWorkspace = Boolean(chat?.workspace_id);
	const isArchivingThisChat = Boolean(
		isArchiving &&
			chat &&
			(archivingChatId === undefined || archivingChatId === chat.id),
	);
	// The per-chat stream updates this before the global chat record catches up.
	const isArchiveBlocked = chat
		? !chatFamilyAllowsArchive(
				liveChatStatus ?? chat.status,
				activeChatChildren,
			)
		: false;
	const showPinAction = Boolean(requestPinAgent && requestUnpinAgent);

	// A chat tracks one status row per ref, ordered newest first.
	// Rows without an open PR have no URL, so only rows with a link
	// become chips. The first linked row is the primary.
	const prStatuses = (chat?.diff_statuses ?? []).filter((status) => status.url);
	const primaryStatus = prStatuses[0];
	const hasMultiplePRs = prStatuses.length > 1;

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
				   Suppressed when there is no chat to act on (loading and not-found views)
				   and when the chat has no menu actions (archived child chats). */}
				{!isEmbedded && chat && chatTitle && chatHasMenuActions(chat) && (
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
								chat={chat}
								hasWorkspace={hasWorkspace}
								isArchiving={isArchivingThisChat}
								isArchiveBlocked={isArchiveBlocked}
								onPinAgent={
									showPinAction && !isArchived
										? () => {
												requestPinAgent?.(chat.id);
											}
										: undefined
								}
								onUnpinAgent={
									showPinAction && !isArchived
										? () => {
												requestUnpinAgent?.(chat.id);
											}
										: undefined
								}
								onArchiveAgent={() => {
									if (isArchived) {
										return;
									}
									requestArchiveAgent?.(chat.id);
								}}
								onUnarchiveAgent={() => {
									if (!isArchived) {
										return;
									}
									requestUnarchiveAgent?.(chat.id);
								}}
								onArchiveAndDeleteWorkspace={() => {
									const workspaceId = chat.workspace_id;
									if (isArchived || !workspaceId) {
										return;
									}
									requestArchiveAndDeleteWorkspace?.(chat.id, workspaceId);
								}}
								onOpenRenameDialog={
									!isArchived && onOpenRenameDialog
										? () => onOpenRenameDialog(chat)
										: undefined
								}
								Item={DropdownMenuItem}
								Separator={DropdownMenuSeparator}
							/>
						</DropdownMenuContent>
					</DropdownMenu>
				)}
			</div>
			{/* PR link. On mobile: icon + number; on desktop: icon + title.
			   Hidden on desktop when the sidebar panel is open
			   (which already shows PR info).
			   One PR links directly. Several PRs open a menu with one
			   link per PR, and the trigger shows the primary. */}
			{hasMultiplePRs ? (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<button
							type="button"
							aria-label="View pull requests"
							className={cn(
								"inline-flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
								panel.showSidebarPanel && "lg:hidden",
							)}
						>
							<PrStateChip status={primaryStatus} />
							<ChevronDownIcon className="size-3 shrink-0 opacity-70" />
						</button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="start" className="min-w-[240px] p-1">
						{prStatuses.map((status) => {
							const parsed = parsePullRequestUrl(status.url);
							const number = status.pr_number?.toString() ?? parsed?.number;
							return (
								<DropdownMenuItem
									key={`${status.remote_origin}/${status.git_branch}`}
									onSelect={() => {
										if (status.url) {
											window.open(status.url, "_blank", "noreferrer");
										}
									}}
									className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-xs"
								>
									<PrStateIcon
										state={status.pull_request_state}
										draft={status.pull_request_draft}
										className="size-3.5! shrink-0"
									/>
									<span className="truncate">
										{status.pull_request_title ||
											(number ? `#${number}` : "PR")}
									</span>
								</DropdownMenuItem>
							);
						})}
					</DropdownMenuContent>
				</DropdownMenu>
			) : (
				prStatuses.length === 1 && (
					<PrLink
						status={prStatuses[0]}
						className={panel.showSidebarPanel ? "lg:hidden" : undefined}
					/>
				)
			)}
			{/* Actions area */}
			<div className="flex items-center gap-2">
				{!isEmbedded && canShareChat && chat && (
					<ChatSharingTopBarButton
						chatId={chat.id}
						organizationId={chat.organization_id}
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

type PrLinkProps = {
	status: TypesGen.ChatDiffStatus;
	className?: string;
};

// The PR chip in the top bar. On mobile it shows the number, on
// desktop the title.
const PrLink: FC<PrLinkProps> = ({ status, className }) => {
	const parsed = parsePullRequestUrl(status.url);
	const number = status.pr_number?.toString() ?? parsed?.number;

	return (
		<a
			href={status.url}
			target="_blank"
			rel="noreferrer"
			className={cn(
				"inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
				className,
			)}
		>
			<PrStateIcon
				state={status.pull_request_state}
				draft={status.pull_request_draft}
				className="size-3.5! shrink-0"
			/>
			<span className="truncate max-w-[120px] hidden sm:inline">
				{status.pull_request_title || (number ? `#${number}` : "PR")}
			</span>
			<span className="sm:hidden">{number ?? "PR"}</span>
		</a>
	);
};

type PrStateChipProps = {
	status?: TypesGen.ChatDiffStatus;
};

// The icon and label half of the PR chip. The multi-PR menu trigger
// reuses it without the link.
const PrStateChip: FC<PrStateChipProps> = ({ status }) => {
	const parsed = parsePullRequestUrl(status?.url);
	const number = status?.pr_number?.toString() ?? parsed?.number;

	return (
		<>
			<PrStateIcon
				state={status?.pull_request_state}
				draft={status?.pull_request_draft}
				className="size-3.5! shrink-0"
			/>
			<span className="truncate max-w-[120px] hidden sm:inline">
				{status?.pull_request_title || (number ? `#${number}` : "PR")}
			</span>
			<span className="sm:hidden">{number ?? "PR"}</span>
		</>
	);
};
