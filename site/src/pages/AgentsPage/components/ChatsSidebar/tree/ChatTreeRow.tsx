import { cn } from "cn";
import {
	BotIcon,
	Building2Icon,
	ChevronDownIcon,
	ChevronRightIcon,
	EllipsisVerticalIcon,
	HouseIcon,
	PinIcon,
} from "lucide-react";
import type { FC, KeyboardEvent } from "react";
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
import { shortRelativeTime } from "#/utils/time";
import {
	ChatActionsMenuItems,
	chatFamilyAllowsArchive,
	chatHasMenuActions,
} from "../../ChatActionsMenuItems";
import { normalizeLocationSearch } from "../locationSearch";
import { useChatTreePanel } from "./ChatTreePanelContext";
import {
	type ChatTreeModelNode,
	type ChatTreeVisibleRow,
	collectDescendantChats,
	getChatTreeChildren,
	isChatAtTreeDepthLimit,
} from "./chatTreeModel";
import { getModelDisplayName } from "./modelDisplayName";
import { getChatDisplayConfig } from "./statusConfig";

const CHAT_TREE_INDENT_PX = 16;

export const chatTreeRowDomId = (treeDomId: string, nodeId: string) =>
	`${treeDomId}-${nodeId}`;

const rowSurfaceClassName = cn(
	"group relative flex min-w-0 select-none items-center gap-1 rounded-md pr-1 text-content-secondary",
	"[@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary has-data-[state=open]:bg-surface-tertiary",
	"has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary",
	"has-[[role=treeitem]:focus-visible]:ring-2 has-[[role=treeitem]:focus-visible]:ring-inset has-[[role=treeitem]:focus-visible]:ring-content-link",
);

const treeitemClassName =
	"flex min-h-7 min-w-0 flex-1 items-center gap-1.5 rounded-[inherit] py-1 text-inherit no-underline outline-hidden";

interface IndentGuidesProps {
	readonly level: number;
}

const IndentGuides: FC<IndentGuidesProps> = ({ level }) => (
	<span
		aria-hidden="true"
		className="flex shrink-0 self-stretch"
		style={{ width: (level - 1) * CHAT_TREE_INDENT_PX }}
	>
		{Array.from({ length: level - 1 }, (_, index) => (
			<span
				key={index}
				className="block h-full border-0 border-l border-solid border-border-default"
				style={{ width: CHAT_TREE_INDENT_PX, marginLeft: index === 0 ? 9 : 0 }}
			/>
		))}
	</span>
);

interface ChevronProps {
	readonly rowId: string;
	readonly hasChildren: boolean;
	readonly isExpanded: boolean;
	readonly onToggle: () => void;
}

