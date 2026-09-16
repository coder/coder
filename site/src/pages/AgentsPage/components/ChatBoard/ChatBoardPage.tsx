import {
	type Collision,
	type CollisionDetection,
	DndContext,
	type DragEndEvent,
	type DragMoveEvent,
	DragOverlay,
	type DragStartEvent,
	KeyboardSensor,
	PointerSensor,
	pointerWithin,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import {
	ChevronLeftIcon,
	ChevronUpIcon,
	PlusIcon,
	SearchIcon,
} from "lucide-react";
import {
	type FC,
	type PointerEvent as ReactPointerEvent,
	useEffect,
	useRef,
	useState,
} from "react";
import { useInfiniteQuery, useQuery } from "react-query";
import { useNavigate, useParams } from "react-router";
import { chatSearch, infiniteChats } from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import { useDebouncedValue } from "#/hooks/debounce";
import { pageTitle } from "#/utils/page";
import { buildChatSearchQuery } from "../ChatsSidebar/dialogs/searchQuery";
import { type DragData, DragGhost, type DropData } from "./BoardCard";
import { BoardColumn, NewColumn } from "./BoardColumn";
import {
	type BoardCard as BoardCardModel,
	buildCards,
	buildColumns,
	type CardColor,
	INBOX_COLUMN,
	keyBetween,
	placementKey,
} from "./boardLabels";
import { type ChatPane, useBoardStorage } from "./boardStorage";
import { ChatPanes, closeTab, openTab } from "./ChatPanes";
import { useBlockSelectionWhileDragging } from "./dragHandle";
import { useBoardMutations } from "./useBoardMutations";

const hasOpenChats = (panes: readonly ChatPane[]) => panes.length > 0;

/** Where a drop would land, resolved from the pointer position. */
export type DropTarget =
	| { kind: "merge"; card: BoardCardModel }
	| { kind: "insert"; column: string; beforeCardId: string | null };

const dropDataOf = (hit: Collision): DropData | undefined =>
	hit.data?.droppableContainer?.data.current as DropData | undefined;

// Resolves the pointer to one target. Over a card, the top and bottom
// quarters insert before/after it and the middle merges (chats always join).
// Over column background, insert before the first card whose middle is
// below the pointer. The target rides along as collision data.
const boardCollision: CollisionDetection = (args) => {
	const pointer = args.pointerCoordinates;
	if (!pointer) return [];
	const hits = pointerWithin(args);
	const drag = args.active.data.current as DragData | undefined;
	const cardHit = hits.find((hit) => {
		const data = dropDataOf(hit);
		return data?.type === "card" && data.card.id !== drag?.card.id;
	});
	const columnHit = hits.find((hit) => dropDataOf(hit)?.type === "column");
	if (!columnHit) return [];
	const columnData = dropDataOf(columnHit);
	if (columnData?.type !== "column") return [];

	let target: DropTarget;
	if (cardHit) {
		const cardData = dropDataOf(cardHit);
		const rect = args.droppableRects.get(cardHit.id);
		if (cardData?.type !== "card" || !rect) return [];
		const y = (pointer.y - rect.top) / rect.height;
		const card = cardData.card;
		if (drag?.type === "chat" || (y > 0.25 && y < 0.75)) {
			target = { kind: "merge", card };
		} else if (y <= 0.25) {
			target = { kind: "insert", column: card.column, beforeCardId: card.id };
		} else {
			target = {
				kind: "insert",
				column: card.column,
				beforeCardId: nextCardIdInColumn(args, card),
			};
		}
	} else {
		const columnRect = args.droppableRects.get(columnHit.id);
		const below = columnRect
			? cardsInColumn(args, columnData.name).find(
					({ rect }) => rect.top + rect.height / 2 > pointer.y,
				)
			: undefined;
		target = {
			kind: "insert",
			column: columnData.name,
			beforeCardId: below?.card.id ?? null,
		};
	}
	const hit = cardHit ?? columnHit;
	return [{ id: hit.id, data: { ...hit.data, target } }];
};

const cardsInColumn = (
	args: Parameters<CollisionDetection>[0],
	column: string,
) =>
	args.droppableContainers
		.flatMap((container) => {
			const data = container.data.current as DropData | undefined;
			const rect = args.droppableRects.get(container.id);
			return data?.type === "card" && data.card.column === column && rect
				? [{ card: data.card, rect }]
				: [];
		})
		.sort((a, b) => a.rect.top - b.rect.top);

const nextCardIdInColumn = (
	args: Parameters<CollisionDetection>[0],
	card: BoardCardModel,
): string | null => {
	const ordered = cardsInColumn(args, card.column);
	const index = ordered.findIndex((entry) => entry.card.id === card.id);
	return ordered[index + 1]?.card.id ?? null;
};

const ChatBoardPage: FC = () => {
	const { agentId } = useParams();
	const navigate = useNavigate();
	const [storage, updateStorage] = useBoardStorage();
	const mutations = useBoardMutations();
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search.trim(), 300);
	const [addingColumn, setAddingColumn] = useState(false);
	const [activeDrag, setActiveDrag] = useState<DragData | null>(null);
	const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
	const splitRef = useRef<HTMLDivElement>(null);
	const { panes, focusedPane, chatsCollapsed } = storage;
	const showChats = hasOpenChats(panes) && !chatsCollapsed;
	const setPanes = (next: readonly ChatPane[], focused: number) =>
		updateStorage({
			panes: next,
			focusedPane: Math.max(0, Math.min(focused, next.length - 1)),
		});

	// The route param is an "open this chat" request: it becomes a tab in the
	// focused pane and is then cleared so the same link works again later.
	useEffect(() => {
		if (!agentId) return;
		updateStorage((prev) => ({
			panes: openTab(prev.panes, prev.focusedPane, agentId),
			chatsCollapsed: false,
		}));
		void navigate("/agents/board", { replace: true });
	}, [agentId, navigate, updateStorage]);

	// Escape closes the focused pane's active tab; the board itself stays.
	useEffect(() => {
		if (panes.length === 0) return;
		const onKey = (e: KeyboardEvent) => {
			const target = e.target as HTMLElement | null;
			const typing =
				target?.tagName === "INPUT" ||
				target?.tagName === "TEXTAREA" ||
				target?.isContentEditable;
			if (e.key !== "Escape" || typing) return;
			const active = panes[focusedPane]?.active;
			if (active) setPanes(closeTab(panes, active), focusedPane);
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [panes, focusedPane]);

	// Same query the sidebar uses, so both views share one cache.
	const chatsQuery = useInfiniteQuery(infiniteChats({}));
	const { hasNextPage, isFetchingNextPage, fetchNextPage } = chatsQuery;
	// The board partitions every chat into columns, so it needs the full list.
	useEffect(() => {
		if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
	}, [hasNextPage, isFetchingNextPage, fetchNextPage]);

	// Free text has to travel as a search:"..." token; the backend rejects
	// bare words. Same builder the search dialog uses.
	const searchQuery = useQuery({
		...chatSearch({
			q: `${buildChatSearchQuery([], debouncedSearch) ?? ""} archived:false`,
		}),
		enabled: debouncedSearch.length > 0,
	});
	const sensors = useSensors(
		useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
		useSensor(KeyboardSensor),
	);
	useBlockSelectionWhileDragging(activeDrag !== null);
	const matchingIds =
		debouncedSearch && searchQuery.data
			? new Set(searchQuery.data.map((chat) => chat.id))
			: undefined;

	const chats = chatsQuery.data?.pages.flat() ?? [];
	const chatsById = new Map(chats.map((chat) => [chat.id, chat]));
	const openChatIds = new Set(panes.flatMap((pane) => pane.tabs));
	const allCards = buildCards(chats);
	// A tab wears its card's color, so a tab and its card read as one thing.
	const cardColorByChatId = new Map<string, CardColor>(
		allCards.flatMap((card) => {
			const color = card.color;
			return color
				? card.members.map((member) => [member.id, color] as const)
				: [];
		}),
	);
	const cards = matchingIds
		? allCards.filter((card) =>
				card.members.some((member) => matchingIds.has(member.id)),
			)
		: allCards;
	const columns = buildColumns(
		cards,
		storage.columnOrder,
		storage.emptyColumns,
	);

	const handleDragStart = ({ active }: DragStartEvent) => {
		setActiveDrag((active.data.current as DragData | undefined) ?? null);
	};

	const targetOf = (collisions: readonly Collision[] | null | undefined) =>
		(collisions?.[0]?.data as { target?: DropTarget } | undefined)?.target ??
		null;

	// onDragOver only fires when the droppable id changes, but the zone within
	// one card (insert above, merge, insert below) changes without that.
	const handleDragMove = ({ collisions }: DragMoveEvent) => {
		setDropTarget(targetOf(collisions));
	};

	// Placement key for the slot before `beforeCardId` in `column`, ignoring
	// the card being moved so its old slot does not count as a neighbour.
	const keyForSlot = (
		column: string,
		beforeCardId: string | null,
		movingId: string,
	) => {
		const ordered = (
			columns.find((c) => c.name === column)?.cards ?? []
		).filter((c) => c.id !== movingId);
		const index = beforeCardId
			? ordered.findIndex((c) => c.id === beforeCardId)
			: ordered.length;
		const above = index > 0 ? ordered[index - 1] : undefined;
		const below = index >= 0 ? ordered[index] : undefined;
		return keyBetween(
			above && placementKey(above.primary),
			below && placementKey(below.primary),
		);
	};

	const handleDragEnd = ({ active, collisions }: DragEndEvent) => {
		setActiveDrag(null);
		setDropTarget(null);
		const drag = active.data.current as DragData | undefined;
		const target = targetOf(collisions);
		if (!drag || !target) return;
		if (target.kind === "merge") {
			if (drag.type === "card") {
				void mutations.mergeCards(drag.card, target.card);
			} else if (drag.card.id !== target.card.id) {
				void mutations.joinCard(drag.chat, target.card);
			}
			return;
		}
		if (drag.type === "card") {
			// Dropping back into its own slot is a no-op.
			const ordered =
				columns.find((c) => c.name === target.column)?.cards ?? [];
			const index = ordered.findIndex((c) => c.id === drag.card.id);
			const nextId = ordered[index + 1]?.id ?? null;
			if (
				index >= 0 &&
				(target.beforeCardId === drag.card.id || target.beforeCardId === nextId)
			) {
				return;
			}
			void mutations.moveCard(
				drag.card,
				target.column,
				keyForSlot(target.column, target.beforeCardId, drag.card.id),
			);
			return;
		}
		void mutations.detachChat(
			drag.chat,
			target.column,
			keyForSlot(target.column, target.beforeCardId, drag.chat.id),
		);
	};

	const addColumn = (name: string) => {
		if (columns.some((column) => column.name === name)) return;
		updateStorage({
			emptyColumns: [...storage.emptyColumns, name],
			columnOrder: [...storage.columnOrder, name],
		});
	};

	const renameColumn = (from: string, to: string) => {
		if (columns.some((column) => column.name === to)) return;
		const column = columns.find((c) => c.name === from);
		if (column) void mutations.renameColumn(column.cards, to);
		updateStorage({
			emptyColumns: storage.emptyColumns.map((n) => (n === from ? to : n)),
			columnOrder: storage.columnOrder.map((n) => (n === from ? to : n)),
		});
	};

	const deleteColumn = (name: string) => {
		const column = columns.find((c) => c.name === name);
		if (column) void mutations.renameColumn(column.cards, INBOX_COLUMN);
		updateStorage({
			emptyColumns: storage.emptyColumns.filter((n) => n !== name),
			columnOrder: storage.columnOrder.filter((n) => n !== name),
		});
	};

	// Pointer capture keeps the resize alive when the cursor leaves the bar.
	const startResize = (e: ReactPointerEvent<HTMLDivElement>) => {
		const container = splitRef.current;
		if (!container) return;
		const bar = e.currentTarget;
		bar.setPointerCapture(e.pointerId);
		const rect = container.getBoundingClientRect();
		const onMove = (ev: PointerEvent) => {
			const ratio = (ev.clientY - rect.top) / rect.height;
			updateStorage({ splitRatio: Math.min(0.85, Math.max(0.15, ratio)) });
		};
		const onUp = () => {
			bar.removeEventListener("pointermove", onMove);
			bar.removeEventListener("pointerup", onUp);
		};
		bar.addEventListener("pointermove", onMove);
		bar.addEventListener("pointerup", onUp);
	};

	return (
		<div ref={splitRef} className="flex min-h-0 flex-1 flex-col">
			<title>{pageTitle("Board", "Agents")}</title>
			<div
				className="flex min-h-0 flex-col"
				style={{
					flex: showChats ? `0 0 ${storage.splitRatio * 100}%` : "1 1 0",
				}}
			>
				<div className="flex h-12 shrink-0 items-center gap-3 border-b border-border pr-4 pl-3">
					<Button
						variant="subtle"
						size="icon"
						aria-label="Exit board"
						className="size-7 text-content-secondary"
						// Leaving lands on the chat being read, or the agents home.
						onClick={() => {
							const reading = panes[focusedPane]?.active;
							void navigate(reading ? `/agents/${reading}` : "/agents");
						}}
					>
						<ChevronLeftIcon className="size-4" />
					</Button>
					<h1 className="m-0 text-sm font-medium tracking-[-0.01em] text-content-primary">
						Board
					</h1>
					<span className="pl-1 text-[11px] text-content-secondary/70">
						{chats.length} chats · {allCards.length} cards
					</span>
					<div className="relative ml-auto flex h-[30px] w-[260px] items-center gap-2 rounded-[7px] border border-border bg-surface-primary px-2.5 focus-within:border-content-link">
						<SearchIcon className="size-3.5 shrink-0 text-content-secondary" />
						<input
							aria-label="Filter cards"
							placeholder="Filter cards"
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[13px] text-content-primary outline-none placeholder:text-content-secondary/60"
						/>
						{matchingIds && (
							<span className="text-[11px] text-content-secondary">
								{cards.length}
							</span>
						)}
					</div>
				</div>
				{chatsQuery.isError && (
					<p className="m-0 px-3 py-2 text-sm text-content-destructive">
						Failed to load chats.
					</p>
				)}
				<DndContext
					sensors={sensors}
					collisionDetection={boardCollision}
					onDragStart={handleDragStart}
					onDragMove={handleDragMove}
					onDragEnd={handleDragEnd}
					onDragCancel={() => {
						setActiveDrag(null);
						setDropTarget(null);
					}}
				>
					<div className="flex min-h-0 flex-1 gap-4 overflow-x-auto bg-surface-secondary px-5 pt-4 pb-3">
						{columns.map((column) => (
							<BoardColumn
								key={column.name}
								column={column}
								openChatIds={openChatIds}
								dropTarget={dropTarget}
								onRename={(to) => renameColumn(column.name, to)}
								onDelete={() => deleteColumn(column.name)}
								onSetCardTitle={(card, title) =>
									void mutations.setCardTitle(card, title)
								}
								onSetCardColor={(card, color) =>
									void mutations.setCardColor(card, color)
								}
								onRenameChat={(chat, title) =>
									void mutations.renameChat(chat, title)
								}
								onAddNote={(card, text) =>
									void mutations.addComment(card, text)
								}
								onEditNote={(card, index, text) =>
									void mutations.editComment(card, index, text)
								}
								onRemoveNote={(card, index) =>
									void mutations.removeComment(card, index)
								}
							/>
						))}
						{addingColumn ? (
							<NewColumn
								onCreate={addColumn}
								onCancel={() => setAddingColumn(false)}
							/>
						) : (
							<button
								type="button"
								aria-label="Add column"
								// Sits on the column header line, matching header height.
								className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-md border border-dashed border-content-secondary/40 bg-transparent text-content-secondary hover:border-content-link hover:text-content-link"
								onClick={() => setAddingColumn(true)}
							>
								<PlusIcon className="size-3.5" />
							</button>
						)}
					</div>
					{/* Portaled above every column so the moving card is never clipped. */}
					<DragOverlay dropAnimation={null}>
						{activeDrag && <DragGhost drag={activeDrag} />}
					</DragOverlay>
				</DndContext>
			</div>
			{showChats && (
				<>
					{/* Grip handle: drag to resize the split. */}
					<div
						className="grid h-[5px] shrink-0 cursor-row-resize touch-none place-items-center border-t border-border bg-surface-primary hover:bg-content-link/15"
						onPointerDown={startResize}
					>
						<div className="h-0.5 w-9 rounded-full bg-content-secondary/30" />
					</div>
					<ChatPanes
						panes={panes}
						focusedPane={focusedPane}
						chatsById={chatsById}
						cardColorByChatId={cardColorByChatId}
						onChange={setPanes}
						onCollapse={() => updateStorage({ chatsCollapsed: true })}
					/>
				</>
			)}
			{hasOpenChats(panes) && chatsCollapsed && (
				// Collapsed panes leave a bar at the bottom edge, where they went.
				<div className="flex h-8 shrink-0 items-center justify-end border-t border-border bg-surface-secondary/60 px-2">
					<Button
						variant="subtle"
						size="sm"
						className="text-content-secondary"
						onClick={() => updateStorage({ chatsCollapsed: false })}
					>
						{openChatIds.size === 1 ? "1 chat" : `${openChatIds.size} chats`}
						<ChevronUpIcon />
					</Button>
				</div>
			)}
		</div>
	);
};

export default ChatBoardPage;
