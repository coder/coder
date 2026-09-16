import {
	type CollisionDetection,
	DndContext,
	type DragEndEvent,
	DragOverlay,
	type DragStartEvent,
	KeyboardSensor,
	MouseSensor,
	pointerWithin,
	TouchSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import { ArrowLeftIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react";
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
import { Input } from "#/components/Input/Input";
import { useDebouncedValue } from "#/hooks/debounce";
import { pageTitle } from "#/utils/page";
import { buildChatSearchQuery } from "../ChatsSidebar/dialogs/searchQuery";
import { type DragData, DragGhost, type DropData } from "./BoardCard";
import { BoardColumn, NewColumn } from "./BoardColumn";
import { buildCards, buildColumns, INBOX_COLUMN } from "./boardLabels";
import { type ChatPane, useBoardStorage } from "./boardStorage";
import { ChatPanes, closeTab, openTab } from "./ChatPanes";
import { useBoardMutations } from "./useBoardMutations";

const hasOpenChats = (panes: readonly ChatPane[]) => panes.length > 0;

// Cards nest inside columns, so a plain pointerWithin would return both.
// A card under the pointer wins so drops merge; otherwise the column wins.
const boardCollision: CollisionDetection = (args) => {
	const hits = pointerWithin(args);
	const activeData = args.active.data.current as DragData | undefined;
	const card = hits.find((hit) => {
		const data = hit.data?.droppableContainer?.data.current as
			| DropData
			| undefined;
		if (data?.type !== "card") return false;
		// A card cannot be dropped on itself or on the card it came from.
		return data.card.id !== activeData?.card.id;
	});
	if (card) return [card];
	const column = hits.find(
		(hit) =>
			(hit.data?.droppableContainer?.data.current as DropData | undefined)
				?.type === "column",
	);
	return column ? [column] : [];
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
	const splitRef = useRef<HTMLDivElement>(null);
	const { panes, focusedPane } = storage;
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
		useSensor(MouseSensor, { activationConstraint: { distance: 4 } }),
		useSensor(TouchSensor, {
			activationConstraint: { delay: 150, tolerance: 5 },
		}),
		useSensor(KeyboardSensor),
	);
	const matchingIds =
		debouncedSearch && searchQuery.data
			? new Set(searchQuery.data.map((chat) => chat.id))
			: undefined;

	const chats = chatsQuery.data?.pages.flat() ?? [];
	const chatsById = new Map(chats.map((chat) => [chat.id, chat]));
	const openChatIds = new Set(panes.flatMap((pane) => pane.tabs));
	const allCards = buildCards(chats);
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

	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		setActiveDrag(null);
		const drag = active.data.current as DragData | undefined;
		const drop = over?.data.current as DropData | undefined;
		if (!drag || !drop) return;
		if (drop.type === "column") {
			if (drag.type === "card") {
				if (drag.card.column !== drop.name) {
					void mutations.moveCard(drag.card, drop.name);
				}
			} else {
				void mutations.detachChat(drag.chat, drop.name);
			}
			return;
		}
		if (drag.type === "card") {
			void mutations.mergeCards(drag.card, drop.card);
		} else if (drag.card.id !== drop.card.id) {
			void mutations.joinCard(drag.chat, drop.card);
		}
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
					flex: hasOpenChats(panes)
						? `0 0 ${storage.splitRatio * 100}%`
						: "1 1 0",
				}}
			>
				<div className="flex items-center gap-2 border-b border-border px-3 py-2">
					<Button
						variant="subtle"
						size="icon"
						aria-label="Exit board"
						// Leaving lands on the chat being read, or the agents home.
						onClick={() => {
							const reading = panes[focusedPane]?.active;
							void navigate(reading ? `/agents/${reading}` : "/agents");
						}}
					>
						<ArrowLeftIcon />
					</Button>
					<h1 className="m-0 text-base font-medium text-content-primary">
						Board
					</h1>
					<div className="relative ml-auto w-64">
						<SearchIcon className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-content-secondary" />
						<Input
							aria-label="Filter cards"
							placeholder="Filter cards"
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							className="h-8 pl-7 text-sm"
						/>
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
					onDragEnd={handleDragEnd}
					onDragCancel={() => setActiveDrag(null)}
				>
					<div className="flex min-h-0 flex-1 gap-3 overflow-x-auto p-3">
						{columns.map((column) => (
							<BoardColumn
								key={column.name}
								column={column}
								openChatIds={openChatIds}
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
							<Button
								variant="subtle"
								size="icon"
								aria-label="Add column"
								// Sits on the column header line, matching header height.
								className="mt-2 size-7 shrink-0 text-content-secondary"
								onClick={() => setAddingColumn(true)}
							>
								<PlusIcon />
							</Button>
						)}
					</div>
					{/* Portaled above every column so the moving card is never clipped. */}
					<DragOverlay dropAnimation={null}>
						{activeDrag && <DragGhost drag={activeDrag} />}
					</DragOverlay>
				</DndContext>
			</div>
			{hasOpenChats(panes) && (
				<>
					{/* Divider: drag to resize, X closes every open chat. */}
					<div
						className="flex h-7 shrink-0 cursor-row-resize touch-none select-none items-center justify-between border-y border-border bg-surface-secondary/60 px-3"
						onPointerDown={startResize}
					>
						<span className="text-xs text-content-secondary">
							{openChatIds.size === 1 ? "1 chat" : `${openChatIds.size} chats`}
						</span>
						<Button
							variant="subtle"
							size="icon"
							aria-label="Close all chats"
							className="size-6"
							onPointerDown={(e) => e.stopPropagation()}
							onClick={() => setPanes([], 0)}
						>
							<XIcon className="size-3.5" />
						</Button>
					</div>
					<ChatPanes
						panes={panes}
						focusedPane={focusedPane}
						chatsById={chatsById}
						onChange={setPanes}
					/>
				</>
			)}
		</div>
	);
};

export default ChatBoardPage;
