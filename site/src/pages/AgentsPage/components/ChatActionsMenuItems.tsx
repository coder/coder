import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	BotIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useId } from "react";
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

// Archive is refused for an active or paused chat and cascades over the
// family (children are embedded, depth capped at 1), so a child's status
// blocks it too. Paused is reported separately so the hint can name the
// queued edit. Other statuses pass; the server conflict response is the
// backstop.
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
 * Archive state is root-only on the backend and cascades to children, so
 * child chats expose no archive or unarchive actions. An archived child chat
 * therefore has no menu actions at all; call sites use this to hide the menu
 * trigger instead of rendering an empty menu.
 */
export const chatHasMenuActions = (chat: TypesGen.Chat): boolean => {
	const isArchivedChild = chat.archived && getParentChatID(chat) !== undefined;
	return !isArchivedChild;
};

interface ChatActionsMenuItemsProps {
	readonly chat: TypesGen.Chat;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
	readonly archiveBlockedReason?: ArchiveBlockedReason;
	readonly subagentCount?: number;
	readonly isSubagentsExpanded?: boolean;
	readonly onToggleSubagents?: () => void;
	readonly onPinAgent?: () => void;
	readonly onUnpinAgent?: () => void;
	readonly onArchiveAgent: () => void;
	readonly onUnarchiveAgent: () => void;
	readonly onArchiveAndDeleteWorkspace: () => void;
	/** When omitted, the "Rename chat" item is hidden. */
	readonly onOpenRenameDialog?: () => void;
	readonly Item: ItemComponent;
	readonly Separator: SeparatorComponent;
}

export const ChatActionsMenuItems: FC<ChatActionsMenuItemsProps> = ({
	chat,
	hasWorkspace,
	isArchiving = false,
	archiveBlockedReason,
	subagentCount = 0,
	isSubagentsExpanded = false,
	onToggleSubagents,
	onPinAgent,
	onUnpinAgent,
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
					{showArchiveActions && (
						<>
							{(onOpenRenameDialog || showPinAction || showSubagentsToggle) && (
								<Separator />
							)}
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
