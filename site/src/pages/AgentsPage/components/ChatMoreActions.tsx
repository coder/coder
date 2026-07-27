import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	EllipsisVerticalIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { cn } from "#/utils/cn";

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

interface ChatMoreActionsProps {
	readonly isArchived: boolean;
	readonly isPinned: boolean;
	readonly isChildChat: boolean;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
	readonly onPinAgent?: () => void;
	readonly onUnpinAgent?: () => void;
	readonly onArchiveAgent: () => void;
	readonly onUnarchiveAgent: () => void;
	readonly onArchiveAndDeleteWorkspace: () => void;
	/** When omitted, the "Rename chat" item is hidden. */
	readonly onOpenRenameDialog?: () => void;
	/** Accessible name for the trigger button. */
	readonly triggerLabel: string;
	readonly triggerClassName?: string;
	readonly triggerIconClassName?: string;
	readonly align?: "start" | "center" | "end";
	readonly contentClassName?: string;
}

export const ChatMoreActions: FC<ChatMoreActionsProps> = ({
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
	triggerLabel,
	triggerClassName,
	triggerIconClassName,
	align = "end",
	contentClassName,
}) => {
	const showPinAction =
		!isArchived && !isChildChat && Boolean(onPinAgent && onUnpinAgent);
	const showArchiveActions = !isArchived && !isChildChat;

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					size="icon"
					variant="subtle"
					className={cn(
						"text-content-secondary hover:text-content-primary",
						triggerClassName,
					)}
					aria-label={triggerLabel}
				>
					<EllipsisVerticalIcon className={triggerIconClassName} />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align={align}
				className={cn("[&_[role=menuitem]]:text-[13px]", contentClassName)}
			>
				{showPinAction && (
					<DropdownMenuItem onSelect={isPinned ? onUnpinAgent : onPinAgent}>
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
					</DropdownMenuItem>
				)}
				{isArchived ? (
					!isChildChat && (
						<DropdownMenuItem
							disabled={isArchiving}
							onSelect={onUnarchiveAgent}
						>
							<ArchiveRestoreIcon className="size-3.5" />
							Unarchive agent
						</DropdownMenuItem>
					)
				) : (
					<>
						{onOpenRenameDialog && (
							<DropdownMenuItem onSelect={onOpenRenameDialog}>
								<SquarePenIcon className="size-3.5" />
								Rename chat
							</DropdownMenuItem>
						)}
						{showArchiveActions && (
							<>
								{(onOpenRenameDialog || showPinAction) && (
									<DropdownMenuSeparator />
								)}
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									disabled={isArchiving}
									onSelect={onArchiveAgent}
								>
									<ArchiveIcon className="size-3.5" />
									Archive agent
								</DropdownMenuItem>
								{hasWorkspace && (
									<DropdownMenuItem
										className="text-content-destructive focus:text-content-destructive"
										disabled={isArchiving}
										onSelect={onArchiveAndDeleteWorkspace}
									>
										<Trash2Icon className="size-3.5" />
										Archive & delete workspace
									</DropdownMenuItem>
								)}
							</>
						)}
					</>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
