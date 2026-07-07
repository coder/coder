import {
	ChevronDownIcon,
	ChevronRightIcon,
	EllipsisVerticalIcon,
	UsersIcon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { NavLink, useLocation } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { shortRelativeTime } from "#/utils/time";
import {
	ChatActionsMenuItems,
	chatHasMenuActions,
} from "../../ChatActionsMenuItems";
import { asNonEmptyString } from "../../ChatConversation/blockUtils";
import { normalizeLocationSearch } from "../locationSearch";
import { useChatTree } from "./ChatTreeContext";
import { getParentChatID } from "./chatTree";
import { getModelDisplayName } from "./modelDisplayName";
import { getChatDisplayConfig } from "./statusConfig";

interface ChatTreeNodeProps {
	readonly chat: Chat;
	readonly isChildNode: boolean;
}

export const ChatTreeNode: FC<ChatTreeNodeProps> = ({ chat, isChildNode }) => {
	const location = useLocation();
	const locationSearch = normalizeLocationSearch(location.search);
	const {
		chatTree,
		chatById,
		visibleChatIDs,
		normalizedSearch,
		expandedById,
		modelOptions,
		modelConfigs,
		chatErrorReasons,
		activeChatId,
		isArchiving,
		archivingChatId,
		toggleExpanded,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog,
	} = useChatTree();
	const chatID = chat.id;
	const isActiveChat = activeChatId === chatID;
	const childIDs = (chatTree.childrenById.get(chatID) ?? []).filter((childID) =>
		visibleChatIDs.has(childID),
	);
	const hasChildren = childIDs.length > 0;
	const isDelegated = Boolean(getParentChatID(chat));
	const isDelegatedExecuting =
		isDelegated && (chat.status === "pending" || chat.status === "running");
	const modelName = getModelDisplayName(
		chat.last_model_config_id,
		modelConfigs,
		modelOptions,
	);
	const errorReason =
		chat.status === "error"
			? chatErrorReasons[chat.id] || chat.last_error?.message || undefined
			: undefined;
	const lastTurnSummary = asNonEmptyString(chat.last_turn_summary);
	const isStreaming = chat.status === "running" || chat.status === "pending";
	const streamingSubtitle = isStreaming ? `${modelName} streaming…` : undefined;
	const staleTurnSummaryReleaseMs = 10_000;
	const [streamingSummary, setStreamingSummary] = useState<string | undefined>(
		isStreaming ? lastTurnSummary : undefined,
	);
	const [suppressionExpired, setSuppressionExpired] = useState(false);
	if (isStreaming) {
		if (streamingSummary !== lastTurnSummary) {
			setStreamingSummary(lastTurnSummary);
		}
		if (suppressionExpired) {
			setSuppressionExpired(false);
		}
	} else if (
		streamingSummary !== undefined &&
		lastTurnSummary !== streamingSummary
	) {
		setStreamingSummary(undefined);
		if (suppressionExpired) {
			setSuppressionExpired(false);
		}
	}
	const isStaleTurnSummary =
		!isStreaming &&
		lastTurnSummary !== undefined &&
		!suppressionExpired &&
		streamingSummary === lastTurnSummary;
	useEffect(() => {
		if (!isStaleTurnSummary) {
			return;
		}
		const timeoutId = setTimeout(() => {
			setSuppressionExpired(true);
		}, staleTurnSummaryReleaseMs);
		return () => clearTimeout(timeoutId);
	}, [isStaleTurnSummary]);
	const displayedTurnSummary = isStaleTurnSummary ? undefined : lastTurnSummary;
	const isSharedChat = chat.shared;
	const subtitle =
		errorReason || streamingSubtitle || displayedTurnSummary || modelName;
	const {
		icon: StatusIcon,
		className: statusClassName,
		diffStatus,
	} = getChatDisplayConfig(chat);
	const hasLinkedDiffStatus = Boolean(diffStatus?.url);
	const changedFiles = diffStatus?.changed_files ?? 0;
	const additions = diffStatus?.additions ?? 0;
	const deletions = diffStatus?.deletions ?? 0;
	const hasLineStats = additions > 0 || deletions > 0 || changedFiles > 0;
	const filesChangedLabel = `${changedFiles} ${
		changedFiles === 1 ? "file" : "files"
	}`;
	const workspaceId = chat.workspace_id;
	const isArchivingThisChat = isArchiving && archivingChatId === chat.id;
	const isExpanded = normalizedSearch ? true : (expandedById[chatID] ?? false);

	const hasMenuActions = chatHasMenuActions({
		isArchived: chat.archived,
		isChildChat: isChildNode,
	});
	const sharedMenuItemProps = {
		isArchived: chat.archived,
		isPinned: chat.pin_order > 0,
		isChildChat: isChildNode,
		hasWorkspace: Boolean(workspaceId),
		isArchiving,
		onPinAgent: () => onPinAgent(chat.id),
		onUnpinAgent: () => onUnpinAgent(chat.id),
		onArchiveAgent: () => onArchiveAgent(chat.id),
		onUnarchiveAgent: () => onUnarchiveAgent(chat.id),
		onArchiveAndDeleteWorkspace: () => {
			if (workspaceId) {
				onArchiveAndDeleteWorkspace(chat.id, workspaceId);
			}
		},
		onOpenRenameDialog: onOpenRenameDialog
			? () => onOpenRenameDialog(chat)
			: undefined,
	};

	return (
		<div className="flex min-w-0 flex-col gap-0.5">
			<ContextMenu>
				<ContextMenuTrigger asChild disabled={!hasMenuActions}>
					<div
						data-testid={`agents-tree-node-${chat.id}`}
						className={cn(
							"group relative flex min-w-0 select-none [@media(pointer:coarse)]:[-webkit-touch-callout:none] items-start gap-1.5 rounded-md pl-1 pr-1.5 text-content-secondary",
							"transition-none [@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary has-[[data-state=open]]:bg-surface-tertiary",
							"has-[[aria-current=page]]:bg-surface-quaternary/25 has-[[aria-current=page]]:text-content-primary [@media(hover:hover)]:has-[[aria-current=page]]:hover:bg-surface-quaternary/50",
							isChildNode &&
								"before:absolute before:-left-2.5 before:top-[17px] before:h-px before:w-2.5 before:bg-border-default/70",
						)}
					>
						<div
							className={cn(
								"group/icon relative mt-1.5 size-5 shrink-0",
								hasChildren && "cursor-pointer",
							)}
						>
							<div
								className={cn(
									"flex size-5 items-center justify-center rounded-md",
									hasChildren &&
										"[@media(hover:hover)]:group-hover/icon:invisible",
								)}
							>
								<StatusIcon
									data-testid={
										isDelegatedExecuting
											? `agents-tree-executing-${chat.id}`
											: undefined
									}
									className={cn("size-3.5 shrink-0", statusClassName)}
								/>
							</div>
							{hasChildren && (
								<Button
									variant="subtle"
									size="icon"
									onClick={() => toggleExpanded(chatID)}
									className={cn(
										"absolute inset-0 invisible flex size-5 min-w-0 items-center justify-center rounded-md p-0 text-content-secondary/60 hover:text-content-primary [&>svg]:size-3.5",
										"[@media(hover:hover)]:group-hover/icon:visible",
									)}
									data-testid={`agents-tree-toggle-${chat.id}`}
									aria-label={isExpanded ? "Collapse" : "Expand"}
									aria-expanded={isExpanded}
								>
									{isExpanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
								</Button>
							)}
						</div>
						<NavLink
							to={{
								pathname: `/agents/${chat.id}`,
								search: locationSearch,
							}}
							className="flex min-h-0 min-w-0 flex-1 items-start gap-2 rounded-[inherit] py-1 pr-0.5 text-inherit no-underline"
						>
							{({ isActive }) => (
								<div className="min-w-0 flex-1 overflow-hidden text-left">
									<div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
										<span
											className={cn(
												"block flex-1 truncate text-[13px] text-content-primary",
												isActive && "font-medium",
											)}
										>
											{chat.title}
										</span>
										{chat.has_unread && !isActiveChat && (
											<span className="sr-only">(unread)</span>
										)}
									</div>
									<div className="flex min-w-0 items-center gap-1.5">
										{hasLinkedDiffStatus && hasLineStats && (
											<span
												className="inline-flex shrink-0 items-center gap-0.5 text-[13px] leading-4 tabular-nums"
												title={`${filesChangedLabel}, +${additions} -${deletions}`}
											>
												<span className="text-git-added-bright">
													+{additions}
												</span>
												<span className="text-git-deleted-bright">
													&minus;{deletions}
												</span>
											</span>
										)}
										<div
											className={cn(
												"min-w-0 overflow-hidden text-[13px] leading-4",
												errorReason
													? "line-clamp-1 whitespace-normal text-content-destructive [overflow-wrap:anywhere]"
													: "truncate text-content-secondary",
											)}
											title={subtitle}
										>
											{subtitle}
										</div>
									</div>
								</div>
							)}
						</NavLink>
						<div className="relative my-1 flex w-7 shrink-0 flex-col items-end self-stretch">
							<div className="flex h-6 w-7 shrink-0 items-center justify-end">
								{isArchivingThisChat ? (
									<Spinner
										className="h-3.5 w-3.5 text-content-secondary"
										loading
									/>
								) : (
									<span
										className={cn(
											"flex items-center justify-end text-xs text-content-secondary/50 tabular-nums",
											// The timestamp swaps out for the actions trigger on
											// hover; without menu actions there is no trigger, so
											// keep the timestamp visible.
											hasMenuActions &&
												"[@media(hover:hover)]:group-hover:hidden group-has-[[data-state=open]]:hidden",
										)}
									>
										{chat.has_unread && !isActiveChat ? (
											<span
												className="size-2 shrink-0 rounded-full bg-content-link pr-1"
												data-testid={`unread-indicator-${chat.id}`}
												aria-hidden="true"
											/>
										) : (
											<>
												{/* Pin the ignored mask width so Chromatic does not diff bounding rect changes. */}
												<span
													data-pixel="ignore"
													className="inline-block w-7 text-right"
												>
													{shortRelativeTime(chat.updated_at)}
												</span>
											</>
										)}
									</span>
								)}
							</div>
							{isSharedChat && (
								<UsersIcon
									className="mt-auto size-3.5 text-content-secondary"
									aria-label="Shared chat"
								/>
							)}
							{hasMenuActions && (
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button
											size="icon"
											variant="subtle"
											className="absolute inset-0 flex h-6 w-7 min-w-0 justify-end rounded-none px-0 opacity-0 text-content-secondary hover:text-content-primary [@media(hover:hover)]:group-hover:opacity-100 data-[state=open]:opacity-100"
											aria-label={`Open actions for ${chat.title}`}
										>
											<EllipsisVerticalIcon className="size-3.5" />
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent
										align="end"
										className="[&_[role=menuitem]]:text-[13px]"
									>
										<ChatActionsMenuItems
											{...sharedMenuItemProps}
											Item={DropdownMenuItem}
											Separator={DropdownMenuSeparator}
										/>
									</DropdownMenuContent>
								</DropdownMenu>
							)}
						</div>
					</div>
				</ContextMenuTrigger>
				<ContextMenuContent className="[&_[role=menuitem]]:text-[13px]">
					<ChatActionsMenuItems
						{...sharedMenuItemProps}
						Item={ContextMenuItem}
						Separator={ContextMenuSeparator}
					/>
				</ContextMenuContent>
			</ContextMenu>

			{hasChildren && isExpanded && (
				<div className="relative ml-4 flex flex-col border-l border-border-default/60 pl-2.5">
					{childIDs.map((childID) => {
						const childChat = chatById.get(childID);
						if (!childChat) return null;
						return (
							<ChatTreeNode key={childChat.id} chat={childChat} isChildNode />
						);
					})}
				</div>
			)}
		</div>
	);
};
