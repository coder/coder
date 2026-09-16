import {
	DndContext,
	type DragEndEvent,
	DragOverlay,
	type DragStartEvent,
	PointerSensor,
	pointerWithin,
	useDraggable,
	useDroppable,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import { cn } from "cn";
import { ChevronDownIcon, XIcon } from "lucide-react";
import { type FC, lazy, type ReactNode, Suspense, useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { AgentChatPageSkeleton } from "../AgentsSkeletons";
import { CARD_COLOR_CLASS, type CardColor } from "./boardLabels";
import type { ChatPane } from "./boardStorage";
import {
	dragHandleListeners,
	useBlockSelectionWhileDragging,
} from "./dragHandle";

const AgentChatPage = lazy(() => import("../../AgentChatPage"));

const MAX_PANES = 2;

type TabDrag = { paneIndex: number; chatId: string };
type TabDrop = { kind: "pane"; paneIndex: number } | { kind: "split" };

/** Adds a chat as the active tab of one pane, creating the pane if needed. */
export const openTab = (
	panes: readonly ChatPane[],
	paneIndex: number,
	chatId: string,
): ChatPane[] => {
	const next = panes.map((p) => ({ ...p, tabs: [...p.tabs] }));
	// A chat lives in one pane at a time.
	for (const pane of next) {
		pane.tabs = pane.tabs.filter((id) => id !== chatId);
	}
	const target = next[paneIndex] ?? next[0];
	if (target) {
		target.tabs.push(chatId);
		target.active = chatId;
	} else {
		next.push({ tabs: [chatId], active: chatId });
	}
	return next.filter((p) => p.tabs.length > 0).map(fixActive);
};

export const closeTab = (
	panes: readonly ChatPane[],
	chatId: string,
): ChatPane[] =>
	panes
		.map((p) => ({ ...p, tabs: p.tabs.filter((id) => id !== chatId) }))
		.filter((p) => p.tabs.length > 0)
		.map(fixActive);

const fixActive = (pane: ChatPane): ChatPane =>
	pane.tabs.includes(pane.active)
		? pane
		: { ...pane, active: pane.tabs[pane.tabs.length - 1] ?? "" };

interface ChatPanesProps {
	readonly panes: readonly ChatPane[];
	readonly focusedPane: number;
	readonly chatsById: ReadonlyMap<string, Chat>;
	readonly cardColorByChatId: ReadonlyMap<string, CardColor>;
	readonly onChange: (panes: readonly ChatPane[], focusedPane: number) => void;
	readonly onCollapse: () => void;
}

/** Chats open below the board as tabs; a tab dragged to the right edge splits into a second pane. */
export const ChatPanes: FC<ChatPanesProps> = ({
	panes,
	focusedPane,
	chatsById,
	cardColorByChatId,
	onChange,
	onCollapse,
}) => {
	const [dragging, setDragging] = useState<TabDrag | null>(null);
	const sensors = useSensors(
		useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
	);
	useBlockSelectionWhileDragging(dragging !== null);

	const handleDragStart = ({ active }: DragStartEvent) => {
		setDragging((active.data.current as TabDrag | undefined) ?? null);
	};

	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		setDragging(null);
		const drag = active.data.current as TabDrag | undefined;
		const drop = over?.data.current as TabDrop | undefined;
		if (!drag || !drop) return;
		if (drop.kind === "split") {
			const remaining = closeTab(panes, drag.chatId);
			onChange(
				[...remaining, { tabs: [drag.chatId], active: drag.chatId }],
				remaining.length,
			);
			return;
		}
		if (drop.paneIndex === drag.paneIndex) return;
		onChange(openTab(panes, drop.paneIndex, drag.chatId), drop.paneIndex);
	};

	return (
		<DndContext
			sensors={sensors}
			collisionDetection={pointerWithin}
			onDragStart={handleDragStart}
			onDragCancel={() => setDragging(null)}
			onDragEnd={handleDragEnd}
		>
			<div className="flex min-h-0 flex-1">
				{panes.map((pane, index) => (
					<PaneView
						key={index}
						pane={pane}
						index={index}
						focused={index === focusedPane}
						chatsById={chatsById}
						cardColorByChatId={cardColorByChatId}
						onFocus={() => onChange(panes, index)}
						onActivate={(chatId) =>
							onChange(
								panes.map((p, i) =>
									i === index ? { ...p, active: chatId } : p,
								),
								index,
							)
						}
						onClose={(chatId) =>
							onChange(closeTab(panes, chatId), Math.min(focusedPane, index))
						}
						// The collapse control lives once, at the far right of the tab row.
						tabBarEnd={
							index === panes.length - 1 ? (
								<Button
									variant="subtle"
									size="icon"
									aria-label="Hide chats"
									className="ml-auto size-[26px] shrink-0 self-center text-content-secondary"
									onClick={onCollapse}
								>
									<ChevronDownIcon className="size-3.5" />
								</Button>
							) : undefined
						}
					/>
				))}
				{dragging && panes.length < MAX_PANES && <SplitDropZone />}
			</div>
			<DragOverlay dropAnimation={null}>
				{dragging && (
					<div className="max-w-56 truncate rounded border border-content-link bg-surface-primary px-2 py-1 text-xs text-content-primary shadow-lg">
						{chatsById.get(dragging.chatId)?.title ?? "Chat"}
					</div>
				)}
			</DragOverlay>
		</DndContext>
	);
};

interface PaneViewProps {
	readonly pane: ChatPane;
	readonly index: number;
	readonly focused: boolean;
	readonly chatsById: ReadonlyMap<string, Chat>;
	readonly cardColorByChatId: ReadonlyMap<string, CardColor>;
	readonly onFocus: () => void;
	readonly onActivate: (chatId: string) => void;
	readonly onClose: (chatId: string) => void;
	readonly tabBarEnd?: ReactNode;
}

const PaneView: FC<PaneViewProps> = ({
	pane,
	index,
	focused,
	chatsById,
	cardColorByChatId,
	onFocus,
	onActivate,
	onClose,
	tabBarEnd,
}) => {
	const dropData: TabDrop = { kind: "pane", paneIndex: index };
	const { setNodeRef, isOver } = useDroppable({
		id: `pane-tabs:${index}`,
		data: dropData,
	});
	return (
		<section
			aria-label={`Chat pane ${index + 1}`}
			className={cn(
				"flex min-h-0 min-w-0 flex-1 flex-col border-l border-border first:border-l-0",
			)}
			onPointerDownCapture={onFocus}
		>
			<div
				ref={setNodeRef}
				role="tablist"
				className={cn(
					"flex h-[34px] shrink-0 items-stretch overflow-x-auto border-b border-border bg-surface-primary pr-1.5",
					isOver && "bg-surface-tertiary/60",
				)}
			>
				{pane.tabs.map((chatId) => (
					<Tab
						key={chatId}
						chatId={chatId}
						paneIndex={index}
						title={chatsById.get(chatId)?.title ?? "Chat"}
						unread={chatsById.get(chatId)?.has_unread ?? false}
						color={cardColorByChatId.get(chatId)}
						active={chatId === pane.active}
						paneFocused={focused}
						onActivate={() => onActivate(chatId)}
						onClose={() => onClose(chatId)}
					/>
				))}
				{tabBarEnd}
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<Suspense fallback={<AgentChatPageSkeleton />}>
					<AgentChatPage key={pane.active} chatId={pane.active} />
				</Suspense>
			</div>
		</section>
	);
};

interface TabProps {
	readonly chatId: string;
	readonly paneIndex: number;
	readonly title: string;
	readonly unread: boolean;
	readonly color: CardColor | undefined;
	readonly active: boolean;
	readonly paneFocused: boolean;
	readonly onActivate: () => void;
	readonly onClose: () => void;
}

const Tab: FC<TabProps> = ({
	chatId,
	paneIndex,
	title,
	unread,
	color,
	active,
	paneFocused,
	onActivate,
	onClose,
}) => {
	const dragData: TabDrag = { paneIndex, chatId };
	const { setNodeRef, listeners, attributes, isDragging } = useDraggable({
		id: `tab:${paneIndex}:${chatId}`,
		data: dragData,
	});
	const colors = color ? CARD_COLOR_CLASS[color] : undefined;
	return (
		<div
			ref={setNodeRef}
			{...dragHandleListeners(listeners)}
			{...attributes}
			role="tab"
			aria-selected={active}
			tabIndex={active ? 0 : -1}
			className={cn(
				"group/tab relative flex max-w-[260px] min-w-0 shrink-0 cursor-default touch-none items-center gap-2 border-r border-border px-3 text-[12.5px] font-medium text-content-primary hover:bg-surface-secondary",
				isDragging && "opacity-40",
			)}
			onClick={onActivate}
			onKeyDown={(e) => {
				if (e.key === "Enter" || e.key === " ") onActivate();
			}}
			onAuxClick={(e) => {
				// Middle click closes, as in browser tabs.
				if (e.button === 1) onClose();
			}}
		>
			{/* The card's color, so the tab and its card read as one thing. */}
			<span
				className={cn(
					"size-2 shrink-0 rounded-[2px]",
					colors ? colors.swatch : "bg-content-secondary/30",
				)}
			/>
			{unread && (
				<span
					role="img"
					aria-label="Unread"
					className="size-1.5 shrink-0 rounded-full bg-content-link"
				/>
			)}
			<span className="min-w-0 truncate">{title}</span>
			<Button
				variant="subtle"
				size="icon"
				aria-label={`Close ${title}`}
				className="size-4 shrink-0 text-content-secondary/70 hover:text-content-primary"
				onPointerDown={(e) => e.stopPropagation()}
				onClick={(e) => {
					e.stopPropagation();
					onClose();
				}}
			>
				<XIcon className="size-[11px]" />
			</Button>
			{active && (
				<span
					className={cn(
						"absolute inset-x-0 -bottom-px h-0.5",
						!paneFocused
							? "bg-content-secondary/40"
							: colors
								? colors.swatch
								: "bg-content-primary",
					)}
				/>
			)}
		</div>
	);
};

const SplitDropZone: FC = () => {
	const dropData: TabDrop = { kind: "split" };
	const { setNodeRef, isOver } = useDroppable({ id: "split", data: dropData });
	return (
		<div
			ref={setNodeRef}
			className={cn(
				"flex w-24 shrink-0 items-center justify-center border-l border-dashed border-border text-center text-xs text-content-secondary",
				isOver && "bg-surface-tertiary text-content-primary",
			)}
		>
			Open side by side
		</div>
	);
};
