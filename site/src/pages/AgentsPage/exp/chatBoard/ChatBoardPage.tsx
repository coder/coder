import {
	DndContext,
	type DragEndEvent,
	type DragMoveEvent,
	DragOverlay,
	type DragStartEvent,
	KeyboardSensor,
	PointerSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import { cn } from "cn";
import { type FC, type ReactNode, useEffect, useState } from "react";
import {
	keepPreviousData,
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatSearch, createChat } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useDebouncedValue } from "#/hooks/debounce";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { buildChatSearchQuery } from "../../components/ChatsSidebar/dialogs/searchQuery";
import type { DragData } from "./BoardCard";
import { BoardColumns } from "./BoardColumns";
import { BoardHeader } from "./BoardHeader";
import { BoardWindows } from "./BoardWindows";
import type { Plan } from "./boardApi";
import {
	boardChats,
	updateBoardChatTitle,
	updateChatLabels,
} from "./boardChats";
import {
	boardCollision,
	type DropTarget,
	dragDataOf,
	dropCommand,
	targetOf,
} from "./boardDrag";
import {
	type BoardCard as BoardCardModel,
	buildCards,
	buildColumns,
	cardColorByChat,
} from "./boardLabels";
import {
	type BoardStorage,
	type ChatWindow,
	readBoardStorage,
	saveBoardStorage,
} from "./boardStorage";
import { assistantIds, openCardAssistant } from "./cardAssistant";
import { DragGhost } from "./DragGhost";
import { runPlan } from "./runPlan";
import {
	changeWindow,
	dismissTop,
	dropPreview,
	raise,
	toFront,
	windowBeside,
	windowCentered,
} from "./windows";

// Hover must be deliberate before a full chat mounts; leaving gives the
// pointer time to cross into the window.
const PREVIEW_OPEN_MS = 450;
const PREVIEW_CLOSE_MS = 300;

const SEARCH_DEBOUNCE_MS = 300;

// Small enough that a drag starts promptly, large enough that a click on a
// card header does not.
const DRAG_ACTIVATION_PX = 4;

/** A preview change waiting for its delay: open beside an anchor, or close. */
type PendingPreview =
	| { kind: "open"; chatId: string; anchor: DOMRect }
	| { kind: "close" };

const ChatBoardPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
	const [storage, setStorage] = useState(() => readBoardStorage(user.id));
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS);
	const [activeDrag, setActiveDrag] = useState<DragData | null>(null);
	const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
	const [pendingPreview, setPendingPreview] = useState<PendingPreview | null>(
		null,
	);
	const { windows } = storage;

	// Column order and pinned windows survive a reload.
	useEffect(() => saveBoardStorage(user.id, storage), [user.id, storage]);

	useEffect(() => {
		if (!pendingPreview) return;
		const timer = setTimeout(
			() => {
				setPendingPreview(null);
				setStorage((prev) => {
					// Either way the old preview goes; opening adds the new one.
					const windows = dropPreview(prev.windows);
					if (pendingPreview.kind !== "open") return { ...prev, windows };
					const preview = windowBeside(
						pendingPreview.chatId,
						pendingPreview.anchor,
						{ pinned: false },
					);
					return { ...prev, windows: [...windows, preview] };
				});
			},
			pendingPreview.kind === "open" ? PREVIEW_OPEN_MS : PREVIEW_CLOSE_MS,
		);
		return () => clearTimeout(timer);
	}, [pendingPreview]);

	const chatsQuery = useInfiniteQuery(boardChats());
	const searchActive = debouncedSearch.length > 0;
	// Free text has to travel as a search:"..." token; the backend rejects
	// bare words. Same builder the search dialog uses.
	const searchQuery = useQuery({
		...chatSearch({
			q: `${buildChatSearchQuery([], debouncedSearch) ?? ""} archived:false`,
		}),
		enabled: searchActive,
		placeholderData: keepPreviousData,
	});
	const labelsMutation = useMutation({
		...updateChatLabels(queryClient),
		onError: (error: unknown) =>
			toast.error(getErrorMessage(error, "Failed to update chat labels.")),
	});
	const titleMutation = useMutation({
		...updateBoardChatTitle(queryClient),
		onError: (error: unknown) =>
			toast.error(getErrorMessage(error, "Failed to rename chat.")),
	});
	const createMutation = useMutation(createChat(queryClient));
	const sensors = useSensors(
		useSensor(PointerSensor, {
			activationConstraint: { distance: DRAG_ACTIVATION_PX },
		}),
		useSensor(KeyboardSensor),
	);

	// Updates are functional: a preview timer, a window gesture or the
	// assistant's request may commit after other windows changed. Defined
	// after the last hook so the compiler can memoize what depends on them.
	const updateStorage = (patch: Partial<BoardStorage>) =>
		setStorage((prev) => ({ ...prev, ...patch }));
	const setWindows = (
		next: (prev: readonly ChatWindow[]) => readonly ChatWindow[],
	) => setStorage((prev) => ({ ...prev, windows: next(prev.windows) }));

	const chats = chatsQuery.data?.pages[0] ?? [];
	const chatsById = new Map(chats.map((chat) => [chat.id, chat]));
	const openChatIds = new Set(windows.map((w) => w.chatId));
	const allCards = buildCards(chats);
	// Looked up in render: an unknown call taking `chats` inside the handler
	// would count as a mutation and cost the handler its memoization.
	const assistantByCard = assistantIds(chats);
	// Commands act on the full model; the filter only decides what is drawn,
	// so renaming a column with a filter active still relabels every card.
	const columns = buildColumns(
		allCards,
		storage.columnOrder,
		storage.emptyColumns,
	);
	const boardState = { cards: allCards, columns, storage };
	// An active search must not fall back to unfiltered cards when results
	// are unavailable; the body below shows the loading or error state then.
	const matchingIds = searchActive
		? new Set((searchQuery.data ?? []).map((chat) => chat.id))
		: undefined;
	const visibleColumns = columns.map((column) => ({
		...column,
		cards: column.cards.filter(
			(card) => !matchingIds || card.members.some((m) => matchingIds.has(m.id)),
		),
	}));
	const visibleCount = visibleColumns.reduce((n, c) => n + c.cards.length, 0);

	const run = (plan: Plan | null) =>
		runPlan(plan, {
			write: (chatId, labels, after) =>
				labelsMutation.mutateAsync({ chatId, labels, after }),
			rename: (chatId, title) => titleMutation.mutateAsync({ chatId, title }),
			updateStorage,
		});

	const openChat = (chat: Chat, anchor: DOMRect) => {
		setPendingPreview(null);
		setWindows((prev) =>
			toFront(
				dropPreview(prev),
				prev.find((w) => w.chatId === chat.id) ??
					windowBeside(chat.id, anchor, { pinned: true }),
			),
		);
	};
	const previewChat = (chat: Chat, anchor: DOMRect) => {
		if (openChatIds.has(chat.id)) {
			setPendingPreview(null);
			return;
		}
		setPendingPreview({ kind: "open", chatId: chat.id, anchor });
	};
	const endPreview = () => setPendingPreview({ kind: "close" });

	const openAssistant = (card: BoardCardModel) =>
		openCardAssistant({
			card,
			existingId: assistantByCard.get(card.id),
			create: createMutation.mutateAsync,
			rename: titleMutation.mutateAsync,
			queryClient,
		}).then((chatId) => {
			if (!chatId) return;
			setPendingPreview(null);
			setWindows((prev) => toFront(dropPreview(prev), windowCentered(chatId)));
		});

	const handleDragStart = ({ active }: DragStartEvent) => {
		setPendingPreview(null);
		setWindows((prev) =>
			prev.some((w) => !w.pinned) ? dropPreview(prev) : prev,
		);
		setActiveDrag(dragDataOf(active) ?? null);
	};
	// onDragOver only fires when the droppable id changes, but the zone within
	// one card (insert above, merge, insert below) changes without that.
	const handleDragMove = ({ collisions }: DragMoveEvent) =>
		setDropTarget(targetOf(collisions));
	const clearDrag = () => {
		setActiveDrag(null);
		setDropTarget(null);
	};
	const handleDragEnd = ({ active, collisions }: DragEndEvent) => {
		clearDrag();
		const drag = dragDataOf(active);
		const target = targetOf(collisions);
		const command = drag && target && dropCommand(drag, target);
		if (command) void run(command(boardState));
	};

	// The board is replaced only while a query has nothing to show. A failed
	// refetch keeps its data, and the board with it, so open note editors
	// are not unmounted by a background request; the failure is shown inline.
	let body: ReactNode;
	if (chatsQuery.data === undefined) {
		body = chatsQuery.isError ? (
			<ErrorAlert error={chatsQuery.error} />
		) : (
			<Loader />
		);
	} else if (searchActive && searchQuery.data === undefined) {
		body = searchQuery.isError ? (
			<ErrorAlert error={searchQuery.error} />
		) : (
			<Loader />
		);
	} else {
		body = (
			<DndContext
				sensors={sensors}
				collisionDetection={boardCollision}
				onDragStart={handleDragStart}
				onDragMove={handleDragMove}
				onDragEnd={handleDragEnd}
				onDragCancel={clearDrag}
			>
				<BoardColumns
					columns={visibleColumns}
					board={boardState}
					run={run}
					openChatIds={openChatIds}
					dropTarget={dropTarget}
					onAssistant={(card) => void openAssistant(card)}
					onOpen={openChat}
					onPreview={previewChat}
					onPreviewEnd={endPreview}
				/>
				{/* Portaled above every column so the moving card is never clipped. */}
				<DragOverlay dropAnimation={null}>
					{activeDrag && <DragGhost drag={activeDrag} />}
				</DragOverlay>
			</DndContext>
		);
	}

	return (
		// No text may be selected while something is dragged over the board.
		<div
			className={cn(
				"flex min-h-0 flex-1 flex-col",
				activeDrag && "select-none",
			)}
		>
			<title>{pageTitle("Board", "Agents")}</title>
			<BoardHeader
				chatCount={chatsQuery.data ? chats.length : undefined}
				cardCount={chatsQuery.data ? allCards.length : undefined}
				visibleCount={
					searchActive && searchQuery.data ? visibleCount : undefined
				}
				search={search}
				onSearchChange={setSearch}
				// Leaving lands on the chat in front, or the agents home.
				onExit={() => {
					const reading = windows.filter((w) => w.pinned).at(-1)?.chatId;
					void navigate(reading ? `/agents/${reading}` : "/agents");
				}}
			/>
			{chatsQuery.data && chatsQuery.isError && (
				<p className="m-0 px-3 py-2 text-sm text-content-destructive">
					Failed to load chats.
				</p>
			)}
			{searchActive && searchQuery.data && searchQuery.isError && (
				<p className="m-0 px-3 py-2 text-sm text-content-destructive">
					Search failed; showing the last results.
				</p>
			)}
			{body}
			<BoardWindows
				windows={windows}
				chatsById={chatsById}
				colorByChatId={cardColorByChat(allCards)}
				onChange={(next) => setWindows((prev) => changeWindow(prev, next))}
				onClose={(chatId) =>
					setWindows((prev) => prev.filter((w) => w.chatId !== chatId))
				}
				onRaise={(chatId) => {
					setPendingPreview(null);
					setWindows((prev) => raise(prev, chatId));
				}}
				onPreviewEnter={() => setPendingPreview(null)}
				onPreviewLeave={endPreview}
				onDismissTop={() => setWindows(dismissTop)}
			/>
		</div>
	);
};

export default ChatBoardPage;