// Pointer-only affordance: always visible and 24 px wide on coarse
// pointers, hover-revealed on fine pointers. It never takes DOM focus
// (pointerdown is cancelled, tabIndex -1) and is hidden from assistive
// technology; the treeitem's aria-expanded carries the state and receives
// focus after every toggle.
const Chevron: FC<ChevronProps> = ({
	rowId,
	hasChildren,
	isExpanded,
	onToggle,
}) => {
	const { treeDomId } = useChatTreePanel();
	if (!hasChildren) {
		return <span aria-hidden="true" className="size-6 shrink-0" />;
	}
	return (
		<Button
			variant="subtle"
			size="icon"
			tabIndex={-1}
			aria-hidden="true"
			onPointerDown={(event) => event.preventDefault()}
			onClick={(event) => {
				event.preventDefault();
				onToggle();
				document.getElementById(chatTreeRowDomId(treeDomId, rowId))?.focus();
			}}
			className={cn(
				"size-6 min-w-0 shrink-0 rounded-md p-0 text-content-secondary/70 hover:text-content-primary [&>svg]:size-3.5",
				"[@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100 [@media(hover:hover)]:group-focus-within:opacity-100",
			)}
		>
			{isExpanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
		</Button>
	);
};

interface ChatTreeRowProps {
	readonly id: string;
}

/**
 * One tree node: the row followed by a `group` of child rows when
 * expanded. The `treeitem` is the navigation anchor (or a div for the
 * synthetic organization node); the wrapper carries no role.
 */
export const ChatTreeRow: FC<ChatTreeRowProps> = ({ id }) => {
	const { model, rowsById, subagentLoadStates } = useChatTreePanel();
	const row = rowsById.get(id);
	const node = model.nodesById.get(id);
	if (!row || !node) {
		return null;
	}
	const childIds = getChatTreeChildren(model, id).filter((childId) =>
		rowsById.has(childId),
	);
	const showChildren = row.isExpanded && childIds.length > 0;
	// A node without children has no expander, so its load line shows as
	// soon as the fetch starts; otherwise it follows the expanded state.
	const subagentLoadState = subagentLoadStates.get(id);
	const showSubagentLoadState =
		subagentLoadState !== undefined && (row.isExpanded || !row.hasChildren);
	return (
		<div role="none" className="flex min-w-0 flex-col">
			{node.kind === "organization" ? (
				<OrganizationTreeItem node={node} row={row} />
			) : node.chat ? (
				<ChatTreeItem node={node} chat={node.chat} row={row} />
			) : null}
			{(showChildren || showSubagentLoadState) && (
				<div role="group" className="flex min-w-0 flex-col">
					{showChildren &&
						childIds.map((childId) => (
							<ChatTreeRow key={childId} id={childId} />
						))}
					{showSubagentLoadState && (
						<div
							role="none"
							className="flex min-h-7 min-w-0 items-center gap-1 pr-1 text-xs text-content-secondary/70"
						>
							<IndentGuides level={node.level + 1} />
							<span aria-hidden="true" className="size-6 shrink-0" />
							{subagentLoadState === "error"
								? "Failed to load subagents"
								: "Loading subagents"}
						</div>
					)}
				</div>
			)}
		</div>
	);
};

type TreeItemProps = {
	readonly node: ChatTreeModelNode;
	readonly row: ChatTreeVisibleRow;
};

const OrganizationTreeItem: FC<TreeItemProps> = ({ node, row }) => {
	const {
		focusedId,
		forceExpandedIds,
		treeDomId,
		setFocusedId,
		toggleExpanded,
	} = useChatTreePanel();
	const isFocused = focusedId === node.id;
	// A filter holds the node open, so the toggle affordance is withheld.
	const canToggle = row.hasChildren && !forceExpandedIds.has(node.id);
	const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		if (event.key === "Enter" || event.key === " ") {
			event.preventDefault();
			toggleExpanded(node.id);
		}
	};
	return (
		<div className={rowSurfaceClassName}>
			<Chevron
				rowId={node.id}
				hasChildren={canToggle}
				isExpanded={row.isExpanded}
				onToggle={() => toggleExpanded(node.id)}
			/>
			<div
				id={chatTreeRowDomId(treeDomId, node.id)}
				role="treeitem"
				tabIndex={isFocused ? 0 : -1}
				aria-level={node.level}
				aria-setsize={row.setSize}
				aria-posinset={row.posInSet}
				aria-expanded={row.hasChildren ? row.isExpanded : undefined}
				className={cn(treeitemClassName, "cursor-default")}
				onClick={() => toggleExpanded(node.id)}
				onFocus={() => setFocusedId(node.id)}
				onKeyDown={onKeyDown}
			>
				<Building2Icon aria-hidden="true" className="size-3.5 shrink-0" />
				<span className="truncate text-[13px] font-medium text-content-primary">
					{node.label}
				</span>
			</div>
		</div>
	);
};

