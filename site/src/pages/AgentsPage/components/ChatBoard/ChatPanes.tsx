import {
	DndContext,
	type DragEndEvent,
	MouseSensor,
	pointerWithin,
	TouchSensor,
	useDraggable,
	useDroppable,
	useSensor,
	useSensors,
} from "@dnd-kit/core";
import { cn } from "cn";
import { XIcon } from "lucide-react";
import { type FC, lazy, Suspense, useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { AgentChatPageSkeleton } from "../AgentsSkeletons";
import type { ChatPane } from "./boardStorage";

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
	readonly onChange: (panes: readonly ChatPane[], focusedPane: number) => void;
}

/** Chats open below the board as tabs; a tab dragged to the right edge splits into a second pane. */
export const ChatPanes: FC<ChatPanesProps> = ({
	panes,
	focusedPane,
	chatsById,
	onChange,
}) => {
	const [dragging, setDragging] = useState(false);
	const sensors = useSensors(
		useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
		useSensor(TouchSensor, {
			activationConstraint: { delay: 150, tolerance: 5 },
		}),
	);

	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		setDragging(false);
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
			onDragStart={() => setDragging(true)}
			onDragCancel={() => setDragging(false)}
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
					/>
				))}
				{dragging && panes.length < MAX_PANES && <SplitDropZone />}
			</div>
		</DndContext>
	);
};

interface PaneViewProps {
	readonly pane: ChatPane;
	readonly index: number;
	readonly focused: boolean;
	readonly chatsById: ReadonlyMap<string, Chat>;
	readonly onFocus: () => void;
	readonly onActivate: (chatId: string) => void;
	readonly onClose: (chatId: string) => void;
}

const PaneView: FC<PaneViewProps> = ({
	pane,
	index,
	focused,
	chatsById,
	onFocus,
	onActivate,
	onClose,
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
					"flex h-8 shrink-0 items-stretch overflow-x-auto border-b border-border bg-surface-secondary/60",
					isOver && "bg-surface-tertiary",
				)}
			>
				{pane.tabs.map((chatId) => (
					<Tab
						key={chatId}
						chatId={chatId}
						paneIndex={index}
						title={chatsById.get(chatId)?.title ?? "Chat"}
						unread={chatsById.get(chatId)?.has_unread ?? false}
						active={chatId === pane.active}
						paneFocused={focused}
						onActivate={() => onActivate(chatId)}
						onClose={() => onClose(chatId)}
					/>
				))}
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
	return (
		<div
			ref={setNodeRef}
			{...listeners}
			{...attributes}
			role="tab"
			aria-selected={active}
			tabIndex={active ? 0 : -1}
			className={cn(
				"group/tab flex max-w-56 min-w-0 shrink-0 cursor-default items-center gap-1 border-r border-border px-2 text-xs",
				active
					? "bg-surface-primary text-content-primary"
					: "text-content-secondary hover:bg-surface-tertiary/60",
				active &&
					paneFocused &&
					"shadow-[inset_0_-2px_0_0_hsl(var(--content-link))]",
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
			{unread && (
				<span
					role="img"
					className="size-1.5 shrink-0 rounded-full bg-content-link"
					aria-label="Unread"
				/>
			)}
			<span className="min-w-0 truncate">{title}</span>
			<Button
				variant="subtle"
				size="icon"
				aria-label={`Close ${title}`}
				className="size-5 shrink-0 opacity-0 group-hover/tab:opacity-100 focus-visible:opacity-100"
				onPointerDown={(e) => e.stopPropagation()}
				onClick={(e) => {
					e.stopPropagation();
					onClose();
				}}
			>
				<XIcon className="size-3" />
			</Button>
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
