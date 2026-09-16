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
import { EditableText, InlineInput } from "./InlineText";
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

// Accent border from the theme's highlight tokens, which flip between dark
// and light saturations with the color mode. Decorative only, never status.
const CARD_ACCENT_CLASS: Record<CardColor, string> = {
	green: "border-l-highlight-green",
	orange: "border-l-highlight-orange",
	sky: "border-l-highlight-sky",
	red: "border-l-highlight-red",
	purple: "border-l-highlight-purple",
	magenta: "border-l-highlight-magenta",
};

const SWATCH_CLASS: Record<CardColor, string> = {
	green: "bg-highlight-green",
	orange: "bg-highlight-orange",
	sky: "bg-highlight-sky",
	red: "bg-highlight-red",
	purple: "bg-highlight-purple",
	magenta: "bg-highlight-magenta",
};

interface BoardCardProps {
	readonly card: BoardCardModel;
	readonly openChatIds: ReadonlySet<string>;
	readonly isMergeTarget: boolean;
	readonly onSetTitle: (title: string) => void;
	readonly onSetColor: (color: CardColor | undefined) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAddNote: (text: string) => void;
	readonly onEditNote: (index: number, text: string) => void;
	readonly onRemoveNote: (index: number) => void;
}

export const BoardCard: FC<BoardCardProps> = ({
	card,
	openChatIds,
	isMergeTarget,
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
	const { setNodeRef: setDropRef } = useDroppable({
		id: cardDropId(card),
		data: dropData,
	});
	const setRefs = (node: HTMLElement | null) => {
		setDragRef(node);
		setDropRef(node);
	};

	// The moving copy is drawn by DragGhost inside DragOverlay; the source
	// stays put so column layout does not shift mid-drag.
	return (
		<article
			ref={setRefs}
			className={cn(
				"flex flex-col rounded-lg border border-border bg-surface-secondary text-sm",
				card.color && cn("border-l-[3px]", CARD_ACCENT_CLASS[card.color]),
				isDragging && "opacity-40",
				isMergeTarget && "border-content-link ring-1 ring-content-link",
			)}
		>
			{/* Header is its own hover group so its pencil does not light up from rows below. */}
			<header
				className="group/card flex cursor-grab items-start gap-1 px-3 py-2 active:cursor-grabbing"
				{...listeners}
				{...attributes}
				ref={setActivatorNodeRef}
			>
				<EditableText
					value={card.title}
					onSave={onSetTitle}
					ariaLabel="card title"
					wrap
					revealOn="card"
					className="font-medium leading-snug text-content-primary"
				/>
				<ColorPicker value={card.color} onChange={onSetColor} />
			</header>

			<ul className="m-0 flex list-none flex-col gap-1 border-t border-border px-3 py-2">
				{card.members.map((chat) => (
					<ChatRow
						key={chat.id}
						chat={chat}
						card={card}
						draggable={card.members.length > 1 && chat.id !== card.id}
						active={openChatIds.has(chat.id)}
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
}

const ColorPicker: FC<ColorPickerProps> = ({ value, onChange }) => {
	const [open, setOpen] = useState(false);
	const pick = (color: CardColor | undefined) => {
		onChange(color);
		setOpen(false);
	};
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Card color"
					className="size-6 shrink-0 text-content-secondary opacity-0 group-hover/card:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
					onPointerDown={(e) => e.stopPropagation()}
				>
					<PaletteIcon className="size-3.5" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="flex w-auto gap-1.5 p-2"
				onPointerDown={(e) => e.stopPropagation()}
			>
				<Swatch
					label="No color"
					selected={value === undefined}
					className="bg-surface-secondary"
					onClick={() => pick(undefined)}
				/>
				{CARD_COLORS.map((name) => (
					<Swatch
						key={name}
						label={name}
						selected={value === name}
						className={SWATCH_CLASS[name]}
						onClick={() => pick(name)}
					/>
				))}
			</PopoverContent>
		</Popover>
	);
};

const Swatch: FC<{
	readonly label: string;
	readonly selected: boolean;
	readonly className: string;
	readonly onClick: () => void;
}> = ({ label, selected, className, onClick }) => (
	<button
		type="button"
		aria-label={label}
		aria-pressed={selected}
		className={cn(
			"size-6 rounded-md border border-border",
			className,
			selected && "ring-2 ring-content-link ring-offset-1",
		)}
		onClick={onClick}
	/>
);

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
		<div
			className={cn(
				"w-80 cursor-grabbing rounded-lg border border-content-link bg-surface-secondary px-3 py-2 text-sm shadow-lg",
				drag.type === "card" &&
					drag.card.color &&
					cn("border-l-[3px]", CARD_ACCENT_CLASS[drag.card.color]),
			)}
		>
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
				"group/row -mx-1.5 flex items-start gap-2 rounded-md px-1.5 py-1 hover:bg-surface-tertiary/70",
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
			<div className="flex min-w-0 flex-1 flex-col">
				<div className="flex items-start gap-1">
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
								className="line-clamp-2 min-w-0 flex-1 text-content-primary leading-snug no-underline"
							>
								{chat.title}
							</Link>
							<EditTrigger
								label={`Rename ${chat.title}`}
								onClick={() => setRenaming(true)}
							/>
							{chat.has_unread && (
								<span
									role="img"
									className="mt-1.5 size-2 shrink-0 rounded-full bg-content-link"
									aria-label="Unread"
								/>
							)}
							<span className="shrink-0 text-xs tabular-nums leading-5 text-content-secondary">
								{shortRelativeTime(chat.updated_at)}
							</span>
							<ChatInfoPopover chat={chat} />
						</>
					)}
				</div>
				{(chat.last_turn_summary || pr?.url) && (
					<div className="flex min-w-0 items-center gap-2 text-xs text-content-secondary">
						{pr?.url && display.prIcon && (
							<a
								href={pr.url}
								target="_blank"
								rel="noreferrer"
								aria-label={display.prIcon.label}
								className={cn(
									"flex shrink-0 items-center gap-0.5 no-underline hover:underline",
									display.prIcon.className,
								)}
							>
								<display.prIcon.icon className="size-3.5" />
								{pr.pr_number ? `#${pr.pr_number}` : null}
							</a>
						)}
						{chat.last_turn_summary && (
							<span className="truncate">{chat.last_turn_summary}</span>
						)}
					</div>
				)}
			</div>
		</li>
	);
};

const EditTrigger: FC<{
	readonly label: string;
	readonly onClick: () => void;
}> = ({ label, onClick }) => (
	<Button
		variant="subtle"
		size="icon"
		aria-label={label}
		className="size-5 shrink-0 text-content-secondary opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100"
		onClick={onClick}
	>
		<PencilIcon className="size-3.5" />
	</Button>
);
