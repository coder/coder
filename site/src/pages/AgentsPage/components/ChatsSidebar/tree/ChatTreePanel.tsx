import {
	closestCenter,
	DndContext,
	type DragEndEvent,
	KeyboardSensor,
	MouseSensor,
	TouchSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import {
	arrayMove,
	SortableContext,
	sortableKeyboardCoordinates,
	verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import {
	type FC,
	type KeyboardEvent,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { useQueries } from "react-query";
import { chat as chatQuery } from "#/api/queries/chats";
import type { Chat, ChatModel, ChatTreeResponse } from "#/api/typesGenerated";
import type { AgentSidebarFilters } from "../../../utils/agentSidebarFilters";
import {
	ChatSectionHeader,
	getSectionToggleTestId,
	PINNED_SECTION_KEY,
} from "../chats/ChatSectionHeader";
import { ChatsEmptyState } from "../chats/ChatsEmptyState";
import {
	ChatTreePanelContext,
	type ChatTreePanelContextValue,
	type SubagentLoadState,
} from "./ChatTreePanelContext";
import { ChatTreeRow, chatTreeRowDomId } from "./ChatTreeRow";
import {
	type ChatTreeExpansionState,
	isChatTreeNodeExpanded,
	loadChatTreeExpansion,
	persistChatTreeExpansion,
	setChatTreeNodeExpanded,
	setChatTreeSubagentsShown,
} from "./chatTreeExpansion";
import {
	buildChatTreeModel,
	type ChatTreeModel,
	type ChatTreeModelNode,
	type ChatTreeSource,
	collectAncestorIDs,
	collectMatchingChatIDs,
	flattenVisibleTree,
} from "./chatTreeModel";
import { FlatChatRow, SortablePinnedChatRow } from "./SortablePinnedChatRow";
import {
	focusTargetAfterRemoval,
	type TreeKeyboardRow,
	treeKeyboardReducer,
} from "./treeKeyboard";

const SHARED_WITH_YOU_SECTION_KEY = "Shared with you";
const TYPEAHEAD_RESET_MS = 500;

type ChatTreeOrganization = {
	readonly id: string;
	readonly displayName: string;
};

/** Tree-mode data the layout resolves from the per organization queries. */
export interface ChatTreePanelData {
	readonly organizations: readonly ChatTreeOrganization[];
	readonly responsesByOrganization: ReadonlyMap<string, ChatTreeResponse>;
	/** Chats shared with the viewer, from the flat list query. */
	readonly sharedChats: readonly Chat[];
	readonly onCreateChildChat: (chat: Chat) => void;
}

interface ChatTreePanelProps {
	readonly data: ChatTreePanelData;
	readonly sidebarFilters: AgentSidebarFilters;
	readonly onSidebarFiltersChange: (filters: AgentSidebarFilters) => void;
	readonly activeChatId: string | undefined;
	readonly modelConfigs: readonly ChatModel[];
	readonly isLoadingModelConfigs: boolean;
	readonly chatErrorReasons: Record<string, string>;
	readonly isArchiving: boolean;
	readonly archivingChatId: string | null;
	readonly onArchiveAgent: (chatId: string) => void;
	readonly onUnarchiveAgent: (chatId: string) => void;
	readonly onArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	readonly onPinAgent: (chatId: string) => void;
	readonly onUnpinAgent: (chatId: string) => void;
	readonly onReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	readonly onOpenRenameDialog?: (chat: Chat) => void;
}

/**
 * A row removed by the user together with the visible rows at the moment
 * of removal, so the focus target is computed from the layout the user
 * last saw.
 */
type PendingFocus = {
	readonly removedId: string;
	readonly rows: readonly TreeKeyboardRow[];
};

/** Roving focus state, valid only while `activeChatId` is unchanged. */
type FocusedRow = {
	readonly id: string;
	readonly activeChatId: string | undefined;
};

type StatusFilterView = {
	readonly visible: Set<string> | undefined;
	readonly forceExpandedIds: Set<string>;
	readonly dimmedIds: Set<string>;
	readonly matchCount: number;
};

const NO_STATUS_FILTER: StatusFilterView = {
	visible: undefined,
	forceExpandedIds: new Set(),
	dimmedIds: new Set(),
	matchCount: 0,
};

/**
 * Keeps matching chats plus the ancestors needed to reach them. Root and
 * organization nodes are structural: they are force-expanded but never
 * dimmed or counted.
 */
const applyStatusFilter = (
	model: ChatTreeModel,
	statusFilter: "unread" | "read" | undefined,
): StatusFilterView => {
	if (!statusFilter) {
		return NO_STATUS_FILTER;
	}
	const wantUnread = statusFilter === "unread";
	const { matches, ancestors } = collectMatchingChatIDs(
		model,
		(node) =>
			node.kind !== "organization" &&
			node.kind !== "root" &&
			node.chat?.has_unread === wantUnread,
	);
	const dimmedIds = new Set<string>();
	for (const id of ancestors) {
		const kind = model.nodesById.get(id)?.kind;
		if (kind === "chat" || kind === "subagent") {
			dimmedIds.add(id);
		}
	}
	return {
		visible: new Set([...matches, ...ancestors]),
		forceExpandedIds: ancestors,
		dimmedIds,
		matchCount: matches.size,
	};
};

/**
 * Moves focus once the removed row has left the tree. Nothing happens
 * while the removed row is still rendered.
 */
const FocusAfterRemoval: FC<{
	readonly pending: PendingFocus | null;
	readonly rows: readonly TreeKeyboardRow[];
	readonly onResolve: (targetId: string | undefined) => void;
}> = ({ pending, rows, onResolve }) => {
	const removedPresent =
		pending !== null && rows.some((row) => row.id === pending.removedId);
	useEffect(() => {
		if (pending === null || removedPresent) {
			return;
		}
		const target = focusTargetAfterRemoval(pending.rows, pending.removedId);
		onResolve(
			target !== undefined && rows.some((row) => row.id === target)
				? target
				: undefined,
		);
	}, [pending, removedPresent, rows, onResolve]);
	return null;
};

export const ChatTreePanel: FC<ChatTreePanelProps> = (props) => {
	const { data, activeChatId } = props;
	const [expansion, setExpansion] = useState<ChatTreeExpansionState>(
		loadChatTreeExpansion,
	);
	const [autoExpandedFor, setAutoExpandedFor] = useState<string | undefined>();
	useEffect(() => {
		persistChatTreeExpansion(expansion);
	}, [expansion]);

	const responseChatIds = new Set<string>();
	for (const response of data.responsesByOrganization.values()) {
		for (const row of response.chats) {
			responseChatIds.add(row.id);
		}
	}
	// Subagents are fetched per toggled node; the tree response has none.
	const subagentParentIds = [...expansion.subagents].filter((id) =>
		responseChatIds.has(id),
	);
	const subagentQueries = useQueries({
		queries: subagentParentIds.map((id) => chatQuery(id)),
	});
	const subagentsByParent = new Map<string, readonly Chat[]>();
	const subagentLoadStates = new Map<string, SubagentLoadState>();
	for (const [index, id] of subagentParentIds.entries()) {
		const query = subagentQueries[index];
		subagentsByParent.set(id, query?.data?.children ?? []);
		if (query?.isError) {
			subagentLoadStates.set(id, "error");
		} else if (query?.isPending) {
			subagentLoadStates.set(id, "pending");
		}
	}

	const sources: ChatTreeSource[] = data.organizations.flatMap(
		(organization) => {
			const response = data.responsesByOrganization.get(organization.id);
			return response ? [{ organization, response, subagentsByParent }] : [];
		},
	);
	const model = buildChatTreeModel(sources);

	// Ancestors of the active chat expand once per navigation, when the
	// chat is present in the model; user collapses afterwards are kept.
	// Both updates target this component's own state during render.
	if (!activeChatId && autoExpandedFor !== undefined) {
		setAutoExpandedFor(undefined);
	}
	if (
		activeChatId &&
		activeChatId !== autoExpandedFor &&
		model.nodesById.has(activeChatId)
	) {
		setAutoExpandedFor(activeChatId);
		let next = expansion;
		for (const ancestorId of collectAncestorIDs(model, activeChatId)) {
			const ancestor = model.nodesById.get(ancestorId);
			if (ancestor) {
				next = setChatTreeNodeExpanded(next, ancestor, true);
			}
		}
		if (next !== expansion) {
			setExpansion(next);
		}
	}

	const setNodeExpanded = (id: string, expanded: boolean) => {
		const node = model.nodesById.get(id);
		if (node) {
			setExpansion((current) =>
				setChatTreeNodeExpanded(current, node, expanded),
			);
		}
	};
	const toggleSubagents = (id: string) => {
		setExpansion((current) =>
			setChatTreeSubagentsShown(current, id, !current.subagents.has(id)),
		);
		const node = model.nodesById.get(id);
		if (node && !expansion.subagents.has(id)) {
			setExpansion((current) => setChatTreeNodeExpanded(current, node, true));
		}
	};

	return (
		<ChatTreeBody
			{...props}
			model={model}
			expansion={expansion}
			subagentLoadStates={subagentLoadStates}
			setNodeExpanded={setNodeExpanded}
			toggleSubagents={toggleSubagents}
		/>
	);
};

interface ChatTreeBodyProps extends ChatTreePanelProps {
	readonly model: ChatTreeModel;
	readonly expansion: ChatTreeExpansionState;
	readonly subagentLoadStates: ReadonlyMap<string, SubagentLoadState>;
	readonly setNodeExpanded: (id: string, expanded: boolean) => void;
	readonly toggleSubagents: (id: string) => void;
}

const ChatTreeBody: FC<ChatTreeBodyProps> = ({
	data,
	sidebarFilters,
	onSidebarFiltersChange,
	activeChatId,
	modelConfigs,
	isLoadingModelConfigs,
	chatErrorReasons,
	isArchiving,
	archivingChatId,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	onPinAgent,
	onUnpinAgent,
	onReorderPinnedAgent,
	onOpenRenameDialog,
	model,
	expansion,
	subagentLoadStates,
	setNodeExpanded,
	toggleSubagents,
}) => {
	const [focusedRow, setFocusedRow] = useState<FocusedRow | undefined>();
	const [pendingFocus, setPendingFocus] = useState<PendingFocus | null>(null);
	const treeDomId = useId();
	const [localPinOrder, setLocalPinOrder] = useState<{
		serverOrder: string;
		ids: string[];
	} | null>(null);
	const sensors = useSensors(
		useSensor(MouseSensor, { activationConstraint: { distance: 5 } }),
		useSensor(TouchSensor, {
			activationConstraint: { delay: 200, tolerance: 5 },
		}),
		useSensor(KeyboardSensor, {
			coordinateGetter: sortableKeyboardCoordinates,
		}),
	);
	const [collapsedSections, setCollapsedSections] = useState<
		Record<string, boolean>
	>({});
	const toggleSection = (key: string) =>
		setCollapsedSections((prev) => ({ ...prev, [key]: !prev[key] }));

	const typeahead = useRef("");
	const typeaheadTimer = useRef<number | undefined>(undefined);
	useEffect(() => () => window.clearTimeout(typeaheadTimer.current), []);

	const statusFilter =
		sidebarFilters.chatStatuses.length === 1
			? sidebarFilters.chatStatuses[0]
			: undefined;
	const showOwnedTree = sidebarFilters.sources.includes("created_by_me");
	const { visible, forceExpandedIds, dimmedIds, matchCount } =
		applyStatusFilter(model, statusFilter);

	const isExpanded = (node: ChatTreeModelNode) =>
		forceExpandedIds.has(node.id) || isChatTreeNodeExpanded(expansion, node);
	const rows = showOwnedTree
		? flattenVisibleTree(model, { isExpanded, visible })
		: [];
	const rowsById = new Map(rows.map((row) => [row.id, row]));
	const keyboardRows: TreeKeyboardRow[] = rows.map((row) => ({
		id: row.id,
		level: row.node.level,
		parentId: row.node.parentId,
		hasChildren: row.hasChildren,
		isExpanded: row.isExpanded,
		isForceExpanded: forceExpandedIds.has(row.id),
		label: row.node.label,
	}));

	// A stored focus only counts while the active chat it was recorded
	// under is still active; after navigation the active chat is the
	// single tab stop.
	const focusedIdState =
		focusedRow !== undefined && focusedRow.activeChatId === activeChatId
			? focusedRow.id
			: undefined;
	const focusedId =
		focusedIdState !== undefined && rowsById.has(focusedIdState)
			? focusedIdState
			: activeChatId && rowsById.has(activeChatId)
				? activeChatId
				: rows[0]?.id;
	const setFocusedId = (id: string) => {
		setFocusedRow({ id, activeChatId });
		setPendingFocus(null);
	};
	const focusRow = (id: string) => {
		setFocusedId(id);
		document.getElementById(chatTreeRowDomId(treeDomId, id))?.focus();
	};
	const resolvePendingFocus = (targetId: string | undefined) => {
		setPendingFocus(null);
		if (targetId) {
			focusRow(targetId);
		}
	};
	const removeRowWithFocus = (chatId: string, action: () => void) => {
		setPendingFocus({ removedId: chatId, rows: keyboardRows });
		action();
	};

	const toggleExpanded = (id: string) => {
		const row = rowsById.get(id);
		if (row && !forceExpandedIds.has(id)) {
			setNodeExpanded(id, !row.isExpanded);
		}
	};

	const handleTreeKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		if (event.ctrlKey || event.metaKey || event.altKey) {
			return;
		}
		const target = event.target;
		if (
			!(target instanceof HTMLElement) ||
			target.getAttribute("role") !== "treeitem"
		) {
			return;
		}
		// Enter is the anchor's native activation (and the organization
		// node handles its own toggle), so the reducer never sees it.
		if (event.key === "Enter") {
			return;
		}
		const result = treeKeyboardReducer(
			keyboardRows,
			focusedId,
			event.key,
			typeahead.current,
		);
		if (!result.handled) {
			return;
		}
		event.preventDefault();
		typeahead.current = result.typeahead;
		window.clearTimeout(typeaheadTimer.current);
		if (result.typeahead) {
			typeaheadTimer.current = window.setTimeout(() => {
				typeahead.current = "";
			}, TYPEAHEAD_RESET_MS);
		}
		if (result.toggle) {
			setNodeExpanded(result.toggle.id, result.toggle.expanded);
		}
		for (const id of result.expandIds ?? []) {
			setNodeExpanded(id, true);
		}
		if (result.focusId) {
			focusRow(result.focusId);
		}
	};

	const requestArchive = (chat: Chat) =>
		removeRowWithFocus(chat.id, () => onArchiveAgent(chat.id));
	// Pinned shortcuts: server order, overridden locally during a drag
	// until the next server order arrives. A status filter narrows them
	// to the visible set.
	const pinnedChats = [...model.nodesById.values()]
		.flatMap((node) =>
			node.kind === "chat" &&
			node.chat &&
			node.chat.pin_order > 0 &&
			(visible === undefined || visible.has(node.id))
				? [node.chat]
				: [],
		)
		.sort((a, b) => a.pin_order - b.pin_order);
	const serverPinOrder = pinnedChats.map((chat) => chat.id).join("\u0000");
	const sortedPinnedChats =
		localPinOrder && localPinOrder.serverOrder === serverPinOrder
			? localPinOrder.ids.flatMap((id) => {
					const chat = pinnedChats.find((c) => c.id === id);
					return chat ? [chat] : [];
				})
			: pinnedChats;
	const pinnedChatIds = sortedPinnedChats.map((chat) => chat.id);
	const disablePinnedReordering =
		statusFilter !== undefined || onReorderPinnedAgent === undefined;
	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		if (disablePinnedReordering || !over || active.id === over.id) {
			return;
		}
		const activeId = String(active.id);
		const oldIndex = pinnedChatIds.indexOf(activeId);
		const newIndex = pinnedChatIds.indexOf(String(over.id));
		if (oldIndex === -1 || newIndex === -1) {
			return;
		}
		setLocalPinOrder({
			serverOrder: serverPinOrder,
			ids: arrayMove(pinnedChatIds, oldIndex, newIndex),
		});
		onReorderPinnedAgent?.(activeId, newIndex + 1);
	};

	const isViewingArchived = sidebarFilters.archiveStatus === "archived";
	const isEmpty = rows.length === 0 && data.sharedChats.length === 0;

	const contextValue: ChatTreePanelContextValue = {
		model,
		rowsById,
		focusedId,
		activeChatId,
		dimmedIds,
		forceExpandedIds,
		subagentsShownIds: expansion.subagents,
		subagentLoadStates,
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
		requestUnarchive: (chat) =>
			removeRowWithFocus(chat.id, () => onUnarchiveAgent(chat.id)),
		requestArchiveAndDeleteWorkspace: (chat, workspaceId) =>
			removeRowWithFocus(chat.id, () =>
				onArchiveAndDeleteWorkspace(chat.id, workspaceId),
			),
		onPinAgent,
		onUnpinAgent,
		onOpenRenameDialog,
		onCreateChildChat: data.onCreateChildChat,
	};

	return (
		<ChatTreePanelContext value={contextValue}>
			<div className="pb-2">
				<div role="status" aria-live="polite" className="sr-only">
					{statusFilter
						? `${matchCount} ${matchCount === 1 ? "chat matches" : "chats match"}`
						: ""}
				</div>
				{isEmpty ? (
					<ChatsEmptyState
						filters={sidebarFilters}
						onFiltersChange={onSidebarFiltersChange}
					/>
				) : (
					<>
						{sortedPinnedChats.length > 0 && (
							<div className="not-first:mt-3">
								<ChatSectionHeader
									label={PINNED_SECTION_KEY}
									count={sortedPinnedChats.length}
									expanded={!collapsedSections[PINNED_SECTION_KEY]}
									onToggle={() => toggleSection(PINNED_SECTION_KEY)}
									testId={getSectionToggleTestId(PINNED_SECTION_KEY)}
								/>
								{!collapsedSections[PINNED_SECTION_KEY] &&
									(disablePinnedReordering ? (
										<div className="flex flex-col gap-0.5">
											{sortedPinnedChats.map((chat) => (
												<FlatChatRow
													key={chat.id}
													chat={chat}
													isActive={chat.id === activeChatId}
												/>
											))}
										</div>
									) : (
										<DndContext
											sensors={sensors}
											collisionDetection={closestCenter}
											modifiers={[({ transform }) => ({ ...transform, x: 0 })]}
											onDragEnd={handleDragEnd}
										>
											<SortableContext
												items={pinnedChatIds}
												strategy={verticalListSortingStrategy}
											>
												<div className="flex flex-col gap-0.5">
													{sortedPinnedChats.map((chat) => (
														<SortablePinnedChatRow
															key={chat.id}
															chat={chat}
															isActive={chat.id === activeChatId}
														/>
													))}
												</div>
											</SortableContext>
										</DndContext>
									))}
							</div>
						)}
						{data.sharedChats.length > 0 && (
							<div className="not-first:mt-3">
								<ChatSectionHeader
									label={SHARED_WITH_YOU_SECTION_KEY}
									count={data.sharedChats.length}
									expanded={!collapsedSections[SHARED_WITH_YOU_SECTION_KEY]}
									onToggle={() => toggleSection(SHARED_WITH_YOU_SECTION_KEY)}
									testId={getSectionToggleTestId(SHARED_WITH_YOU_SECTION_KEY)}
								/>
								{!collapsedSections[SHARED_WITH_YOU_SECTION_KEY] && (
									<div className="flex flex-col gap-0.5">
										{data.sharedChats.map((chat) => (
											<FlatChatRow
												key={chat.id}
												chat={chat}
												isActive={chat.id === activeChatId}
											/>
										))}
									</div>
								)}
							</div>
						)}
						{rows.length > 0 && (
							<div
								role="tree"
								aria-label={
									isViewingArchived ? "Archived chat tree" : "Chat tree"
								}
								className="not-first:mt-3 flex min-w-0 flex-col gap-0.5"
								onKeyDown={handleTreeKeyDown}
							>
								{model.topLevelIds
									.filter((id) => rowsById.has(id))
									.map((id) => (
										<ChatTreeRow key={id} id={id} />
									))}
							</div>
						)}
					</>
				)}
			</div>
			<FocusAfterRemoval
				pending={pendingFocus}
				rows={keyboardRows}
				onResolve={resolvePendingFocus}
			/>
		</ChatTreePanelContext>
	);
};
