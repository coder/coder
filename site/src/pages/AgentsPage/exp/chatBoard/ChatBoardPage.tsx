import {
	type DragDropManager,
	DragDropProvider,
	DragOverlay,
} from "@dnd-kit/react";
import { cn } from "cn";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import {
	keepPreviousData,
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useNavigate } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { chatProjects } from "#/api/queries/chatProjects";
import {
	archiveChat,
	chatSearch,
	createChat,
	moveChatToProject,
	unarchiveChat,
	updateChatTitle,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { toast } from "#/components/Toaster/toast";
import { useDebouncedValue } from "#/hooks/debounce";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { chatFamilyAllowsArchive } from "../../components/ChatActionsMenuItems";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";
import { canDragChatToProject } from "../../components/ChatsSidebar/chats/projectDrag";
import { buildChatSearchQuery } from "../../components/ChatsSidebar/dialogs/searchQuery";
import {
	type AssistantSpec,
	type AssistantTools,
	BOARD_ASSISTANT_KEY,
	boardAssistantSpec,
	cardAssistantSpec,
	laneAssistantKey,
	laneAssistantSpec,
} from "./assistantSpecs";
import { assistantIds, findOrCreateAssistant } from "./assistants";
import type { DragData } from "./BoardCard";
import { BoardColumns } from "./BoardColumns";
import { BoardHeader } from "./BoardHeader";
import { BoardLane, type LaneProject } from "./BoardLane";
import { BoardWindows } from "./BoardWindows";
import {
	cardProjectIds,
	effortCounts,
	moveCardToProject,
	type Plan,
	removeFromGroup,
	renameEffort,
} from "./boardApi";
import {
	boardChats,
	boardWriteScope,
	isBoardWriting,
	updateChatLabels,
} from "./boardChats";
import {
	type DropTarget,
	dragCanChangeLane,
	dragDataOf,
	dragPlugins,
	dragSensors,
	dragTargetOf,
	dropCommand,
} from "./boardDrag";
import {
	BOARD_LANE,
	type BoardCard as BoardCardModel,
	buildCards,
	buildColumns,
	cardColorByChat,
	laneOf,
	NO_PROJECT_LANE,
} from "./boardLabels";
import {
	type BoardStorage,
	type ChatWindow,
	type DraftTarget,
	readBoardStorage,
	saveBoardStorage,
} from "./boardStorage";
import { DragGhost } from "./DragGhost";
import { ProjectRepair } from "./ProjectRepair";
import { refetchChatListUntilLanded } from "./refreshChatList";
import { runPlan } from "./runPlan";
import {
	changeWindow,
	closeWindow,
	dismissTop,
	draftWindow,
	dropPreview,
	minimizeWindow,
	raise,
	replaceDraftWithChat,
	toFront,
	windowBeside,
	windowCentered,
} from "./windows";

// Hover must be deliberate before a full chat mounts; leaving gives the
// pointer time to cross into the window.
const PREVIEW_OPEN_MS = 450;
const PREVIEW_CLOSE_MS = 300;

const SEARCH_DEBOUNCE_MS = 300;

// Matches the board's other undo toasts.
const ARCHIVE_UNDO_MS = 10_000;

/** A preview change waiting for its delay: open beside an anchor, or close. */
type PendingPreview =
	| { kind: "open"; chatId: string; anchor: DOMRect }
	| { kind: "close" };

const ChatBoardPage: React.FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
	const [storage, setStorage] = useState(() => readBoardStorage(user.id));
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS);
	const [activeDrag, setActiveDrag] = useState<DragData | null>(null);
	const projectsQuery = useQuery(chatProjects());
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
		...updateChatTitle(queryClient),
		scope: boardWriteScope,
		onError: (error: unknown) =>
			toast.error(getErrorMessage(error, "Failed to rename chat.")),
	});
	const createMutation = useMutation(createChat(queryClient));
	const archiveOptions = archiveChat(queryClient);
	const archiveMutation = useMutation({
		...archiveOptions,
		onError: (error, chatId, context) => {
			archiveOptions.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to archive chat."));
		},
	});
	const unarchiveOptions = unarchiveChat(queryClient);
	const unarchiveMutation = useMutation({
		...unarchiveOptions,
		onError: (error, chatId, context) => {
			unarchiveOptions.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to unarchive chat."));
		},
	});
	const projectMoveOptions = moveChatToProject(queryClient);
	const projectMutation = useMutation({
		...projectMoveOptions,
		onError: (error, variables, context) => {
			projectMoveOptions.onError(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to move chat to project."));
		},
	});

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
			return key !== BOARD_ASSISTANT_KEY &&
				!key.startsWith(laneAssistantKey("")) &&
				assistant
				? [[key, assistant] as const]
				: [];
		}),
	);
	// Watch payloads carry labels, but mergeWatchedChatSummary drops them, and
	// label PATCHes publish no event. Every assistant can write labels (the
	// board one relabels chats, a card one writes notes), so the list is
	// refetched whenever the set of working assistants changes, which
	// includes one ending its turn.
	const activeAssistantIds = [...assistantByKey.values()]
		.filter((id) => isActiveChatStatus(chatsById.get(id)?.status ?? null))
		.sort()
		.join(",");
	useEffect(() => {
		if (!activeAssistantIds) return;
		return () => void refetchChatListUntilLanded(queryClient);
	}, [activeAssistantIds, queryClient]);

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
	const efforts = effortCounts(allCards);
	const effortFilter = efforts.some((e) => e.name === storage.effortFilter)
		? storage.effortFilter
		: null;
	// Skipped while a plan writes, like the column order save below: the plan
	// saves storage before its label patch returns, so a renamed effort would
	// read as missing and be cleared.
	if (
		storage.effortFilter !== null &&
		effortFilter === null &&
		chatsQuery.data !== undefined &&
		!isBoardWriting(queryClient)
	) {
		updateStorage({ effortFilter: null });
	}
	// A column first seen in the labels is saved at the end of the order.
	// Unsaved columns follow their newest card, so a move would reorder them.
	const unsavedColumns = columns
		.map((column) => column.name)
		.filter((name) => !storage.columnOrder.includes(name));
	if (
		chatsQuery.data !== undefined &&
		unsavedColumns.length > 0 &&
		!isBoardWriting(queryClient)
	) {
		updateStorage({
			columnOrder: [...storage.columnOrder, ...unsavedColumns],
		});
	}
	// An active search must not fall back to unfiltered cards when results
	// are unavailable; the body below shows the loading or error state then.
	const matchingIds = searchActive
		? new Set((searchQuery.data ?? []).map((chat) => chat.id))
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
	// Every project I can use gets a lane, so a card can be dragged into an
	// empty one. A card whose project is not in the list keeps a lane of its
	// own, so no card is ever hidden.
	const projects = projectsQuery.data ?? [];
	const unknownProjects = [
		...new Set(
			allCards
				.map(laneOf)
				.filter(
					(key) =>
						key !== NO_PROJECT_LANE && !projects.some((p) => p.id === key),
				),
		),
	];
	const lanes =
		projects.length > 0
			? [
					...projects.map((project) => ({
						key: project.id,
						name: project.name,
						project,
					})),
					...unknownProjects.map((key) => ({
						key,
						name: "Unknown project",
						project: undefined,
					})),
					{ key: NO_PROJECT_LANE, name: "No project", project: undefined },
				]
			: undefined;
	// What a chat started in a lane is created in: the lane's project, or null
	// for none. Undefined without lanes, where the composer's picker decides.
	const laneProject = (key: string): LaneProject | null | undefined => {
		const lane = lanes?.find((l) => l.key === key);
		if (!lane) return undefined;
		if (key === NO_PROJECT_LANE) return null;
		const organizationId =
			lane.project?.organization_id ??
			allCards.find((card) => laneOf(card) === key)?.primary.organization_id;
		return organizationId === undefined
			? undefined
			: { id: key, organization_id: organizationId, name: lane.name };
	};
	const toggleLane = (key: string) =>
		updateStorage({
			collapsedLanes: storage.collapsedLanes.includes(key)
				? storage.collapsedLanes.filter((k) => k !== key)
				: [...storage.collapsedLanes, key],
		});

	// A draft whose card was merged away or removed, or whose column was
	// renamed or deleted, has nowhere to land; submitting it would recreate
	// the old column. Adjusted in render, not in an effect, so no frame shows
	// an orphan form.
	const draftTarget = windows.find((w) => w.kind === "draft")?.target;
	if (
		chatsQuery.data !== undefined &&
		draftTarget !== undefined &&
		("cardId" in draftTarget
			? !allCards.some((c) => c.id === draftTarget.cardId)
			: !columns.some((c) => c.name === draftTarget.column))
	) {
		setWindows((prev) => prev.filter((w) => w.kind !== "draft"));
	}

	const run = (plan: Plan | null) =>
		runPlan(plan, {
			write: (chatId, labels, after) =>
				labelsMutation.mutateAsync({ chatId, labels, after }),
			rename: (chatId, title) => titleMutation.mutateAsync({ chatId, title }),
			moveProject: (chatId, projectId) =>
				projectMutation.mutateAsync({ chatId, projectId }),
			updateStorage,
		});

	const openChat = (chat: Chat, anchor: DOMRect) => {
		setPendingPreview(null);
		setWindows((prev) =>
			toFront(
				dropPreview(prev),
				prev.find((w) => w.kind === "chat" && w.chatId === chat.id) ??
					windowBeside(chat.id, anchor, { pinned: true }),
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

	const showAssistant = (
		key: string,
		spec: (tools: AssistantTools) => AssistantSpec,
	) =>
		findOrCreateAssistant({
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
	const archiveChatsAndOfferUndo = async (
		chats: readonly Chat[],
		message: string,
	) => {
		const results = await Promise.allSettled(
			chats.map((chat) => archiveMutation.mutateAsync(chat.id)),
		);
		const archived = chats.filter((_, i) => results[i]?.status === "fulfilled");
		if (archived.length === 0) return;
		// Typed, so the shared Toaster draws an icon and puts Undo under the text.
		toast.success(message, {
			duration: ARCHIVE_UNDO_MS,
			action: {
				label: "Undo",
				onClick: () => {
					for (const chat of archived) unarchiveMutation.mutate(chat.id);
				},
			},
		});
	};
	const archiveCard = (card: BoardCardModel) => {
		const assistant = cardAssistants.get(card.id);
		void archiveChatsAndOfferUndo(
			[...card.members, ...(assistant ? [assistant] : [])],
			`Archived "${card.title}"`,
		);
	};
	// A card with a working chat is left whole rather than half archived.
	const archiveCards = (cards: readonly BoardCardModel[]) => {
		const chatsOf = (card: BoardCardModel) => {
			const assistant = cardAssistants.get(card.id);
			return [...card.members, ...(assistant ? [assistant] : [])];
		};
		const ready = cards.filter((card) =>
			chatsOf(card).every((chat) =>
				chatFamilyAllowsArchive(chat.status, chat.children),
			),
		);
		const skipped = cards.length - ready.length;
		if (ready.length === 0) {
			toast.error("Every card here has a working chat.");
			return;
		}
		const count = ready.length === 1 ? "1 card" : `${ready.length} cards`;
		void archiveChatsAndOfferUndo(
			ready.flatMap(chatsOf),
			skipped > 0
				? `Archived ${count}, ${skipped} with a working chat left`
				: `Archived ${count}`,
		);
	};
	// A group's card data lives on its primary, so the chat first leaves the
	// group, which hands that data to the next chat, and is archived after.
	const archiveCardChat = async (chat: Chat, card: BoardCardModel) => {
		if (card.members.length > 1) {
			const plan = removeFromGroup(boardState, chat.id);
			if (plan && !(await run({ ...plan, undo: undefined }))) return;
		}
		await archiveChatsAndOfferUndo([chat], `Archived "${chat.title}"`);
	};
	const openCardAssistant = (card: BoardCardModel) =>
		void showAssistant(card.id, (tools) => cardAssistantSpec(card, tools));
	const openLaneAssistant = (lane: Readonly<{ key: string; name: string }>) => {
		const inLane = (card: BoardCardModel) => laneOf(card) === lane.key;
		const project = laneProject(lane.key);
		const organizationId =
			project?.organization_id ?? chats[0]?.organization_id;
		if (!organizationId) return;
		void showAssistant(laneAssistantKey(lane.key), (tools) =>
			laneAssistantSpec(
				boardState,
				{
					key: lane.key,
					name: lane.name,
					projectId: project?.id,
					organizationId,
				},
				inLane,
				tools,
			),
		);
	};
	const openBoardAssistant = () => {
		const organizationId = chats[0]?.organization_id;
		if (!organizationId) return;
		void showAssistant(BOARD_ASSISTANT_KEY, (tools) =>
			boardAssistantSpec(boardState, organizationId, tools),
		);
	};

	const dragManagerRef = useRef<DragDropManager | null>(null);
	const assistantOfCard = (cardId: string) => cardAssistants.get(cardId);
	const handleDragStart = (_event: unknown, manager: DragDropManager) => {
		dragManagerRef.current = manager;
		setPendingPreview(null);
		setWindows((prev) =>
			prev.some((w) => !w.pinned) ? dropPreview(prev) : prev,
		);
		setActiveDrag(dragDataOf(manager) ?? null);
	};
	// Collisions are recomputed on every move, and the zone within one card
	// (insert above, merge, insert below) changes without the target changing.
	const handleCollision = (_event: unknown, manager: DragDropManager) =>
		setDropTarget(dragTargetOf(manager, user.id, assistantOfCard));
	const handleDragEnd = (
		{ canceled }: { canceled: boolean },
		manager: DragDropManager,
	) => {
		dragManagerRef.current = null;
		setActiveDrag(null);
		setDropTarget(null);
		const drag = dragDataOf(manager);
		const target = canceled
			? null
			: dragTargetOf(manager, user.id, assistantOfCard);
		const command =
			drag && target && dropCommand(drag, target, assistantOfCard);
		if (command) void run(command(boardState));
	};
	// dnd-kit skips the collision check while the pointer stands still, so a
	// scroll under it, by wheel or auto-scroll, resolves the target here.
	// An Effect Event, so a scroll resolves against the current assistants.
	const retarget = useEffectEvent(() => {
		const manager = dragManagerRef.current;
		if (manager) setDropTarget(dragTargetOf(manager, user.id, assistantOfCard));
	});
	useEffect(() => {
		if (!activeDrag) return;
		let frame: number | undefined;
		const update = () => {
			frame = undefined;
			retarget();
		};
		const handleScroll = () => {
			// Not ??=: the React Compiler cannot lower it and skips the file.
			if (frame === undefined) frame = requestAnimationFrame(update);
		};
		const options = { capture: true, passive: true };
		document.addEventListener("scroll", handleScroll, options);
		return () => {
			document.removeEventListener("scroll", handleScroll, options);
			if (frame !== undefined) cancelAnimationFrame(frame);
		};
	}, [activeDrag]);

	// The board is replaced only while a query has nothing to show. A failed
	// refetch keeps its data, and the board with it, so open note editors
	// are not unmounted by a background request; the failure is shown inline.
	let body: React.ReactNode;
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
			<DragDropProvider
				sensors={dragSensors}
				plugins={dragPlugins}
				onDragStart={handleDragStart}
				onCollision={handleCollision}
				onDragEnd={handleDragEnd}
			>
				{lanes ? (
					<div className="flex min-h-0 flex-1 flex-col overflow-auto bg-surface-secondary">
						{lanes.map((lane, index) => {
							const laneColumns = visibleColumns.map((column) => ({
								...column,
								cards: column.cards.filter((card) => laneOf(card) === lane.key),
							}));
							return (
								<BoardLane
									key={lane.key}
									name={lane.name}
									project={lane.project}
									cardCount={laneColumns.reduce(
										(n, c) => n + c.cards.length,
										0,
									)}
									collapsed={storage.collapsedLanes.includes(lane.key)}
									onToggle={() => toggleLane(lane.key)}
									assistant={chatsById.get(
										assistantByKey.get(laneAssistantKey(lane.key)) ?? "",
									)}
									onAssistant={() => openLaneAssistant(lane)}
									onPreview={previewChat}
									onPreviewEnd={endPreview}
								>
									<BoardColumns
										columns={laneColumns}
										lane={lane.key}
										layout="lane"
										showAddColumn={index === 0}
										board={boardState}
										run={run}
										assistants={cardAssistants}
										openChatIds={openChatIds}
										dropTarget={dropTarget}
										knownEfforts={efforts.map((e) => e.name)}
										onCardAssistant={openCardAssistant}
										onNewChat={openDraft}
										onFilterEffort={(effort) =>
											updateStorage({ effortFilter: effort })
										}
										onArchiveCard={archiveCard}
										onArchiveCards={archiveCards}
										onArchiveChat={archiveCardChat}
										onOpen={openChat}
										onPreview={previewChat}
										onPreviewEnd={endPreview}
									/>
								</BoardLane>
							);
						})}
					</div>
				) : (
					<BoardColumns
						columns={visibleColumns}
						lane={BOARD_LANE}
						layout="board"
						showAddColumn
						board={boardState}
						run={run}
						assistants={cardAssistants}
						openChatIds={openChatIds}
						dropTarget={dropTarget}
						knownEfforts={efforts.map((e) => e.name)}
						onCardAssistant={openCardAssistant}
						onNewChat={openDraft}
						onFilterEffort={(effort) => updateStorage({ effortFilter: effort })}
						onArchiveCard={archiveCard}
						onArchiveCards={archiveCards}
						onArchiveChat={archiveCardChat}
						onOpen={openChat}
						onPreview={previewChat}
						onPreviewEnd={endPreview}
					/>
				)}
				{/* Portaled above every column so the moving card is never clipped. */}
				<DragOverlay dropAnimation={null}>
					{activeDrag && (
						<DragGhost
							drag={activeDrag}
							hint={
								lanes &&
								!dragCanChangeLane(activeDrag, user.id, assistantOfCard)
									? "A working chat keeps this in its project"
									: undefined
							}
						/>
					)}
				</DragOverlay>
			</DragDropProvider>
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
					const reading = windows
						.flatMap((w) =>
							w.kind === "chat" && w.pinned && !w.minimized ? [w.chatId] : [],
						)
						.at(-1);
					void navigate(reading ? `/agents/${reading}` : "/agents");
				}}
				onBoardAssistant={openBoardAssistant}
				effortCounts={efforts}
				effortFilter={effortFilter}
				onFilterEffort={(effort) => updateStorage({ effortFilter: effort })}
				projectRepair={
					<ProjectRepair
						cards={allCards.filter((card) => cardProjectIds(card).length > 1)}
						optionsOf={(card) =>
							cardProjectIds(card).map((id) => ({
								id,
								name:
									id === null
										? "No project"
										: (projects.find((p) => p.id === id)?.name ??
											"Unknown project"),
							}))
						}
						blockedReason={(card, projectId) => {
							const assistant = cardAssistants.get(card.id);
							const stuck = [...card.members, ...(assistant ? [assistant] : [])]
								.filter(
									(chat) =>
										!chat.parent_chat_id &&
										(chat.project_id ?? null) !== projectId,
								)
								.find((chat) => !canDragChatToProject(chat, user.id));
							return stuck && `"${stuck.title}" is working or cannot be moved`;
						}}
						onMove={(card, option) =>
							void run(
								moveCardToProject(
									card,
									option.id,
									option.name,
									cardAssistants.get(card.id),
								),
							)
						}
					/>
				}
				onRenameEffort={(from, to) =>
					void run(renameEffort(boardState, from, to))
				}
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
				projectOf={laneProject}
				board={boardState}
				onChange={(next) => setWindows((prev) => changeWindow(prev, next))}
				onClose={(key) => setWindows((prev) => closeWindow(prev, key))}
				onMinimize={(key) => setWindows((prev) => minimizeWindow(prev, key))}
				onRestore={(key) => {
					setPendingPreview(null);
					setWindows((prev) => raise(dropPreview(prev), key));
				}}
				onRaise={(key) => {
					setPendingPreview(null);
					setWindows((prev) => raise(prev, key));
				}}
				onPreviewEnter={() => setPendingPreview(null)}
				onPreviewLeave={endPreview}
				onDismissTop={() => setWindows(dismissTop)}
				onDraftCreated={(target, chatId) =>
					setWindows((prev) => replaceDraftWithChat(prev, target, chatId))
				}
				onCardAssistant={openCardAssistant}
			/>
		</div>
	);
};

export default ChatBoardPage;
