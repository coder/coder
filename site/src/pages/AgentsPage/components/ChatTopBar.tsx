import { cn } from "cn";
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
import { useState } from "react";
import {
	useIsMutating,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { Link, useLocation, useOutletContext } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	archiveChat,
	chatArchiveMutationKey,
	chat as chatById,
	pinChat,
	unarchiveChat,
	unpinChat,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Popover, PopoverTrigger } from "#/components/Popover/Popover";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import type { AgentsPageOutletContext } from "../AgentsPageLayout";
import { parsePullRequestUrl } from "../utils/pullRequest";
import { clearPersistedRightPanelState } from "../utils/rightPanelTabStorage";
import { clearPersistedSidebarTabId } from "../utils/sidebarTabStorage";
import {
	ChatActionsMenu,
	canManageChat,
	chatFamilyAllowsArchive,
} from "./ChatActionsMenuItems";
import { getParentChatID } from "./ChatConversation/chatHelpers";
import { ChatSharingPopoverContent } from "./ChatSharingPopover";
import { useEmbedContext } from "./EmbedContext";
import { PrStateIcon } from "./GitPanel/GitPanel";

type SidebarPanelState = {
	showSidebarPanel: boolean;
	onToggleSidebar: () => void;
};

type ChatSharingTopBarButtonProps = {
	chatId: string;
	organizationId: string;
};

type ChatTopBarProps = {
	panelToggleRef?: React.RefObject<HTMLButtonElement | null>;
	chat?: TypesGen.Chat;
	liveChatStatus?: TypesGen.ChatStatus | null;
	panel: SidebarPanelState;
};

const ChatSharingTopBarButton: React.FC<ChatSharingTopBarButtonProps> = ({
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

export const ChatTopBar: React.FC<ChatTopBarProps> = ({
	chat,
	liveChatStatus,
	panel,
	panelToggleRef,
}) => {
	const { isEmbedded } = useEmbedContext();
	const { user: currentUser } = useAuthenticated();
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
		clearChatErrorReason,
		navigateAfterArchive,
		onOpenRenameDialog,
		activeChatChildren,
	} = useOutletContext<AgentsPageOutletContext | undefined>() ?? {};

	const queryClient = useQueryClient();
	const mutationKey = chatArchiveMutationKey(chat?.id ?? "");
	const pinOptions = pinChat(queryClient);
	const pinMutation = useMutation({
		...pinOptions,
		onError: (error, chatId, context) => {
			pinOptions.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to pin agent."));
		},
	});
	const unpinOptions = unpinChat(queryClient);
	const unpinMutation = useMutation({
		...unpinOptions,
		onError: (error, chatId, context) => {
			unpinOptions.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to unpin agent."));
		},
	});

	const archiveOptions = archiveChat(queryClient);
	const archiveMutation = useMutation({
		...archiveOptions,
		mutationKey,
		onSuccess: (data, chatId) => {
			archiveOptions.onSuccess(data, chatId);
			clearPersistedSidebarTabId(chatId);
			clearPersistedRightPanelState(chatId);
			clearChatErrorReason?.(chatId);
		},
		onError: (error, chatId, context) => {
			archiveOptions.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to archive agent."));
		},
	});
	const unarchiveOptions = unarchiveChat(queryClient);
	const unarchiveMutation = useMutation({
		...unarchiveOptions,
		mutationKey,
		onError: (error, chatId, context) => {
			unarchiveOptions.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to unarchive agent."));
		},
	});

	const chatTitle = chat?.title;
	const isArchived = chat?.archived ?? false;
	const isSharedChat = chat?.shared;
	const canManage = chat !== undefined && canManageChat(chat, currentUser.id);
	const hasWorkspace = Boolean(chat?.workspace_id);
	const isArchivingThisChat = useIsMutating({ mutationKey }) > 0;
	// The per-chat stream updates this before the global chat record catches up.
	const isArchiveBlocked = chat
		? !chatFamilyAllowsArchive(
				liveChatStatus ?? chat.status,
				activeChatChildren,
			)
		: false;
	// Suppressed when there is no chat to act on (loading and not-found views).
	const showActionsMenu =
		!isEmbedded && chat !== undefined && Boolean(chatTitle);
	const diffStatus = chat?.diff_status;

	const prUrl = diffStatus?.url;
	const prState = diffStatus?.pull_request_state;
	const prDraft = diffStatus?.pull_request_draft;
	const prTitle = diffStatus?.pull_request_title;
	const parsedPr = parsePullRequestUrl(prUrl);
	const prNumberMatch = diffStatus?.pr_number?.toString() ?? parsedPr?.number;
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
				{/* Actions menu sits inline with the title so it tracks the title's right edge. */}
				{chat && showActionsMenu && (
					<ChatActionsMenu
						key={chat.id}
						align="start"
						contentClassName="mobile-full-width-dropdown mobile-full-width-dropdown-top [&_[role=menuitem]]:text-[13px]"
						onArchived={navigateAfterArchive}
						chat={chat}
						canManage={canManage}
						hasWorkspace={hasWorkspace}
						isArchiving={isArchivingThisChat}
						isArchiveBlocked={isArchiveBlocked}
						onPinAgent={() => pinMutation.mutate(chat.id)}
						onUnpinAgent={() => unpinMutation.mutate(chat.id)}
						onArchiveAgent={() => {
							if (isArchived) {
								return;
							}
							archiveMutation.mutate(chat.id);
						}}
						onUnarchiveAgent={() => {
							if (!isArchived) {
								return;
							}
							unarchiveMutation.mutate(chat.id);
						}}
						onOpenRenameDialog={
							!isArchived && onOpenRenameDialog
								? () => onOpenRenameDialog(chat)
								: undefined
						}
					>
						<Button
							size="icon"
							variant="subtle"
							className="size-7 shrink-0 text-content-secondary hover:text-content-primary"
							aria-label="Open agent actions"
						>
							<EllipsisVerticalIcon className="size-4" />
						</Button>
					</ChatActionsMenu>
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
						"inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
						panel.showSidebarPanel && "lg:hidden",
					)}
				>
					<PrStateIcon
						state={prState}
						draft={prDraft}
						className="size-3.5! shrink-0"
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
						ref={panelToggleRef}
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
