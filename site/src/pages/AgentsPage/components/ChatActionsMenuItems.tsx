import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	BotIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	MessageSquarePlusIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useId } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatTreeMaxDepth } from "#/api/typesGenerated";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";

// Backend chatstate permits archive only from W, E0, and E1. Unknown status
// stays fail-open so the server conflict response remains the backstop.
const chatStatusAllowsArchive = (
	status: TypesGen.ChatStatus | null | undefined,
): boolean =>
	status === undefined ||
	status === null ||
	status === "waiting" ||
	status === "error";

// Archive cascades atomically over the whole family, so the backend
// rejects it when any child is still active, not just the root. Children
// are embedded on chat records (depth capped at 1); a missing children
// array stays fail-open like an unknown status.
export const chatFamilyAllowsArchive = (
	status: TypesGen.ChatStatus | null | undefined,
	children: readonly TypesGen.Chat[] | null | undefined,
): boolean =>
	chatStatusAllowsArchive(status) &&
	(children ?? []).every((child) => chatStatusAllowsArchive(child.status));

type ItemComponent = typeof DropdownMenuItem | typeof ContextMenuItem;
type SeparatorComponent =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

/**
 * Archive state cascades from a parent to its subagents, so subagents expose
 * no archive or unarchive actions. An archived subagent therefore has no menu
 * actions at all; call sites use this to hide the menu trigger instead of
 * rendering an empty menu. A named child chat (kind "chat") keeps its actions
 * regardless of parent_chat_id.
 */
export const chatHasMenuActions = (chat: TypesGen.Chat): boolean => {
	const isArchivedSubagent = chat.archived && chat.kind === "subagent";
	return !isArchivedSubagent;
};

interface ChatActionsMenuItemsProps {
	readonly chat: TypesGen.Chat;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
	readonly isArchiveBlocked?: boolean;
	/** Unarchive is hidden while the parent chat is archived. */
	readonly isParentArchived?: boolean;
	/**
	 * Number of subagents known for the chat. Undefined means the count is
	 * unknown (tree rows carry none), in which case the toggle is offered
	 * without a count.
	 */
	readonly subagentCount?: number;
	readonly isSubagentsExpanded?: boolean;
	readonly onToggleSubagents?: () => void;
	/** Expand or collapse the node's named children (tree sidebar only). */
	readonly onToggleExpanded?: () => void;
	readonly isExpanded?: boolean;
	/** Offered on root and chat kinds when provided (tree sidebar only). */
	readonly onCreateChildChat?: () => void;
	readonly isChildChatDepthLimitReached?: boolean;
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
	isArchiveBlocked = false,
	isParentArchived = false,
	subagentCount,
	isSubagentsExpanded = false,
	onToggleSubagents,
	onToggleExpanded,
	isExpanded = false,
	onCreateChildChat,
	isChildChatDepthLimitReached = false,
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
	const isChildChat = chat.kind === "subagent";
	// The tree root cannot be pinned or archived on the server.
	const isRoot = chat.kind === "root";
	const showSubagentsToggle =
		Boolean(onToggleSubagents) &&
		(subagentCount === undefined || subagentCount > 0);
	const showCreateChildChat = Boolean(onCreateChildChat) && !isChildChat;
	const showPinAction =
		!isArchived &&
		!isChildChat &&
		!isRoot &&
		Boolean(onPinAgent && onUnpinAgent);
	const showArchiveActions = !isArchived && !isChildChat && !isRoot;
	const archiveBlockedHintId = useId();
	const archiveBlockedDescribedBy = isArchiveBlocked
		? archiveBlockedHintId
		: undefined;

	const subagentToggle = showSubagentsToggle ? (
		<Item onSelect={onToggleSubagents}>
			<BotIcon className="size-3.5" />
			{isSubagentsExpanded
				? "Hide subagents"
				: subagentCount === undefined
					? "Show subagents"
					: `Show subagents (${subagentCount})`}
		</Item>
	) : null;

	const expandToggle = onToggleExpanded ? (
		<Item onSelect={onToggleExpanded}>
			{isExpanded ? (
				<>
					<ChevronDownIcon className="size-3.5" />
					Collapse
				</>
			) : (
				<>
					<ChevronRightIcon className="size-3.5" />
					Expand
				</>
			)}
		</Item>
	) : null;

	// The item stays focusable at the depth limit so arrow navigation
	// reaches it and announces the reason; Radix skips disabled items.
	const createChildChat = showCreateChildChat ? (
		<Item
			aria-disabled={isChildChatDepthLimitReached || undefined}
			className={
				isChildChatDepthLimitReached
					? "flex-col items-start gap-0.5 text-content-disabled"
					: undefined
			}
			onSelect={(event) => {
				if (isChildChatDepthLimitReached) {
					event.preventDefault();
					return;
				}
				onCreateChildChat?.();
			}}
		>
			<span className="flex items-center gap-2">
				<MessageSquarePlusIcon className="size-3.5" />
				New chat here
			</span>
			{isChildChatDepthLimitReached && (
				<span className="pl-[22px] text-xs">
					Chat tree depth limit reached ({ChatTreeMaxDepth} levels)
				</span>
			)}
		</Item>
	) : null;

	return (
		<>
			{!isArchived && createChildChat}
			{expandToggle}
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
						{!isRoot && !isParentArchived && (
							<Item disabled={isArchiving} onSelect={onUnarchiveAgent}>
								<ArchiveRestoreIcon className="size-3.5" />
								Unarchive agent
							</Item>
						)}
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
							{(onOpenRenameDialog ||
								showPinAction ||
								showSubagentsToggle ||
								showCreateChildChat ||
								onToggleExpanded) && <Separator />}
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
									Interrupt or wait for the agent to finish first.
								</div>
							)}
						</>
					)}
				</>
			)}
		</>
	);
};
