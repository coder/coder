import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	BotIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import type { FC } from "react";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";

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
export const chatHasMenuActions = ({
	isArchived,
	isChildChat,
}: {
	isArchived: boolean;
	isChildChat: boolean;
}): boolean => !(isArchived && isChildChat);

interface ChatActionsMenuItemsProps {
	readonly isArchived: boolean;
	readonly isPinned: boolean;
	readonly isChildChat: boolean;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
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
	isArchived,
	isPinned,
	isChildChat,
	hasWorkspace,
	isArchiving = false,
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
	const showSubagentsToggle = Boolean(onToggleSubagents) && subagentCount > 0;
	const showPinAction =
		!isArchived && !isChildChat && Boolean(onPinAgent && onUnpinAgent);
	const showArchiveActions = !isArchived && !isChildChat;

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
								disabled={isArchiving}
								onSelect={onArchiveAgent}
							>
								<ArchiveIcon className="size-3.5" />
								Archive agent
							</Item>
							{hasWorkspace && (
								<Item
									className="text-content-destructive focus:text-content-destructive"
									disabled={isArchiving}
									onSelect={onArchiveAndDeleteWorkspace}
								>
									<Trash2Icon className="size-3.5" />
									Archive & delete workspace
								</Item>
							)}
						</>
					)}
				</>
			)}
		</>
	);
};
