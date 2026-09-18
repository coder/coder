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
import { type FC, useEffect, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatSearch, createChat, updateChatTitle } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { useDebouncedValue } from "#/hooks/debounce";
import { pageTitle } from "#/utils/page";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";
import { buildChatSearchQuery } from "../../components/ChatsSidebar/dialogs/searchQuery";
import {
	type AssistantSpec,
	type AssistantTools,
	BOARD_ASSISTANT_KEY,
	boardAssistantSpec,
	cardAssistantSpec,
} from "./assistantSpecs";
import { assistantIds, openAssistant } from "./assistants";
import type { DragData } from "./BoardCard";
import { BoardColumns } from "./BoardColumns";
import { BoardHeader } from "./BoardHeader";
import { BoardWindows } from "./BoardWindows";
import { effortsOf, type Plan, renameEffort } from "./boardApi";
import { boardChats, updateChatLabels } from "./boardChats";
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
	type DraftTarget,
	readBoardStorage,
	saveBoardStorage,
} from "./boardStorage";
import { DragGhost } from "./DragGhost";
import { refetchChatListUntilLanded } from "./refreshChatList";
import { runPlan } from "./runPlan";
import {
	changeWindow,
	closeWindow,
	dismissTop,
	draftCreated,
	draftWindow,
	dropPreview,
	previewOf,
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
	const [storage, setStorage] = useState(readBoardStorage);
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS);
	const [activeDrag, setActiveDrag] = useState<DragData | null>(null);
	const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
	const [pendingPreview, setPendingPreview] = useState<PendingPreview | null>(
		null,
	);
	const { windows } = storage;

	// Column order and pinned windows survive a reload.
	useEffect(() => saveBoardStorage(storage), [storage]);

	useEffect(() => {
		if (!pendingPreview) return;
		const timer = setTimeout(
			() => {
				setPendingPreview(null);
				setStorage((prev) => ({
					...prev,
					windows:
						pendingPreview.kind === "open"
							? [
									...dropPreview(prev.windows),
									windowBeside(
										pendingPreview.chatId,
										pendingPreview.anchor,
										false,
									),
								]
							: dropPreview(prev.windows),
				}));
			},
			pendingPreview.kind === "open" ? PREVIEW_OPEN_MS : PREVIEW_CLOSE_MS,
		);
		return () => clearTimeout(timer);
	}, [pendingPreview]);

	const chatsQuery = useInfiniteQuery(boardChats());
	// Free text has to travel as a search:"..." token; the backend rejects
	// bare words. Same builder the search dialog uses.
	const searchQuery = useQuery({
		...chatSearch({
			q: `${buildChatSearchQuery([], debouncedSearch) ?? ""} archived:false`,
		}),
		enabled: debouncedSearch.length > 0,
	});
	const labelsMutation = useMutation({
		...updateChatLabels(queryClient),
		onError: (error: unknown) =>
			toast.error(getErrorMessage(error, "Failed to update chat labels.")),
	});
	const titleMutation = useMutation({
		...updateChatTitle(queryClient),
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

	const chats = chatsQuery.data?.pages[0] ?? [];
	const chatsById = new Map(chats.map((chat) => [chat.id, chat]));
	// Looked up in render: an unknown call taking `chats` inside the handler
	// would count as a mutation and cost the handler its memoization.
	const assistantByKey = assistantIds(chats);
	// The board list is filtered `archived:false` and the archive cache helper
	// drops an archived chat from it, so an archived assistant leaves this map
	// on its own.
	const cardAssistants = new Map(
		[...assistantByKey].flatMap(([key, id]) => {
			const assistant = chatsById.get(id);
			return key !== BOARD_ASSISTANT_KEY && assistant
				? [[key, assistant] as const]
				: [];
		}),
	);
	// Watch events carry status but not labels. While the board assistant is
	// on a turn it may relabel chats, so the turn ending refetches the list.
	// The effect's cleanup is that ending: it runs when the status leaves the
	// active set, or when the board unmounts mid-turn.
	const boardAssistantId = assistantByKey.get(BOARD_ASSISTANT_KEY);
	const boardAssistantActive = isActiveChatStatus(
		(boardAssistantId ? chatsById.get(boardAssistantId)?.status : null) ?? null,
	);
	useEffect(() => {
		if (!boardAssistantActive) return;
		return () => void refetchChatListUntilLanded(queryClient);
	}, [boardAssistantActive, queryClient]);

	// Updates are functional: a preview timer, a window gesture or the
	// assistant's request may commit after other windows changed. Defined
	// after the last hook so the compiler can memoize what depends on them.
	const updateStorage = (patch: Partial<BoardStorage>) =>
		setStorage((prev) => ({ ...prev, ...patch }));
	const setWindows = (
		next: (prev: readonly ChatWindow[]) => readonly ChatWindow[],
	) => setStorage((prev) => ({ ...prev, windows: next(prev.windows) }));

	const openChatIds = new Set(
		windows.flatMap((w) => (w.kind === "chat" ? [w.chatId] : [])),
	);
	const allCards = buildCards(chats);
	// Commands act on the full model; the filter only decides what is drawn,
	// so renaming a column with a filter active still relabels every card.
	const columns = buildColumns(
		allCards,
		storage.columnOrder,
		storage.emptyColumns,
	);
	const boardState = { cards: allCards, columns, storage };
	const efforts = effortsOf(allCards);
	// A stored filter whose last card lost the effort falls back to All; once
	// the list is loaded that is known for sure and the filter is cleared.
	const effortFilter = efforts.some((e) => e.name === storage.effortFilter)
		? storage.effortFilter
		: null;
	if (
		storage.effortFilter !== null &&
		effortFilter === null &&
		chatsQuery.data !== undefined
	) {
		updateStorage({ effortFilter: null });
	}
	const matchingIds =
		debouncedSearch && searchQuery.data
			? new Set(searchQuery.data.map((chat) => chat.id))
			: undefined;
	const visibleColumns = columns.map((column) => ({
		...column,
		cards: column.cards.filter(
			(card) =>
				(!matchingIds || card.members.some((m) => matchingIds.has(m.id))) &&
				(effortFilter === null || card.efforts.includes(effortFilter)),
		),
	}));
	const visibleCount = visibleColumns.reduce((n, c) => n + c.cards.length, 0);

	// A draft for a card that was merged away or removed has nowhere to land.
	// Adjusted in render, not in an effect, so no frame shows an orphan form.
	const draftTarget = windows.find((w) => w.kind === "draft")?.target;
	if (
		chatsQuery.data !== undefined &&
		draftTarget !== undefined &&
		"cardId" in draftTarget &&
		!allCards.some((c) => c.id === draftTarget.cardId)
	) {
		setWindows((prev) => closeWindow(prev, "draft"));
	}

	const run = (plan: Plan | null) =>
		runPlan(plan, {
			write: (chatId, labels) => labelsMutation.mutateAsync({ chatId, labels }),
			rename: (chatId, title) => titleMutation.mutateAsync({ chatId, title }),
			updateStorage,
		});

	const openChat = (chat: Chat, anchor: DOMRect) => {
		setPendingPreview(null);
		setWindows((prev) =>
			toFront(
				dropPreview(prev),
				prev.find((w) => w.kind === "chat" && w.chatId === chat.id) ??
					windowBeside(chat.id, anchor, true),
			),
		);
	};
	// The create form keeps one shared draft in localStorage, so a second
	// draft window would edit the first one's text: toFront replaces it.
	const openDraft = (target: DraftTarget) => {
		setPendingPreview(null);
		setWindows((prev) => toFront(dropPreview(prev), draftWindow(target)));
	};
	const previewChat = (chat: Chat, anchor: DOMRect) => {
		if (openChatIds.has(chat.id)) {
			setPendingPreview(null);
			return;
		}
		setPendingPreview({ kind: "open", chatId: chat.id, anchor });
	};
	const endPreview = () => setPendingPreview({ kind: "close" });

	// Opens the chat for the assistant labelled `key`, creating it on first use.
	const showAssistant = (
		key: string,
		spec: (tools: AssistantTools) => AssistantSpec,
	) =>
		openAssistant({
			spec,
			existingId: assistantByKey.get(key),
			create: createMutation.mutateAsync,
			rename: titleMutation.mutateAsync,
			queryClient,
		}).then((chatId) => {
			if (!chatId) return;
			setPendingPreview(null);
			setWindows((prev) => toFront(dropPreview(prev), windowCentered(chatId)));
		});
	const openCardAssistant = (card: BoardCardModel) =>
		void showAssistant(card.id, (tools) => cardAssistantSpec(card, tools));
	const openBoardAssistant = () => {
		const organizationId = chats[0]?.organization_id;
		if (!organizationId) return;
		void showAssistant(BOARD_ASSISTANT_KEY, (tools) =>
			boardAssistantSpec(boardState, organizationId, tools),
		);
	};

	const handleDragStart = ({ active }: DragStartEvent) => {
		setPendingPreview(null);
		if (previewOf(windows)) setWindows(dropPreview);
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
				chatCount={chats.length}
				cardCount={allCards.length}
				visibleCount={matchingIds ? visibleCount : undefined}
				search={search}
				onSearchChange={setSearch}
				// Leaving lands on the chat in front, or the agents home.
				onExit={() => {
					const reading = windows
						.flatMap((w) => (w.kind === "chat" && w.pinned ? [w.chatId] : []))
						.at(-1);
					void navigate(reading ? `/agents/${reading}` : "/agents");
				}}
				onAssistant={openBoardAssistant}
				efforts={efforts}
				effortFilter={effortFilter}
				onEffortFilter={(effort) => updateStorage({ effortFilter: effort })}
				onRenameEffort={(from, to) =>
					void run(renameEffort(boardState, from, to))
				}
			/>
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
				onDragCancel={clearDrag}
			>
				<BoardColumns
					columns={visibleColumns}
					board={boardState}
					run={run}
					assistants={cardAssistants}
					openChatIds={openChatIds}
					dropTarget={dropTarget}
					knownEfforts={efforts.map((e) => e.name)}
					onAssistant={openCardAssistant}
					onNewChat={openDraft}
					onFilterEffort={(effort) => updateStorage({ effortFilter: effort })}
					onOpen={openChat}
					onPreview={previewChat}
					onPreviewEnd={endPreview}
				/>
				{/* Portaled above every column so the moving card is never clipped. */}
				<DragOverlay dropAnimation={null}>
					{activeDrag && <DragGhost drag={activeDrag} />}
				</DragOverlay>
			</DndContext>
			<BoardWindows
				windows={windows}
				chatsById={chatsById}
				colorByChatId={cardColorByChat(allCards)}
				board={boardState}
				onChange={(next) => setWindows((prev) => changeWindow(prev, next))}
				onClose={(key) => setWindows((prev) => closeWindow(prev, key))}
				onRaise={(key) => {
					setPendingPreview(null);
					setWindows((prev) => raise(prev, key));
				}}
				onPreviewEnter={() => setPendingPreview(null)}
				onPreviewLeave={endPreview}
				onDismissTop={() => setWindows(dismissTop)}
				onDraftCreated={(target, chatId) =>
					setWindows((prev) => draftCreated(prev, target, chatId))
				}
				onCardAssistant={openCardAssistant}
			/>
		</div>
	);
};

export default ChatBoardPage;
