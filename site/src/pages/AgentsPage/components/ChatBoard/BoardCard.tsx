import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import { PaletteIcon, PencilIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { shortRelativeTime } from "#/utils/time";
import { getChatDisplayConfig } from "../ChatsSidebar/tree/statusConfig";
import {
	type BoardCard as BoardCardModel,
	CARD_COLORS,
	type CardColor,
} from "./boardLabels";
import { ChatInfoPopover } from "./ChatInfoPopover";
import { InlineInput, InlineText } from "./InlineText";
import { NotesSection } from "./NotesSection";

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
	readonly onSetColor: (color: CardColor | undefined) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAddNote: (text: string) => void;
	readonly onEditNote: (index: number, text: string) => void;
	readonly onRemoveNote: (index: number) => void;
}

export const BoardCard: FC<BoardCardProps> = ({
	card,
	activeChatId,
	onSetTitle,
	onSetColor,
	onRenameChat,
	onAddNote,
	onEditNote,
	onRemoveNote,
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
			style={
				card.color ? { borderLeftColor: CARD_COLORS[card.color] } : undefined
			}
			className={cn(
				"flex flex-col rounded-lg border border-border border-l-[3px] bg-surface-primary text-sm shadow-xs",
				isDragging && "opacity-40",
				isMergeTarget && "border-content-link ring-1 ring-content-link",
			)}
		>
			<header
				className="group/header flex cursor-grab items-start gap-1 px-3 pt-2.5 pb-2 active:cursor-grabbing"
				{...listeners}
				{...attributes}
				ref={setActivatorNodeRef}
			>
				<InlineText
					value={card.title}
					onSave={onSetTitle}
					ariaLabel="card title"
					wrap
					className="flex-1 font-medium leading-snug text-content-primary"
				/>
				<ColorPicker
					value={card.color}
					onChange={onSetColor}
					className="-mr-1.5 -mt-0.5 opacity-0 group-hover/header:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
				/>
			</header>

			<ul className="m-0 flex list-none flex-col border-t border-border p-1.5">
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

			<NotesSection
				notes={card.comments}
				cardTitle={card.title}
				onAdd={onAddNote}
				onEdit={onEditNote}
				onRemove={onRemoveNote}
			/>
		</article>
	);
};

interface ColorPickerProps {
	readonly value: CardColor | undefined;
	readonly onChange: (color: CardColor | undefined) => void;
	readonly className?: string;
}

const ColorPicker: FC<ColorPickerProps> = ({ value, onChange, className }) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Card color"
					className={cn("size-6 shrink-0 text-content-secondary", className)}
					onPointerDown={(e) => e.stopPropagation()}
				>
					{value ? (
						<span
							className="size-3 rounded-full"
							style={{ backgroundColor: CARD_COLORS[value] }}
						/>
					) : (
						<PaletteIcon className="size-3.5" />
					)}
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="flex w-auto gap-1.5 p-2"
				onPointerDown={(e) => e.stopPropagation()}
			>
				<button
					type="button"
					aria-label="No color"
					aria-pressed={value === undefined}
					className={cn(
						"size-5 rounded-full border border-border bg-transparent",
						value === undefined && "ring-2 ring-content-link ring-offset-1",
					)}
					onClick={() => {
						onChange(undefined);
						setOpen(false);
					}}
				/>
				{(Object.keys(CARD_COLORS) as CardColor[]).map((name) => (
					<button
						key={name}
						type="button"
						aria-label={name}
						aria-pressed={value === name}
						className={cn(
							"size-5 rounded-full border-0",
							value === name && "ring-2 ring-content-link ring-offset-1",
						)}
						style={{ backgroundColor: CARD_COLORS[name] }}
						onClick={() => {
							onChange(name);
							setOpen(false);
						}}
					/>
				))}
			</PopoverContent>
		</Popover>
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
		<div className="w-80 cursor-grabbing rounded-lg border border-content-link bg-surface-primary p-3 text-sm shadow-lg">
			<div className="font-medium leading-snug text-content-primary">
				{title}
			</div>
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
				"group flex items-start gap-2 rounded-md px-1.5 py-1.5",
				active && "bg-surface-tertiary",
				isDragging && "opacity-40",
			)}
		>
			{/* The status icon doubles as the drag handle so rows need no extra gutter. */}
			<span
				ref={draggable ? setActivatorNodeRef : undefined}
				{...(draggable ? { ...listeners, ...attributes } : {})}
				className={cn(
					"mt-0.5 flex size-4 shrink-0 items-center justify-center",
					draggable && "cursor-grab active:cursor-grabbing",
				)}
				title={draggable ? "Drag to move this chat" : undefined}
			>
				<StatusIcon
					className={cn("size-3.5", display.className)}
					aria-label={display.label}
				/>
			</span>
			<div className="min-w-0 flex-1">
				{renaming ? (
					<InlineInput
						value={chat.title}
						onSave={onRename}
						onDone={() => setRenaming(false)}
						ariaLabel={`title of ${chat.title}`}
						className="w-full text-content-primary"
					/>
				) : (
					<Link
						to={`/agents/board/${chat.id}`}
						className="line-clamp-2 text-content-primary leading-snug no-underline hover:underline"
					>
						{chat.title}
					</Link>
				)}
				{chat.last_turn_summary && (
					<div className="truncate text-xs text-content-secondary">
						{chat.last_turn_summary}
					</div>
				)}
			</div>
			<div className="flex shrink-0 items-center gap-0.5">
				{!renaming && (
					<Button
						variant="subtle"
						size="icon"
						aria-label={`Rename ${chat.title}`}
						className="size-6 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
						onClick={() => setRenaming(true)}
					>
						<PencilIcon className="size-3" />
					</Button>
				)}
				{pr?.url && display.prIcon && (
					<a
						href={pr.url}
						target="_blank"
						rel="noreferrer"
						aria-label={display.prIcon.label}
						className={cn(
							"flex items-center gap-0.5 px-0.5 text-xs no-underline hover:underline",
							display.prIcon.className,
						)}
					>
						<display.prIcon.icon className="size-3.5" />
						{pr.pr_number ? `#${pr.pr_number}` : null}
					</a>
				)}
				<span className="px-0.5 text-xs tabular-nums text-content-secondary/60">
					{shortRelativeTime(chat.updated_at)}
				</span>
				<ChatInfoPopover chat={chat} />
			</div>
		</li>
	);
};
