import { cn } from "cn";
import {
	ChevronDownIcon,
	ChevronRightIcon,
	EllipsisVerticalIcon,
	UsersIcon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { NavLink, useLocation } from "react-router";
import type { Chat, ChatDiffStatus } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
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
import { Spinner } from "#/components/Spinner/Spinner";
import { Tooltip, TooltipTrigger } from "#/components/Tooltip/Tooltip";
import { shortRelativeTime } from "#/utils/time";
import { useSidebarChatLayout } from "../../../hooks/useSidebarChatLayout";
import {
	ChatActionsMenuItems,
	canManageChat,
	chatFamilyAllowsArchive,
	chatHasMenuActions,
	type PullRequestSubmenuComponents,
} from "../../ChatActionsMenuItems";
import { asNonEmptyString } from "../../ChatConversation/blockUtils";
import { normalizeLocationSearch } from "../locationSearch";
import { ChatNodePRIcon, PRListTooltipContent } from "./ChatNodePRIcon";
import { useChatTree } from "./ChatTreeContext";
import { getParentChatID } from "./chatTree";
import { getModelDisplayName } from "./modelDisplayName";
import { getChatDisplayConfig } from "./statusConfig";

type ChatTreeNodeProps = {
	readonly chat: Chat;
	readonly depth?: number;
};

const CHILD_INDENT_PX = 24;

const dropdownSubmenu: PullRequestSubmenuComponents = {
	Sub: DropdownMenuSub,
	SubTrigger: DropdownMenuSubTrigger,
	SubContent: DropdownMenuSubContent,
};

const contextSubmenu: PullRequestSubmenuComponents = {
	Sub: ContextMenuSub,
	SubTrigger: ContextMenuSubTrigger,
	SubContent: ContextMenuSubContent,
};

export const ChatTreeNode: FC<ChatTreeNodeProps> = ({ chat, depth = 0 }) => {
	const location = useLocation();
	const locationSearch = normalizeLocationSearch(location.search);
	const {
		chatTree,
		chatById,
		visibleChatIDs,
		normalizedSearch,
		expandedById,
		modelConfigs,
		isLoadingModelConfigs,
		chatErrorReasons,
		activeChatId,
		currentUserId,
		isArchiving,
		archivingChatId,
		toggleExpanded,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog,
		shareableOrganizationIds,
		onOpenSharingDialog,
	} = useChatTree();
	const [sidebarChatLayout] = useSidebarChatLayout();
	const isOneLine = sidebarChatLayout === "one_line";
	const chatID = chat.id;
	const isActiveChat = activeChatId === chatID;
	const childIDs = (chatTree.childrenById.get(chatID) ?? []).filter((childID) =>
		visibleChatIDs.has(childID),
	);
	const hasChildren = childIDs.length > 0;
	const isDelegated = Boolean(getParentChatID(chat));
	const isDelegatedExecuting = isDelegated && chat.status === "running";
	// Subagent rows always use the single-line row, even in the two-line
	// layout.
	const isCompact = isOneLine || isDelegated;
	// The status icon, title, badge and kebab share one center line: 12px
	// from the top for two-line rows, 14px for subagent rows (28px tall)
	// and 16px for one-line rows (32px tall).
	const rowSpacing = isDelegated
		? { link: "py-0.5", statusIcon: "mt-1", side: "my-0.5" }
		: isOneLine
			? { link: "py-1", statusIcon: "mt-1.5", side: "my-1" }
			: { link: "pt-1 pb-1.5", statusIcon: "mt-0.5", side: "my-0" };
	const modelName = getModelDisplayName(
		chat.last_model_config_id,
		modelConfigs,
		isLoadingModelConfigs,
	);
	const errorReason =
		chat.status === "error"
			? chatErrorReasons[chat.id] || chat.last_error?.message || undefined
			: undefined;
	const lastTurnSummary = asNonEmptyString(chat.last_turn_summary);
	const isStreaming = chat.status === "running";
	const streamingSubtitle =
		isStreaming && modelName ? `${modelName} streaming…` : undefined;
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
	const subtitle =
		errorReason || streamingSubtitle || displayedTurnSummary || modelName;
	const {
		icon: StatusIcon,
		className: statusClassName,
		label: statusLabel,
		prStatuses,
	} = getChatDisplayConfig(chat);
	// The sole PR's line stats can differ from the primary row's,
	// which may be a newer branch-only ref with zeroed counts.
	const solePR = prStatuses.length === 1 ? prStatuses[0] : undefined;
	const hasLinkedDiffStatus = Boolean(solePR?.url);

	const changedFiles = solePR?.changed_files ?? 0;
	const additions = solePR?.additions ?? 0;
	const deletions = solePR?.deletions ?? 0;
	const hasLineStats = additions > 0 || deletions > 0 || changedFiles > 0;
	const filesChangedLabel = `${changedFiles} ${
		changedFiles === 1 ? "file" : "files"
	}`;
	const workspaceId = chat.workspace_id;
	const isArchivingThisChat = isArchiving && archivingChatId === chat.id;
	const isExpanded = normalizedSearch ? true : (expandedById[chatID] ?? false);

	const canManage = canManageChat(chat, currentUserId);
	const linkedPullRequests = prStatuses.filter((status) => status.url);
	const hasMenuActions = chatHasMenuActions(chat, {
		canManage,
		hasSubagentsToggle: hasChildren,
		hasPullRequests: linkedPullRequests.length > 0,
	});
	const canShare =
		canManage &&
		!isDelegated &&
		!chat.archived &&
		shareableOrganizationIds.has(chat.organization_id);
	// Idle chats swap their checkmark for the unread dot; other statuses
	// (working, error, needs action) keep their icon.
	const showUnreadDot =
		chat.has_unread && !isActiveChat && chat.status === "waiting";
	// Titles that need attention (working or unread) stand out; the rest
	// recede until hovered or opened.
	const isEmphasizedTitle = isStreaming || (chat.has_unread && !isActiveChat);

	// Rows sit inside a section's guide-lined list, so the highlight
	// bleeds left only to that line (the active border then overlays
	// it) and fully to the right edge. Padding offsets the bleed so the
	// content does not shift.
	const hoverLayout =
		"[@media(hover:hover)]:hover:-ml-[5px] [@media(hover:hover)]:hover:-mr-2 [@media(hover:hover)]:hover:pl-[5px] [@media(hover:hover)]:hover:pr-3.5 [@media(hover:hover)]:hover:rounded-none";
	const activeLayout =
		"has-[[aria-current=page]]:-ml-[5px] has-[[aria-current=page]]:-mr-2 has-[[aria-current=page]]:pl-[3px] has-[[aria-current=page]]:pr-3.5 has-[[aria-current=page]]:rounded-none has-[[aria-current=page]]:border-l-2 has-[[aria-current=page]]:border-content-secondary [@media(hover:hover)]:has-[[aria-current=page]]:hover:pl-[3px]";
	const sharedMenuItemProps = {
		chat,
		canManage,
		hasWorkspace: Boolean(workspaceId),
		isArchiving,
		isArchiveBlocked: !chatFamilyAllowsArchive(chat.status, chat.children),
		subagentCount: childIDs.length,
		isSubagentsExpanded: isExpanded,
		onToggleSubagents: () => toggleExpanded(chatID),
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
		onOpenSharingDialog: canShare ? () => onOpenSharingDialog(chat) : undefined,
		pullRequests: linkedPullRequests,
	};

	// The tooltip lists every tracked PR, so it belongs to the whole
	// row: link focus opens it for keyboard users, and no focusable
	// descendant nests inside the anchor.
	const chatLink = (
		<NavLink
			to={{
				pathname: `/agents/${chat.id}`,
				search: locationSearch,
			}}
			className={cn(
				"flex min-h-0 min-w-0 flex-1 items-start gap-2 rounded-[inherit] pr-0.5 text-inherit no-underline",
				rowSpacing.link,
			)}
		>
			{({ isActive }) => (
				<div className="min-w-0 flex-1 overflow-hidden text-left">
					<div
						className={cn(
							"flex min-w-0 items-center gap-1.5 overflow-hidden",
							// Compact rows match the 24px status and badge slots;
							// two-line rows use a 16px line per rowSpacing.
							isCompact ? "h-6" : "h-4",
						)}
					>
						<span
							className={cn(
								"block flex-1 truncate text-[13px] leading-4",
								isActive || isEmphasizedTitle
									? "text-content-primary"
									: "text-content-secondary [@media(hover:hover)]:group-hover:text-content-primary",
							)}
						>
							{chat.title}
						</span>
						{chat.has_unread && !isActiveChat && (
							<span className="sr-only">(unread)</span>
						)}
					</div>
					{!isCompact && (
						<div className="mt-0.5 flex min-w-0 items-center gap-1.5">
							<ChatNodePRIcon prStatuses={prStatuses} />
							{prStatuses.length === 1 &&
								hasLinkedDiffStatus &&
								hasLineStats && (
									<span
										className="inline-flex shrink-0 items-center gap-0.5 text-[13px] leading-4 tabular-nums"
										title={`${filesChangedLabel}, +${additions} -${deletions}`}
									>
										<span className="text-git-added-bright">+{additions}</span>
										<span className="text-git-deleted-bright">
											&minus;{deletions}
										</span>
									</span>
								)}
							<div
								className={cn(
									"min-w-0 overflow-hidden text-[13px] leading-4",
									errorReason
										? "line-clamp-1 whitespace-normal text-content-destructive wrap-anywhere"
										: "truncate text-content-secondary",
								)}
								title={subtitle}
							>
								{subtitle}
							</div>
						</div>
					)}
				</div>
			)}
		</NavLink>
	);

	return (
		<div className="flex min-w-0 flex-col">
			<ContextMenu>
				<ContextMenuTrigger asChild disabled={!hasMenuActions}>
					<div
						data-testid={`agents-tree-node-${chat.id}`}
						className={cn(
							"group relative flex min-w-0 select-none pointer-coarse:[-webkit-touch-callout:none] items-start gap-1.5 rounded-md pr-1.5 text-content-secondary",
							"transition-none [@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary has-data-[state=open]:bg-surface-tertiary",
							"has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary [@media(hover:hover)]:has-[[aria-current=page]]:hover:bg-surface-quaternary/50",
							hoverLayout,
							activeLayout,
						)}
					>
						<div
							className={cn(
								"group/icon relative size-5 shrink-0",
								rowSpacing.statusIcon,
								hasChildren && "cursor-pointer",
							)}
							style={
								depth > 0 ? { marginLeft: depth * CHILD_INDENT_PX } : undefined
							}
						>
							<div
								className={cn(
									"flex size-5 items-center justify-center rounded-md",
									hasChildren &&
										"[@media(hover:hover)]:group-hover/icon:invisible",
								)}
							>
								{showUnreadDot ? (
									<span
										className="size-2 rounded-full bg-content-link"
										data-testid={`unread-indicator-${chat.id}`}
										aria-hidden="true"
									/>
								) : (
									<StatusIcon
										data-testid={
											isDelegatedExecuting
												? `agents-tree-executing-${chat.id}`
												: undefined
										}
										role="img"
										aria-label={statusLabel}
										className={cn("size-3.5 shrink-0", statusClassName)}
									/>
								)}
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
						{prStatuses.length > 1 && !isCompact ? (
							<Tooltip>
								<TooltipTrigger asChild>{chatLink}</TooltipTrigger>
								<PRListTooltipContent prStatuses={prStatuses} />
							</Tooltip>
						) : (
							chatLink
						)}
						<div
							className={cn(
								"relative flex min-w-7 shrink-0 justify-end",
								rowSpacing.side,
							)}
						>
							<div className="flex h-6 items-center justify-end">
								{isArchivingThisChat ? (
									<Spinner
										className="h-3.5 w-3.5 text-content-secondary"
										loading
									/>
								) : (
									<ChatRowBadge
										chat={chat}
										isOneLine={isCompact}
										prStatuses={prStatuses}
										className={cn(
											// The badge swaps out for the actions trigger on
											// hover; without menu actions there is no trigger,
											// so keep the badge visible.
											hasMenuActions &&
												"[@media(hover:hover)]:group-hover:hidden group-has-data-[state=open]:hidden",
											hasMenuActions && isActiveChat && "hidden",
										)}
									/>
								)}
							</div>
							{hasMenuActions && !isArchivingThisChat && (
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button
											size="icon"
											variant="subtle"
											className={cn(
												"absolute right-0 top-0 flex h-6 w-7 min-w-0 justify-end rounded-none px-0 opacity-0 text-content-secondary hover:text-content-primary [@media(hover:hover)]:group-hover:opacity-100 data-[state=open]:opacity-100",
												isActiveChat && "opacity-100",
											)}
											aria-label={`Open actions for ${chat.title}`}
											onContextMenuCapture={(e) => {
												e.preventDefault();
												e.stopPropagation();
											}}
											onMouseDownCapture={(e) => {
												if (e.button === 2) {
													e.preventDefault();
													e.stopPropagation();
												}
											}}
											onPointerDownCapture={(e) => {
												if (e.button === 2) {
													e.preventDefault();
													e.stopPropagation();
												}
											}}
										>
											<EllipsisVerticalIcon className="size-3.5" />
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent
										align="end"
										className="[&_[role=menuitem]]:text-[13px]"
										// The dropdown is portaled to the body, but React
										// portals bubble events through the React tree, so a
										// right-click inside the menu would still reach the
										// row's context-menu trigger and open a duplicate menu.
										onContextMenu={(e) => {
											e.preventDefault();
											e.stopPropagation();
										}}
									>
										<ChatActionsMenuItems
											{...sharedMenuItemProps}
											submenu={dropdownSubmenu}
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
						submenu={contextSubmenu}
						Item={ContextMenuItem}
						Separator={ContextMenuSeparator}
					/>
				</ContextMenuContent>
			</ContextMenu>

			{hasChildren && isExpanded && (
				<div className="relative flex flex-col pb-2">
					{childIDs.map((childID) => {
						const childChat = chatById.get(childID);
						if (!childChat) return null;
						return (
							<ChatTreeNode
								key={childChat.id}
								chat={childChat}
								depth={depth + 1}
							/>
						);
					})}
				</div>
			)}
		</div>
	);
};

type ChatRowBadgeProps = {
	readonly chat: Chat;
	readonly isOneLine: boolean;
	readonly prStatuses: ChatDiffStatus[];
	readonly className?: string;
};

const badgeClassName =
	"inline-flex h-5 shrink-0 items-center gap-1 rounded-md bg-surface-secondary px-1.5 text-xs text-content-secondary tabular-nums";

// Two-line rows show when the last turn ended plus the shared icon; the
// PR state sits on the details line. One-line rows have no details
// line, so the badge carries the shared and PR icons instead.
const ChatRowBadge: FC<ChatRowBadgeProps> = ({
	chat,
	isOneLine,
	prStatuses,
	className,
}) => {
	const sharedIcon = chat.shared && (
		<UsersIcon
			role="img"
			aria-label="Shared chat"
			className="size-3.5 shrink-0"
		/>
	);

	if (isOneLine) {
		if (!chat.shared && prStatuses.length === 0) {
			return null;
		}
		return (
			<span className={cn(badgeClassName, className)}>
				{sharedIcon}
				<ChatNodePRIcon prStatuses={prStatuses} />
			</span>
		);
	}

	return (
		<span className={cn(badgeClassName, className)}>
			{/* Pin the ignored mask width so Pixel does not diff bounding rect changes. */}
			<span data-pixel="ignore" className="inline-block w-6 text-center">
				{shortRelativeTime(chat.updated_at)}
			</span>
			{sharedIcon}
		</span>
	);
};
