import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import { GripVerticalIcon, PencilIcon, XIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Textarea } from "#/components/Textarea/Textarea";
import { shortRelativeTime } from "#/utils/time";
import { getChatDisplayConfig } from "../ChatsSidebar/tree/statusConfig";
import type { BoardCard as BoardCardModel } from "./boardLabels";
import { ChatInfoPopover } from "./ChatInfoPopover";
import { InlineInput, InlineText } from "./InlineText";

export type DragData =
	| { type: "card"; card: BoardCardModel }
	| { type: "chat"; chat: Chat; card: BoardCardModel };

export type DropData =
	| { type: "column"; name: string }
	| { type: "card"; card: BoardCardModel };

const cardDragId = (card: BoardCardModel) => `card:${card.id}`;
const chatDragId = (chat: Chat) => `chat:${chat.id}`;
const cardDropId = (card: BoardCardModel) => `drop-card:${card.id}`;

interface BoardCardProps {
	readonly card: BoardCardModel;
	readonly activeChatId: string | undefined;
	readonly onSetTitle: (title: string) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAddComment: (text: string) => void;
	readonly onRemoveComment: (index: number) => void;
}

export const BoardCard: FC<BoardCardProps> = ({
	card,
	activeChatId,
	onSetTitle,
	onRenameChat,
	onAddComment,
	onRemoveComment,
}) => {
	const dragData: DragData = { type: "card", card };
	const dropData: DropData = { type: "card", card };
	const {
		setNodeRef: setDragRef,
		setActivatorNodeRef,
		listeners,
		attributes,
		isDragging,
	} = useDraggable({ id: cardDragId(card), data: dragData });
	const {
		setNodeRef: setDropRef,
		isOver,
		active,
	} = useDroppable({ id: cardDropId(card), data: dropData });
	const setRefs = (node: HTMLElement | null) => {
		setDragRef(node);
		setDropRef(node);
	};
	const isMergeTarget = isOver && active?.id !== cardDragId(card);

	// The moving copy is drawn by DragGhost inside DragOverlay; the source
	// stays put so column layout does not shift mid-drag.
	return (
		<article
			ref={setRefs}
			className={cn(
				"flex flex-col gap-2 rounded-lg border border-border bg-surface-secondary p-2 text-sm",
				isDragging && "opacity-40",
				isMergeTarget && "border-content-link ring-1 ring-content-link",
			)}
		>
			<header
				className="flex cursor-grab items-center gap-1.5 border-b border-border pb-2 font-medium text-content-primary active:cursor-grabbing"
				{...listeners}
				{...attributes}
				ref={setActivatorNodeRef}
			>
				<InlineText
					value={card.title}
					onSave={onSetTitle}
					ariaLabel="card title"
					className="flex-1"
				/>
			</header>

			<ul className="m-0 flex list-none flex-col gap-0.5 p-0">
				{card.members.map((chat) => (
					<ChatRow
						key={chat.id}
						chat={chat}
						card={card}
						draggable={card.members.length > 1 && chat.id !== card.id}
						active={chat.id === activeChatId}
						onRename={(title) => onRenameChat(chat, title)}
					/>
				))}
			</ul>

			<CommentThread
				card={card}
				onAdd={onAddComment}
				onRemove={onRemoveComment}
			/>
		</article>
	);
};

interface DragGhostProps {
	readonly drag: DragData;
}

/** Compact stand-in rendered in the DragOverlay while a card or chat moves. */
export const DragGhost: FC<DragGhostProps> = ({ drag }) => {
	const title = drag.type === "card" ? drag.card.title : drag.chat.title;
	const detail =
		drag.type === "card" && drag.card.members.length > 1
			? `${drag.card.members.length} chats`
			: undefined;
	return (
		<div className="w-80 cursor-grabbing rounded-lg border border-content-link bg-surface-secondary p-2 text-sm shadow-lg">
			<div className="truncate font-medium text-content-primary">{title}</div>
			{detail && <div className="text-xs text-content-secondary">{detail}</div>}
		</div>
	);
};

interface ChatRowProps {
	readonly chat: Chat;
	readonly card: BoardCardModel;
	readonly draggable: boolean;
	readonly active: boolean;
	readonly onRename: (title: string) => void;
}

