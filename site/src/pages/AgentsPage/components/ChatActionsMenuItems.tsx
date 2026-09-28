import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	BotIcon,
	CopyIcon,
	GitBranchIcon,
	HashIcon,
	MailIcon,
	MailOpenIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import { useId, useState } from "react";
import { useIsMutating, useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	archiveAndDeleteChat,
	archiveAndDeleteChatKey,
	chatArchiveMutationKey,
} from "#/api/queries/chats";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { useClipboard } from "#/hooks/useClipboard";
import {
	type ArchiveAndDeleteAction,
	fetchArchiveAndDeleteAction,
	notifyArchiveAndDeleteFailed,
	notifyDeleteQueueState,
} from "../utils/agentWorkspaceUtils";
import { clearPersistedRightPanelState } from "../utils/rightPanelTabStorage";
import { clearPersistedSidebarTabId } from "../utils/sidebarTabStorage";
import { ArchiveAndDeleteWorkspaceDialog } from "./ArchiveAndDeleteWorkspaceDialog";
import { getParentChatID } from "./ChatConversation/chatHelpers";
import { ChatPRMenuItems } from "./ChatsSidebar/tree/ChatPRMenuItems";

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
type SubComponent = typeof DropdownMenuSub | typeof ContextMenuSub;
type SubTriggerComponent =
	| typeof DropdownMenuSubTrigger
	| typeof ContextMenuSubTrigger;
type SubContentComponent =
	| typeof DropdownMenuSubContent
	| typeof ContextMenuSubContent;

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

/**
 * Every chat exposes copy actions, so the menu itself is never empty and
 * call sites always render their trigger.
 */
type ChatActionsMenuItemsProps = {
	readonly chat: TypesGen.Chat;
	/**
	 * See {@link canManageChat}. When false, only the subagents toggle, the
	 * copy actions and the PR links render.
	 */
	readonly canManage: boolean;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
	readonly isUpdatingReadState?: boolean;
	readonly isArchiveBlocked?: boolean;
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
	/** When omitted or empty, no PR links render. */
	readonly prStatuses?: readonly TypesGen.ChatDiffStatus[];
	readonly Item: ItemComponent;
	readonly Separator: SeparatorComponent;
	readonly Sub: SubComponent;
	readonly SubTrigger: SubTriggerComponent;
	readonly SubContent: SubContentComponent;
};

type ChatActionsMenuProps = Omit<
	ChatActionsMenuItemsProps,
	| "Item"
	| "Separator"
	| "Sub"
	| "SubTrigger"
	| "SubContent"
	| "onArchiveAndDeleteWorkspace"
> & {
	readonly children: React.ReactNode;
	readonly variant?: "dropdown" | "context";
	readonly align?: "start" | "end";
	readonly contentClassName?: string;
	readonly disabled?: boolean;
	readonly onArchived?: (chatId: string) => void;
};

/** Owns chat menu actions and keeps confirmation alive after the menu closes. */
export const ChatActionsMenu: React.FC<ChatActionsMenuProps> = ({
	children,
	variant = "dropdown",
	align = "end",
	contentClassName,
	disabled,
	onArchived,
	...items
}) => {
	const { chat } = items;
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const [confirmation, setConfirmation] = useState<TypesGen.Workspace>();
	const isArchiving =
		useIsMutating({ mutationKey: chatArchiveMutationKey(chat.id) }) > 0;
	const options = archiveAndDeleteChat(queryClient);
	const mutation = useMutation({
		...options,
		mutationKey: archiveAndDeleteChatKey(chat.id),
		onSuccess: (result, variables) => {
			options.onSuccess(result, variables);
			clearPersistedSidebarTabId(variables.chatId);
			clearPersistedRightPanelState(variables.chatId);
			if (variables.workspaceId) {
				notifyDeleteQueueState(
					queryClient.getQueryData<TypesGen.Workspace>(
						workspaceByIdKey(variables.workspaceId),
					),
					result.deleteBuild,
				);
			}
			onArchived?.(variables.chatId);
		},
		onError: (error, variables) => {
			notifyArchiveAndDeleteFailed(
				variables.workspaceId
					? queryClient.getQueryData<TypesGen.Workspace>(
							workspaceByIdKey(variables.workspaceId),
						)
					: undefined,
				error,
				navigate,
			);
		},
	});

	const filters = { mutationKey: chatArchiveMutationKey(chat.id) };
	const requestArchiveAndDelete = async () => {
		const workspaceId = chat.workspace_id;
		if (chat.archived || !workspaceId || queryClient.isMutating(filters)) {
			return;
		}
		let action: ArchiveAndDeleteAction;
		try {
			action = await fetchArchiveAndDeleteAction(
				queryClient,
				workspaceId,
				chat.created_at,
			);
		} catch (error) {
			toast.error(
				getErrorMessage(error, "Failed to look up workspace for deletion."),
			);
			return;
		}
		if (queryClient.isMutating(filters)) {
			return;
		}
		if (action === "confirm") {
			setConfirmation(
				queryClient.getQueryData<TypesGen.Workspace>(
					workspaceByIdKey(workspaceId),
				),
			);
		} else {
			mutation.mutate({
				chatId: chat.id,
				workspaceId: action === "archive-only" ? undefined : workspaceId,
			});
		}
	};

	const menuItems = (
		<ChatActionsMenuItems
			{...items}
			isArchiving={items.isArchiving || isArchiving}
			onArchiveAndDeleteWorkspace={requestArchiveAndDelete}
			Item={variant === "context" ? ContextMenuItem : DropdownMenuItem}
			Separator={
				variant === "context" ? ContextMenuSeparator : DropdownMenuSeparator
			}
			Sub={variant === "context" ? ContextMenuSub : DropdownMenuSub}
			SubTrigger={
				variant === "context" ? ContextMenuSubTrigger : DropdownMenuSubTrigger
			}
			SubContent={
				variant === "context" ? ContextMenuSubContent : DropdownMenuSubContent
			}
		/>
	);

	return (
		<>
			{variant === "context" ? (
				<ContextMenu>
					<ContextMenuTrigger asChild disabled={disabled}>
						{children}
					</ContextMenuTrigger>
					<ContextMenuContent className={contentClassName}>
						{menuItems}
					</ContextMenuContent>
				</ContextMenu>
			) : (
				<DropdownMenu>
					<DropdownMenuTrigger asChild disabled={disabled}>
						{children}
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align={align}
						className={contentClassName}
						onContextMenu={(event) => {
							// Portaled dropdown events must not open the row's context menu.
							event.preventDefault();
							event.stopPropagation();
						}}
					>
						{menuItems}
					</DropdownMenuContent>
				</DropdownMenu>
			)}
			<ArchiveAndDeleteWorkspaceDialog
				workspace={confirmation}
				onCancel={() => setConfirmation(undefined)}
				onConfirm={(workspace) => {
					if (!queryClient.isMutating(filters)) {
						mutation.mutate({ chatId: chat.id, workspaceId: workspace.id });
					}
					setConfirmation(undefined);
				}}
			/>
		</>
	);
};

