import {
	ArchiveIcon,
	ArchiveRestoreIcon,
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

/**
 * Polymorphic item/separator components. Lets the same menu body render into
 * either a DropdownMenu (kebab-triggered) or a ContextMenu (right-click).
 */
type ItemComponent = typeof DropdownMenuItem | typeof ContextMenuItem;
type SeparatorComponent =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

interface ChatActionsMenuItemsProps {
	readonly isArchived: boolean;
	readonly isPinned: boolean;
	readonly isChildChat: boolean;
	readonly hasWorkspace: boolean;
	/** Disables destructive actions while an archive request is in flight. */
	readonly isArchiving?: boolean;
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

/**
 * Shared body of the per-chat actions menu. Used by both the chat top bar's
 * kebab and the sidebar row's kebab/right-click menus so the two stay in
 * lockstep.
 */
export const ChatActionsMenuItems: FC<ChatActionsMenuItemsProps> = ({
	isArchived,
	isPinned,
	isChildChat,
	hasWorkspace,
	isArchiving = false,
	onPinAgent,
	onUnpinAgent,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	onOpenRenameDialog,
	Item,
	Separator,
}) => {
	return (
		<>
			{!isArchived && !isChildChat && onPinAgent && onUnpinAgent && (
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
				<Item disabled={isArchiving} onSelect={onUnarchiveAgent}>
					<ArchiveRestoreIcon className="size-3.5" />
					Unarchive agent
				</Item>
			) : (
				<>
					{onOpenRenameDialog && (
						<Item onSelect={onOpenRenameDialog}>
							<SquarePenIcon className="size-3.5" />
							Rename chat
						</Item>
					)}
					<Separator />
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
	);
};