const ChatTreeItem: FC<TreeItemProps & { readonly chat: Chat }> = ({
	node,
	chat,
	row,
}) => {
	const location = useLocation();
	const locationSearch = normalizeLocationSearch(location.search);
	const {
		model,
		focusedId,
		activeChatId,
		dimmedIds,
		forceExpandedIds,
		subagentsShownIds,
		modelConfigs,
		isLoadingModelConfigs,
		chatErrorReasons,
		isArchiving,
		archivingChatId,
		treeDomId,
		setFocusedId,
		toggleExpanded,
		toggleSubagents,
		requestArchive,
		requestUnarchive,
		requestArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog,
		onCreateChildChat,
	} = useChatTreePanel();

	const isActiveChat = activeChatId === chat.id;
	const isFocused = focusedId === chat.id;
	const isDimmed = dimmedIds.has(chat.id);
	const isSubagent = node.kind === "subagent";
	const isRoot = node.kind === "root";
	const {
		icon: StatusIcon,
		className: statusClassName,
		label: statusLabel,
	} = getChatDisplayConfig(chat);
	const errorReason =
		chat.status === "error"
			? chatErrorReasons[chat.id] || chat.last_error?.message || undefined
			: undefined;
	const modelName = getModelDisplayName(
		chat.last_model_config_id,
		modelConfigs,
		isLoadingModelConfigs,
	);
	const subtitle = errorReason ?? (isRoot ? undefined : modelName);
	const isArchivingThisChat = isArchiving && archivingChatId === chat.id;
	const hasMenuActions = chatHasMenuActions(chat);
	const workspaceId = chat.workspace_id;
	const isPinned = chat.pin_order > 0;
	// A filter holds the node open, so the toggle affordance is withheld.
	const canToggle = row.hasChildren && !forceExpandedIds.has(chat.id);
	const namedChildCount = getChatTreeChildren(model, chat.id).filter(
		(childId) => model.nodesById.get(childId)?.kind !== "subagent",
	).length;
	const childCountHint =
		!row.isExpanded && namedChildCount > 0 ? namedChildCount : undefined;
	const parent =
		node.parentId === undefined
			? undefined
			: model.nodesById.get(node.parentId);
	const isParentArchived = parent?.chat?.archived === true;
	const descendants = collectDescendantChats(model, chat.id);
	const isArchiveBlocked =
		!chatFamilyAllowsArchive(chat.status, chat.children) ||
		descendants.some(
			(descendant) => !chatFamilyAllowsArchive(descendant.status, undefined),
		);

	const menuItemProps = {
		chat,
		hasWorkspace: Boolean(workspaceId),
		isArchiving,
		isArchiveBlocked,
		isParentArchived,
		isSubagentsExpanded: subagentsShownIds.has(chat.id),
		onToggleSubagents: isSubagent ? undefined : () => toggleSubagents(chat.id),
		onToggleExpanded: canToggle ? () => toggleExpanded(chat.id) : undefined,
		isExpanded: row.isExpanded,
		onCreateChildChat: isSubagent ? undefined : () => onCreateChildChat(chat),
		isChildChatDepthLimitReached: isChatAtTreeDepthLimit(chat),
		onPinAgent: () => onPinAgent(chat.id),
		onUnpinAgent: () => onUnpinAgent(chat.id),
		onArchiveAgent: () => requestArchive(chat),
		onUnarchiveAgent: () => requestUnarchive(chat),
		onArchiveAndDeleteWorkspace: () => {
			if (workspaceId) {
				requestArchiveAndDeleteWorkspace(chat, workspaceId);
			}
		},
		onOpenRenameDialog: onOpenRenameDialog
			? () => onOpenRenameDialog(chat)
			: undefined,
	};

	const KindIcon = isRoot ? HouseIcon : isSubagent ? BotIcon : StatusIcon;
	// Read after the title so the treeitem name starts with the title and
	// type-ahead matches what is announced.
	const kindLabel = isRoot
		? "Root chat"
		: isSubagent
			? `Subagent, ${statusLabel}`
			: statusLabel;

	return (
		<ContextMenu>
			<ContextMenuTrigger asChild disabled={!hasMenuActions}>
				<div
					data-testid={`chat-tree-row-${chat.id}`}
					className={cn(rowSurfaceClassName, isDimmed && "opacity-60")}
				>
					<IndentGuides level={node.level} />
					<Chevron
						rowId={chat.id}
						hasChildren={canToggle}
						isExpanded={row.isExpanded}
						onToggle={() => toggleExpanded(chat.id)}
					/>
					<NavLink
						id={chatTreeRowDomId(treeDomId, chat.id)}
						to={{ pathname: `/agents/${chat.id}`, search: locationSearch }}
						role="treeitem"
						tabIndex={isFocused ? 0 : -1}
						aria-level={node.level}
						aria-setsize={row.setSize}
						aria-posinset={row.posInSet}
						aria-expanded={row.hasChildren ? row.isExpanded : undefined}
						className={treeitemClassName}
						onFocus={() => setFocusedId(chat.id)}
					>
						<KindIcon
							aria-hidden="true"
							className={cn(
								"size-3.5 shrink-0",
								isRoot ? "text-content-secondary" : statusClassName,
							)}
						/>
						<span className="flex min-w-0 flex-1 flex-col overflow-hidden">
							<span className="flex min-w-0 items-center gap-1.5">
								<span
									className={cn(
										"truncate text-[13px] text-content-primary",
										!isActiveChat &&
											"opacity-85 [@media(hover:hover)]:group-hover:opacity-100",
									)}
								>
									{chat.title}
								</span>
								<span className="sr-only">, {kindLabel}</span>
								{chat.has_unread && !isActiveChat && (
									<span className="sr-only">(unread)</span>
								)}
								{isDimmed && (
									<span className="sr-only">(does not match filters)</span>
								)}
								{childCountHint !== undefined && (
									<span className="shrink-0 text-xs text-content-secondary/70 tabular-nums">
										{childCountHint} {childCountHint === 1 ? "chat" : "chats"}
									</span>
								)}
								{isPinned && (
									<PinIcon
										role="img"
										aria-label="Pinned"
										className="size-3 shrink-0 text-content-secondary/70"
									/>
								)}
							</span>
							{subtitle && (
								<span
									className={cn(
										"truncate text-xs leading-4",
										errorReason
											? "text-content-destructive"
											: "text-content-secondary",
									)}
									title={subtitle}
								>
									{subtitle}
								</span>
							)}
						</span>
					</NavLink>
					<div className="relative flex h-6 w-7 shrink-0 items-center justify-end">
						{isArchivingThisChat ? (
							<Spinner className="h-3.5 w-3.5 text-content-secondary" loading />
						) : (
							<span
								className={cn(
									"flex items-center justify-end text-xs text-content-secondary/50 tabular-nums",
									hasMenuActions &&
										"[@media(hover:none)]:hidden [@media(hover:hover)]:group-hover:hidden group-has-data-[state=open]:hidden group-focus-within:hidden",
								)}
							>
								{chat.has_unread && !isActiveChat ? (
									<span className="flex w-3.5 shrink-0 justify-center">
										<span
											className="size-2 rounded-full bg-content-link"
											data-testid={`unread-indicator-${chat.id}`}
											aria-hidden="true"
										/>
									</span>
								) : (
									<span
										data-pixel="ignore"
										className="inline-block w-7 text-right"
									>
										{isRoot ? "" : shortRelativeTime(chat.updated_at)}
									</span>
								)}
							</span>
						)}
						{hasMenuActions && !isArchivingThisChat && (
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button
										size="icon"
										variant="subtle"
										tabIndex={isFocused ? 0 : -1}
										className={cn(
											"absolute inset-0 flex h-6 w-7 min-w-0 justify-end rounded-none px-0 text-content-secondary hover:text-content-primary",
											"[@media(hover:hover)]:group-hover:opacity-100 [@media(hover:hover)]:group-focus-within:opacity-100 [@media(hover:hover)]:data-[state=open]:opacity-100",
											!isActiveChat && "[@media(hover:hover)]:opacity-0",
										)}
										aria-label={`Open actions for ${chat.title}`}
										onContextMenuCapture={(event) => {
											event.preventDefault();
											event.stopPropagation();
										}}
										onPointerDownCapture={(event) => {
											if (event.button === 2) {
												event.preventDefault();
												event.stopPropagation();
											}
										}}
									>
										<EllipsisVerticalIcon className="size-3.5" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent
									align="end"
									className="[&_[role=menuitem]]:text-[13px]"
									onContextMenu={(event) => {
										event.preventDefault();
										event.stopPropagation();
									}}
								>
									<ChatActionsMenuItems
										{...menuItemProps}
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
					{...menuItemProps}
					Item={ContextMenuItem}
					Separator={ContextMenuSeparator}
				/>
			</ContextMenuContent>
		</ContextMenu>
	);
};
