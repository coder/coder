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
import { ChevronLeftIcon, PlusIcon, SearchIcon } from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useQuery } from "react-query";
import { useNavigate, useParams } from "react-router";
import { chatSearch, infiniteChats } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useDebouncedValue } from "#/hooks/debounce";
import { pageTitle } from "#/utils/page";
import { buildChatSearchQuery } from "../../components/ChatsSidebar/dialogs/searchQuery";
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
import { type ChatWindow, useBoardStorage } from "./boardStorage";
import { FloatingChat, windowBeside, windowCentered } from "./ChatWindows";
import { useBlockSelectionWhileDragging } from "./dragHandle";
import { useBoardMutations } from "./useBoardMutations";
import { findAssistant, useCardAssistant } from "./useCardAssistant";

// Hover must be deliberate before a full chat mounts; leaving gives the
// pointer time to cross into the window.
const PREVIEW_OPEN_MS = 450;
const PREVIEW_CLOSE_MS = 300;

/** Where a drop would land, resolved from the pointer position. */
export type DropTarget =
	| { kind: "merge"; card: BoardCardModel }
	| { kind: "insert"; column: string; beforeCardId: string | null }
	| { kind: "column"; name: string; side: "before" | "after" };

const dropDataOf = (hit: Collision): DropData | undefined =>
	hit.data?.droppableContainer?.data.current as DropData | undefined;

