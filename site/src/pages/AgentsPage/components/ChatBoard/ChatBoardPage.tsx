import {
	type CollisionDetection,
	DndContext,
	type DragEndEvent,
	KeyboardSensor,
	MouseSensor,
	pointerWithin,
	TouchSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import { cn } from "cn";
import { ArrowLeftIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react";
import { type FC, lazy, Suspense, useEffect, useState } from "react";
import { useInfiniteQuery, useQuery } from "react-query";
import { useNavigate, useParams } from "react-router";
import { chatSearch, infiniteChats } from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { useDebouncedValue } from "#/hooks/debounce";
import { pageTitle } from "#/utils/page";
import { AgentChatPageSkeleton } from "../AgentsSkeletons";
import type { DragData, DropData } from "./BoardCard";
import { BoardColumn } from "./BoardColumn";
import { buildCards, buildColumns, INBOX_COLUMN } from "./boardLabels";
import { useBoardStorage } from "./boardStorage";
import { InlineInput } from "./InlineText";
import { useBoardMutations } from "./useBoardMutations";

const AgentChatPage = lazy(() => import("../../AgentChatPage"));

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

	// Same query the sidebar uses, so both views share one cache.
	const chatsQuery = useInfiniteQuery(infiniteChats({}));
	const { hasNextPage, isFetchingNextPage, fetchNextPage } = chatsQuery;
	// The board partitions every chat into columns, so it needs the full list.
	useEffect(() => {
		if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
	}, [hasNextPage, isFetchingNextPage, fetchNextPage]);

	const searchQuery = useQuery({
		...chatSearch({ q: `${debouncedSearch} archived:false` }),
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

	const handleDragEnd = ({ active, over }: DragEndEvent) => {
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

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<title>{pageTitle("Board", "Agents")}</title>
			<div
				className="flex min-h-0 flex-col"
				style={{ flex: agentId ? `0 0 ${storage.splitRatio * 100}%` : "1 1 0" }}
			>
				<div className="flex items-center gap-2 border-b border-border px-3 py-2">
					<Button
						variant="subtle"
						size="icon"
						aria-label="Exit board"
						// Leaving lands on the chat being read, or the agents home.
						onClick={() =>
							void navigate(agentId ? `/agents/${agentId}` : "/agents")
						}
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
					{agentId && (
						<Button
							variant="subtle"
							size="icon"
							aria-label="Close chat"
							onClick={() => void navigate("/agents/board")}
						>
							<XIcon />
						</Button>
					)}
				</div>
				{chatsQuery.isError && (
					<p className="m-0 px-3 py-2 text-sm text-content-destructive">
						Failed to load chats.
					</p>
				)}
				<DndContext
					sensors={sensors}
					collisionDetection={boardCollision}
					onDragEnd={handleDragEnd}
				>
					<div className="flex min-h-0 flex-1 gap-3 overflow-x-auto p-3">
						{columns.map((column) => (
							<BoardColumn
								key={column.name}
								column={column}
								activeChatId={agentId}
								onRename={(to) => renameColumn(column.name, to)}
								onDelete={() => deleteColumn(column.name)}
								onSetCardTitle={(card, title) =>
									void mutations.setCardTitle(card, title)
								}
								onRenameChat={(chat, title) =>
									void mutations.renameChat(chat, title)
								}
								onAddComment={(card, text) =>
									void mutations.addComment(card, text)
								}
								onRemoveComment={(card, index) =>
									void mutations.removeComment(card, index)
								}
							/>
						))}
						<div className="w-64 shrink-0">
							{addingColumn ? (
								<InlineInput
									value=""
									ariaLabel="New column name"
									className="w-full px-2 py-1 text-sm"
									onSave={addColumn}
									onDone={() => setAddingColumn(false)}
								/>
							) : (
								<Button
									variant="outline"
									className="w-full justify-start"
									onClick={() => setAddingColumn(true)}
								>
									<PlusIcon />
									Add column
								</Button>
							)}
						</div>
					</div>
				</DndContext>
			</div>
			{agentId && (
				<div
					className={cn(
						"flex min-h-0 flex-1 flex-col border-t-2 border-border",
					)}
				>
					<Suspense fallback={<AgentChatPageSkeleton />}>
						<AgentChatPage />
					</Suspense>
				</div>
			)}
		</div>
	);
};

export default ChatBoardPage;