const ChatRow: FC<ChatRowProps> = ({
	chat,
	card,
	draggable,
	active,
	onRename,
}) => {
	const dragData: DragData = { type: "chat", chat, card };
	const { setNodeRef, setActivatorNodeRef, listeners, attributes, isDragging } =
		useDraggable({
			id: chatDragId(chat),
			data: dragData,
			disabled: !draggable,
		});
	const [renaming, setRenaming] = useState(false);
	const display = getChatDisplayConfig(chat);
	const StatusIcon = display.icon;
	const pr = display.diffStatus;

	return (
		<li
			ref={setNodeRef}
			className={cn(
				"group flex items-center gap-1.5 rounded px-1 py-0.5",
				active && "bg-surface-tertiary",
				isDragging && "opacity-40",
			)}
		>
			{draggable ? (
				<button
					type="button"
					aria-label={`Drag ${chat.title}`}
					className="cursor-grab border-0 bg-transparent p-0 text-content-secondary opacity-0 group-hover:opacity-100 active:cursor-grabbing"
					ref={setActivatorNodeRef}
					{...listeners}
					{...attributes}
				>
					<GripVerticalIcon className="size-3.5" />
				</button>
			) : (
				<span className="w-3.5 shrink-0" />
			)}
			<StatusIcon
				className={cn("size-3.5 shrink-0", display.className)}
				aria-label={display.label}
			/>
			{renaming ? (
				<InlineInput
					value={chat.title}
					onSave={onRename}
					onDone={() => setRenaming(false)}
					ariaLabel={`title of ${chat.title}`}
					className="flex-1 text-content-primary"
				/>
			) : (
				<>
					<Link
						to={`/agents/board/${chat.id}`}
						className="min-w-0 flex-1 truncate text-content-primary no-underline hover:underline"
						title={chat.last_turn_summary ?? undefined}
					>
						{chat.title}
						{chat.last_turn_summary && (
							<span className="ml-1.5 text-xs text-content-secondary">
								{chat.last_turn_summary}
							</span>
						)}
					</Link>
					<Button
						variant="subtle"
						size="icon"
						aria-label={`Rename ${chat.title}`}
						className="size-5 shrink-0 opacity-0 group-hover:opacity-100"
						onClick={() => setRenaming(true)}
					>
						<PencilIcon className="size-3" />
					</Button>
				</>
			)}
			{pr?.url && display.prIcon && (
				<a
					href={pr.url}
					target="_blank"
					rel="noreferrer"
					aria-label={display.prIcon.label}
					className={cn(
						"flex shrink-0 items-center gap-0.5 text-xs no-underline hover:underline",
						display.prIcon.className,
					)}
				>
					<display.prIcon.icon className="size-3.5" />
					{pr.pr_number ? `#${pr.pr_number}` : null}
				</a>
			)}
			<span className="shrink-0 text-xs tabular-nums text-content-secondary/60">
				{shortRelativeTime(chat.updated_at)}
			</span>
			<ChatInfoPopover chat={chat} />
		</li>
	);
};

interface CommentThreadProps {
	readonly card: BoardCardModel;
	readonly onAdd: (text: string) => void;
	readonly onRemove: (index: number) => void;
}

const CommentThread: FC<CommentThreadProps> = ({ card, onAdd, onRemove }) => {
	const [draft, setDraft] = useState("");
	const submit = () => {
		const text = draft.trim();
		if (!text) return;
		onAdd(text);
		setDraft("");
	};

	return (
		<div
			className="flex flex-col gap-1 border-t border-border pt-2"
			// Typing and selecting text must not start a card drag.
			onPointerDown={(e) => e.stopPropagation()}
		>
			{card.comments.map((comment) => (
				<div
					key={comment.index}
					className="group/comment flex items-start gap-2 text-xs"
				>
					<span className="w-7 shrink-0 text-right tabular-nums text-content-secondary/60">
						{comment.timestamp ? shortRelativeTime(comment.timestamp) : ""}
					</span>
					<p className="m-0 min-w-0 flex-1 whitespace-pre-wrap text-content-secondary">
						{comment.text}
					</p>
					<Button
						variant="subtle"
						size="icon"
						aria-label="Delete comment"
						className="size-5 shrink-0 opacity-0 group-hover/comment:opacity-100"
						onClick={() => onRemove(comment.index)}
					>
						<XIcon className="size-3" />
					</Button>
				</div>
			))}
			<Textarea
				aria-label={`Add a comment to ${card.title}`}
				placeholder="Add a comment..."
				value={draft}
				rows={1}
				className="min-h-7 resize-none px-2 py-1 text-xs"
				onChange={(e) => setDraft(e.target.value)}
				onKeyDown={(e) => {
					if (e.key === "Enter" && !e.shiftKey) {
						e.preventDefault();
						submit();
					}
				}}
			/>
		</div>
	);
};