// Resolves the pointer to one target. A column drag lands before or after
// the column under the pointer. Over a card, the top and bottom quarters
// insert before/after it and the middle merges (chats always join). Over
// column background, insert before the first card whose middle is below the
// pointer. The target rides along as collision data.
const boardCollision: CollisionDetection = (args) => {
	const pointer = args.pointerCoordinates;
	if (!pointer) return [];
	const hits = pointerWithin(args);
	const drag = args.active.data.current as DragData | undefined;
	const columnHit = hits.find((hit) => dropDataOf(hit)?.type === "column");
	if (!columnHit) return [];
	const columnData = dropDataOf(columnHit);
	if (columnData?.type !== "column") return [];

	if (drag?.type === "column") {
		const rect = args.droppableRects.get(columnHit.id);
		if (!rect || columnData.name === drag.name) return [];
		const target: DropTarget = {
			kind: "column",
			name: columnData.name,
			side: pointer.x < rect.left + rect.width / 2 ? "before" : "after",
		};
		return [{ id: columnHit.id, data: { ...columnHit.data, target } }];
	}

	const cardHit = hits.find((hit) => {
		const data = dropDataOf(hit);
		return data?.type === "card" && data.card.id !== drag?.card.id;
	});
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
	// An unpinned window shown while the pointer rests on a chat icon.
	const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
	// One list for pinned windows and the hover preview (pinned: false), so
	// pinning is a flag flip on the same element and a gesture in progress
	// survives it. Every update is functional: gesture handlers hold stale
	// closures by the time they commit.
	const { windows } = storage;
	const preview = windows.find((w) => !w.pinned);

	const clearPreviewTimer = () => {
		if (previewTimer.current) clearTimeout(previewTimer.current);
		previewTimer.current = null;
	};
	const setWindows = (
		next: (prev: readonly ChatWindow[]) => readonly ChatWindow[],
	) => updateStorage((prev) => ({ windows: next(prev.windows) }));
	const dropPreview = (list: readonly ChatWindow[]) =>
		list.filter((w) => w.pinned);
	// Pinned or raised windows move to the end, which is the front.
	const toFront = (list: readonly ChatWindow[], win: ChatWindow) => [
		...list.filter((w) => w.chatId !== win.chatId),
		{ ...win, pinned: true },
	];
	const pinWindow = (win: ChatWindow) => {
		clearPreviewTimer();
		setWindows((prev) => toFront(dropPreview(prev), win));
	};
	const interact = (chatId: string) => {
		clearPreviewTimer();
		setWindows((prev) => {
			const win = prev.find((w) => w.chatId === chatId);
			return win ? toFront(prev, win) : prev;
		});
	};
	const changeWindow = (next: ChatWindow) =>
		setWindows((prev) =>
			prev.map((w) =>
				w.chatId === next.chatId ? { ...next, pinned: w.pinned } : w,
			),
		);
	const closeWindow = (chatId: string) =>
		setWindows((prev) => prev.filter((w) => w.chatId !== chatId));

	const openChat = (chat: Chat, anchor: DOMRect) => {
		clearPreviewTimer();
		setWindows((prev) =>
			toFront(
				dropPreview(prev),
				prev.find((w) => w.chatId === chat.id) ??
					windowBeside(chat.id, anchor, true),
			),
		);
	};
	const previewChat = (chat: Chat, anchor: DOMRect) => {
		clearPreviewTimer();
		if (windows.some((w) => w.chatId === chat.id)) return;
		previewTimer.current = setTimeout(
			() =>
				setWindows((prev) => [
					...dropPreview(prev),
					windowBeside(chat.id, anchor, false),
				]),
			PREVIEW_OPEN_MS,
		);
	};
	const endPreview = () => {
		clearPreviewTimer();
		previewTimer.current = setTimeout(
			() => setWindows(dropPreview),
			PREVIEW_CLOSE_MS,
		);
	};

	// The route param is an "open this chat" request: it becomes a window
	// and is then cleared so the same link works again later.
	useEffect(() => {
		if (!agentId) return;
		updateStorage((prev) => ({
			windows: toFront(dropPreview(prev.windows), windowCentered(agentId)),
		}));
		void navigate("/agents/board", { replace: true });
	}, [agentId, navigate, updateStorage]);

	// Escape dismisses the preview, else the frontmost window; the board stays.
	useEffect(() => {
		if (windows.length === 0) return;
		const onKey = (e: KeyboardEvent) => {
			const target = e.target as HTMLElement | null;
			const typing =
				target?.tagName === "INPUT" ||
				target?.tagName === "TEXTAREA" ||
				target?.isContentEditable;
			if (e.key !== "Escape" || typing) return;
			setWindows((prev) =>
				prev.some((w) => !w.pinned) ? dropPreview(prev) : prev.slice(0, -1),
			);
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [windows]);

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
	const openChatIds = new Set(windows.map((w) => w.chatId));
	const allCards = buildCards(chats);
	const assistant = useCardAssistant(chats, allCards);
	// A window wears its card's color, so a window and its card read as one thing.
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
		clearPreviewTimer();
		if (preview) setWindows(dropPreview);
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

	const moveColumn = (
		name: string,
		target: DropTarget & { kind: "column" },
	) => {
		const names = columns.map((c) => c.name).filter((n) => n !== name);
		const index =
			names.indexOf(target.name) + (target.side === "after" ? 1 : 0);
		names.splice(index, 0, name);
		updateStorage({ columnOrder: names });
	};

	const handleDragEnd = ({ active, collisions }: DragEndEvent) => {
		setActiveDrag(null);
		setDropTarget(null);
		const drag = active.data.current as DragData | undefined;
		const target = targetOf(collisions);
		if (!drag || !target) return;
		if (target.kind === "column") {
			if (drag.type === "column") moveColumn(drag.name, target);
			return;
		}
		if (drag.type === "column") return;
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
			columnOrder: [...columns.map((c) => c.name), name],
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

	const openAssistant = async (card: BoardCardModel) => {
		const chatId = await assistant.open(card, findAssistant(card, chats));
		pinWindow(windowCentered(chatId));
	};

	const floating = (win: ChatWindow) => (
		<FloatingChat
			key={win.chatId}
			window={win}
			chat={chatsById.get(win.chatId)}
			color={cardColorByChatId.get(win.chatId)}
			onChange={changeWindow}
			onClose={() => closeWindow(win.chatId)}
			onInteract={() => interact(win.chatId)}
			onPreviewEnter={clearPreviewTimer}
			onPreviewLeave={endPreview}
		/>
	);

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<title>{pageTitle("Board", "Agents")}</title>
			<div className="flex h-12 shrink-0 items-center gap-3 border-b border-border pr-4 pl-3">
				<Button
					variant="subtle"
					size="icon"
					aria-label="Exit board"
					className="size-7 text-content-secondary"
					// Leaving lands on the chat in front, or the agents home.
					onClick={() => {
						const reading = windows.filter((w) => w.pinned).at(-1)?.chatId;
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
							onAssistant={(card) => void openAssistant(card)}
							onOpen={openChat}
							onPreview={previewChat}
							onPreviewEnd={endPreview}
							onAddNote={(card, text) => void mutations.addComment(card, text)}
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
			{windows.map(floating)}
		</div>
	);
};

export default ChatBoardPage;
