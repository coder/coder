import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	BotIcon,
	MailIcon,
	MailOpenIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import { useId } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";
import { getParentChatID } from "./ChatConversation/chatHelpers";
import { isActiveChatStatus } from "./ChatConversation/chatStore";

type ArchiveBlockedReason = "active" | "paused";

// Archiving cascades to the embedded children (depth capped at 1), so the
// backend refuses it while the chat or any child is active or paused.
// Active takes precedence when both apply. Other statuses pass; the server
// conflict response is the backstop.
export const getArchiveBlockedReason = (
	status: TypesGen.ChatStatus,
	children: readonly TypesGen.Chat[] | undefined,
): ArchiveBlockedReason | undefined => {
	const statuses = [status, ...(children ?? []).map((child) => child.status)];
	if (statuses.some(isActiveChatStatus)) {
		return "active";
	}
	if (statuses.some((s) => s === "paused")) {
		return "paused";
	}
	return undefined;
};

type ItemComponent = typeof DropdownMenuItem | typeof ContextMenuItem;
type SeparatorComponent =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

/**
 * Pin, rename, and archive write to the chat record itself, so from a shared
 * chat they would change the owner's sidebar and title. Sharing only grants
 * read access, and admins who would pass the server's update check should
 * not manage another user's chat from a shared view either, so ownership
 * rather than authorization decides who sees those actions.
 */
export const canManageChat = (
	chat: TypesGen.Chat,
	currentUserId: string,
): boolean => chat.owner_id === currentUserId;

type ChatMenuActionsOptions = {
	readonly canManage: boolean;
	/** Whether the menu offers the subagents toggle, the only viewer action. */
	readonly hasSubagentsToggle?: boolean;
};

/**
 * Archive state is root-only on the backend and cascades to children, so
 * child chats expose no archive or unarchive actions. An archived child chat
 * therefore has no menu actions at all, and a non-owner only has the
 * subagents toggle; call sites use this to hide the menu trigger instead of
 * rendering an empty menu.
 */
export const chatHasMenuActions = (
	chat: TypesGen.Chat,
	{ canManage, hasSubagentsToggle = false }: ChatMenuActionsOptions,
): boolean => {
	if (!canManage) {
		return hasSubagentsToggle;
	}
	const isArchivedChild = chat.archived && getParentChatID(chat) !== undefined;
	return !isArchivedChild;
};

type ChatActionsMenuItemsProps = {
	readonly chat: TypesGen.Chat;
	/** See {@link canManageChat}. When false, only the subagents toggle renders. */
	readonly canManage: boolean;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
	readonly archiveBlockedReason?: ArchiveBlockedReason;
	readonly subagentCount?: number;
	readonly isSubagentsExpanded?: boolean;
	readonly onToggleSubagents?: () => void;
	readonly onPinAgent?: () => void;
	readonly onUnpinAgent?: () => void;
	/** Omit either read handler to hide the read-state toggle. */
	readonly onMarkRead?: () => void;
	readonly onMarkUnread?: () => void;
	readonly onArchiveAgent: () => void;
	readonly onUnarchiveAgent: () => void;
	readonly onArchiveAndDeleteWorkspace: () => void;
	/** When omitted, the "Rename chat" item is hidden. */
	readonly onOpenRenameDialog?: () => void;
	readonly Item: ItemComponent;
	readonly Separator: SeparatorComponent;
};

export const ChatActionsMenuItems: React.FC<ChatActionsMenuItemsProps> = ({
	chat,
	canManage,
	hasWorkspace,
	isArchiving = false,
	archiveBlockedReason,
	subagentCount = 0,
	isSubagentsExpanded = false,
	onToggleSubagents,
	onPinAgent,
	onUnpinAgent,
	onMarkRead,
	onMarkUnread,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	onOpenRenameDialog,
	Item,
	Separator,
}) => {
	const isArchived = chat.archived;
	const isPinned = chat.pin_order > 0;
	const isChildChat = getParentChatID(chat) !== undefined;
	const showSubagentsToggle = Boolean(onToggleSubagents) && subagentCount > 0;
	const showReadToggle = Boolean(onMarkRead && onMarkUnread);
	const showPinAction =
		!isArchived && !isChildChat && Boolean(onPinAgent && onUnpinAgent);
	const showArchiveActions = !isArchived && !isChildChat;
	const archiveBlockedHintId = useId();
	const isArchiveBlocked = archiveBlockedReason !== undefined;
	const archiveBlockedDescribedBy = isArchiveBlocked
		? archiveBlockedHintId
		: undefined;

	const subagentToggle = showSubagentsToggle ? (
		<Item onSelect={onToggleSubagents}>
			<BotIcon className="size-3.5" />
			{isSubagentsExpanded
				? "Hide subagents"
				: `Show subagents (${subagentCount})`}
		</Item>
	) : null;

	const readToggle = showReadToggle ? (
		<Item onSelect={chat.has_unread ? onMarkRead : onMarkUnread}>
			{chat.has_unread ? (
				<>
					<MailOpenIcon className="size-3.5" />
					Mark as read
				</>
			) : (
				<>
					<MailIcon className="size-3.5" />
					Mark as unread
				</>
			)}
		</Item>
	) : null;

	if (!canManage) {
		return subagentToggle;
	}

	return (
		<>
			{showPinAction && (
				<Item onSelect={isPinned ? onUnpinAgent : onPinAgent}>
					{isPinned ? (
						<>
							<PinOffIcon className="size-3.5" />
							Unpin agent
						</>
					) : (
						<>
							<PinIcon className="size-3.5" />
							Pin agent
						</>
					)}
				</Item>
			)}
			{isArchived ? (
				!isChildChat && (
					<>
						<Item disabled={isArchiving} onSelect={onUnarchiveAgent}>
							<ArchiveRestoreIcon className="size-3.5" />
							Unarchive agent
						</Item>
						{subagentToggle}
						{readToggle}
					</>
				)
			) : (
				<>
					{onOpenRenameDialog && (
						<Item onSelect={onOpenRenameDialog}>
							<SquarePenIcon className="size-3.5" />
							Rename chat
						</Item>
					)}
					{subagentToggle}
					{readToggle}
					{showArchiveActions && (
						<>
							{(onOpenRenameDialog ||
								showPinAction ||
								showSubagentsToggle ||
								showReadToggle) && <Separator />}
							<Item
								className="text-content-destructive focus:text-content-destructive"
								aria-describedby={archiveBlockedDescribedBy}
								disabled={isArchiving || isArchiveBlocked}
								onSelect={onArchiveAgent}
							>
								<ArchiveIcon className="size-3.5" />
								Archive agent
							</Item>
							{hasWorkspace && (
								<Item
									className="text-content-destructive focus:text-content-destructive"
									aria-describedby={archiveBlockedDescribedBy}
									disabled={isArchiving || isArchiveBlocked}
									onSelect={onArchiveAndDeleteWorkspace}
								>
									<Trash2Icon className="size-3.5" />
									Archive & delete workspace
								</Item>
							)}
							{isArchiveBlocked && (
								<div
									id={archiveBlockedHintId}
									className="max-w-56 px-2 py-1.5 text-xs text-content-secondary"
								>
									{archiveBlockedReason === "paused"
										? "Finish editing the queued message first."
										: "Interrupt or wait for the agent to finish first."}
								</div>
							)}
						</>
					)}
				</>
			)}
		</>
	);
};
