import { cn } from "cn";
import { EllipsisVerticalIcon } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, useLocation } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	archiveChat,
	chatArchiveMutationKey,
	chatCost,
	pinChat,
	unarchiveChat,
	unpinChat,
} from "#/api/queries/chats";
import type { Chat, User } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { toast } from "#/components/Toaster/toast";
import { formatCostMicros } from "#/utils/currency";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { buildAgentChatPath } from "../../utils/navigation";
import { clearPersistedRightPanelState } from "../../utils/rightPanelTabStorage";
import { clearPersistedSidebarTabId } from "../../utils/sidebarTabStorage";
import {
	ChatActionsMenu,
	canManageChat,
	chatFamilyAllowsArchive,
} from "../ChatActionsMenuItems";
import { getChatCostTreeID } from "../ChatConversation/chatHelpers";
import { ChatPRStateIcon } from "../ChatDiffStats";
import { normalizeLocationSearch } from "../ChatsSidebar/locationSearch";
import { getChatDisplayConfig } from "../ChatsSidebar/tree/statusConfig";

/** The chat actions a project chat row offers from its menu. */
export type ProjectChatRowActions = Pick<
	AgentsPageOutletContext,
	"clearChatErrorReason" | "navigateAfterArchive" | "onOpenRenameDialog"
>;

type ProjectChatRowProps = {
	readonly chat: Chat;
	readonly currentUser: User;
	/** Omit to hide the actions menu. */
	readonly actions?: ProjectChatRowActions;
	readonly showCost: boolean;
};

const ProjectChatCost: React.FC<{ readonly chat: Chat }> = ({ chat }) => {
	const costQuery = useQuery(chatCost(getChatCostTreeID(chat) ?? chat.id));
	if (costQuery.isLoading) {
		return <Skeleton variant="text" className="w-10 shrink-0" />;
	}
	if (!costQuery.data) {
		return null;
	}
	return (
		<Badge
			asChild
			size="sm"
			className="shrink-0 tabular-nums group-hover:border-surface-tertiary group-hover:bg-surface-tertiary group-has-[[data-state=open]]:border-surface-tertiary group-has-[[data-state=open]]:bg-surface-tertiary"
		>
			<span>{formatCostMicros(costQuery.data.total_cost_micros)}</span>
		</Badge>
	);
};

export const ProjectChatRow: React.FC<ProjectChatRowProps> = ({
	chat,
	currentUser,
	actions,
	showCost,
}) => {
	const location = useLocation();
	const {
		icon: StatusIcon,
		className: statusClassName,
		label: statusLabel,
	} = getChatDisplayConfig(chat);
	const canManage = canManageChat(chat, currentUser.id);
	const isOwnChat = chat.owner_id === currentUser.id;
	const ownerName = isOwnChat
		? currentUser.name || currentUser.username
		: chat.owner_name || chat.owner_username || "Unknown";
	const workspaceId = chat.workspace_id;
	const queryClient = useQueryClient();
	const mutationKey = chatArchiveMutationKey(chat.id);
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
			actions?.clearChatErrorReason(chatId);
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

	return (
		<li className="group -mx-2 flex min-w-0 items-center gap-2 rounded-lg pr-2 hover:bg-surface-secondary has-[[data-state=open]]:bg-surface-secondary">
			<Link
				// Keeps the sidebar filters, which live in the query string.
				to={{
					pathname: buildAgentChatPath({ chatId: chat.id }),
					search: normalizeLocationSearch(location.search),
				}}
				className="flex min-w-0 flex-1 items-center gap-3 py-2.5 pl-2 text-sm text-content-primary no-underline"
			>
				<StatusIcon
					role="img"
					aria-label={statusLabel}
					className={cn("size-3.5 shrink-0", statusClassName)}
				/>
				<span className="min-w-0 max-w-104 flex-1 truncate">{chat.title}</span>
				<span className="ml-auto flex shrink-0 items-center gap-3">
					{showCost && <ProjectChatCost chat={chat} />}
					<ChatPRStateIcon chat={chat} />
				</span>
			</Link>
			<div className="grid size-7 shrink-0 place-items-center *:[grid-area:1/1]">
				<Avatar
					role="img"
					aria-label={`Owned by ${isOwnChat ? "you" : ownerName}`}
					size="sm"
					src={isOwnChat ? currentUser.avatar_url : undefined}
					fallback={ownerName}
					className={cn(
						actions &&
							"[@media(hover:hover)]:group-hover:invisible group-has-focus-visible:invisible group-has-[[data-state=open]]:invisible [@media(hover:none)]:invisible",
					)}
				/>
				{actions && (
					<ChatActionsMenu
						chat={chat}
						canManage={canManage}
						hasWorkspace={Boolean(workspaceId)}
						isArchiveBlocked={
							!chatFamilyAllowsArchive(chat.status, chat.children)
						}
						onPinAgent={() => pinMutation.mutate(chat.id)}
						onUnpinAgent={() => unpinMutation.mutate(chat.id)}
						onArchiveAgent={() => archiveMutation.mutate(chat.id)}
						onUnarchiveAgent={() => unarchiveMutation.mutate(chat.id)}
						onArchived={actions.navigateAfterArchive}
						onOpenRenameDialog={
							actions.onOpenRenameDialog
								? () => actions.onOpenRenameDialog?.(chat)
								: undefined
						}
					>
						<Button
							variant="subtle"
							size="icon"
							className="size-7 opacity-0 [@media(hover:hover)]:group-hover:opacity-100 group-has-focus-visible:opacity-100 data-[state=open]:opacity-100 [@media(hover:none)]:opacity-100"
							aria-label={`Open chat actions for ${chat.title}`}
						>
							<EllipsisVerticalIcon />
						</Button>
					</ChatActionsMenu>
				)}
			</div>
		</li>
	);
};