const ChatActionsMenuItems: React.FC<ChatActionsMenuItemsProps> = ({
	chat,
	canManage,
	hasWorkspace,
	isArchiving = false,
	isUpdatingReadState = false,
	isArchiveBlocked = false,
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
	prStatuses,
	Item,
	Separator,
	Sub,
	SubTrigger,
	SubContent,
}) => {
	const { copyToClipboard } = useClipboard();
	const isArchived = chat.archived;
	const isPinned = chat.pin_order > 0;
	const isChildChat = getParentChatID(chat) !== undefined;
	const showSubagentsToggle = Boolean(onToggleSubagents) && subagentCount > 0;
	const showReadToggle = Boolean(onMarkRead && onMarkUnread);
	const showPinAction =
		!isArchived && !isChildChat && Boolean(onPinAgent && onUnpinAgent);
	// Archive state is root-only on the backend and cascades to children, so
	// child chats expose neither archive nor unarchive.
	const showArchiveActions = canManage && !isArchived && !isChildChat;
	const showUnarchiveAction = canManage && isArchived && !isChildChat;
	const hasActionsAboveCopy = canManage
		? showUnarchiveAction ||
			(!isArchived &&
				(Boolean(onOpenRenameDialog) ||
					showPinAction ||
					showSubagentsToggle ||
					showReadToggle))
		: showSubagentsToggle;
	const branch = chat.diff_status?.head_branch;
	const archiveBlockedHintId = useId();
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
		<Item
			disabled={isUpdatingReadState}
			onSelect={chat.has_unread ? onMarkRead : onMarkUnread}
		>
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

	const copyItems = [
		<Item
			key="id"
			onSelect={() => {
				void copyToClipboard(chat.id);
			}}
		>
			<HashIcon className="size-3.5" />
			Copy ID
		</Item>,
	];
	if (branch) {
		copyItems.push(
			<Item
				key="branch"
				onSelect={() => {
					void copyToClipboard(branch);
				}}
			>
				<GitBranchIcon className="size-3.5" />
				Copy branch
			</Item>,
		);
	}
	const copyActions =
		copyItems.length > 1 ? (
			<Sub>
				<SubTrigger>
					<CopyIcon className="size-3.5" />
					Copy
				</SubTrigger>
				<SubContent>{copyItems}</SubContent>
			</Sub>
		) : (
			copyItems
		);
	const prLinks = prStatuses && prStatuses.length > 0 && (
		<>
			<Separator />
			<ChatPRMenuItems
				prStatuses={prStatuses}
				Item={Item}
				Sub={Sub}
				SubTrigger={SubTrigger}
				SubContent={SubContent}
			/>
		</>
	);

	if (!canManage) {
		return (
			<>
				{subagentToggle}
				{hasActionsAboveCopy && <Separator />}
				{copyActions}
				{prLinks}
			</>
		);
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
				showUnarchiveAction && (
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
				</>
			)}
			{hasActionsAboveCopy && <Separator />}
			{copyActions}
			{prLinks}
			{showArchiveActions && (
				<>
					<Separator />
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
	);
};
