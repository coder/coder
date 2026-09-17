import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import {
	BotIcon,
	CopyIcon,
	MessageSquareIcon,
	PencilIcon,
	UngroupIcon,
} from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { shortRelativeTime } from "#/utils/time";
import { getChatDisplayConfig } from "../../components/ChatsSidebar/tree/statusConfig";
import { ActionsMenu } from "./ActionsMenu";
import type { NoteSlot } from "./boardApi";
import {
	type BoardCard as BoardCardModel,
	type BoardNote,
	CARD_COLOR_CLASS,
	CARD_COLORS,
	type CardColor,
} from "./boardLabels";
import { ChatInfoPopover } from "./ChatInfo";
import { dragHandleListeners } from "./dragHandle";
import { InlineEdit } from "./InlineEdit";
import { NotesSection } from "./NotesSection";

export type DragData =
	| { type: "card"; card: BoardCardModel }
	| { type: "chat"; chat: Chat; card: BoardCardModel }
	| { type: "column"; name: string }
	| { type: "note"; card: BoardCardModel; note: BoardNote };

export type DropData =
	| { type: "column"; name: string }
	| { type: "card"; card: BoardCardModel }
	| { type: "note"; card: BoardCardModel; note: BoardNote };

const cardDragId = (card: BoardCardModel) => `card:${card.id}`;
const chatDragId = (chat: Chat) => `chat:${chat.id}`;
const cardDropId = (card: BoardCardModel) => `drop-card:${card.id}`;

// Chats open in a floating window are marked by their title alone, no chrome.
const OPEN_TITLE_CLASS = "font-medium text-highlight-purple";

/** How a card hands a chat to the board: with the element to place a window beside. */
export interface ChatOpenHandlers {
	readonly onOpen: (chat: Chat, anchor: DOMRect) => void;
	readonly onPreview: (chat: Chat, anchor: DOMRect) => void;
	readonly onPreviewEnd: () => void;
}

interface BoardCardProps extends ChatOpenHandlers {
	readonly card: BoardCardModel;
	readonly openChatIds: ReadonlySet<string>;
	readonly isMergeTarget: boolean;
	/** A dragged note hovering one of this card's notes. */
	readonly noteDrop: NoteSlot | undefined;
	readonly onSetTitle: (title: string) => void;
	readonly onSetColor: (color: CardColor | undefined) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAssistant: () => void;
	readonly onRemoveFromGroup: (chat: Chat) => void;
	readonly onAddNote: (text: string) => void;
	readonly onEditNote: (index: number, text: string) => void;
	readonly onRemoveNote: (index: number) => void;
}

export const BoardCard: FC<BoardCardProps> = ({
	card,
	openChatIds,
	isMergeTarget,
	noteDrop,
	onSetTitle,
	onSetColor,
	onRenameChat,
	onAssistant,
	onRemoveFromGroup,
	onOpen,
	onPreview,
	onPreviewEnd,
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
	const [renaming, setRenaming] = useState(false);
	const [pickingColor, setPickingColor] = useState(false);

	const single = card.members.length === 1;
	const lead = card.primary;
	const leadDisplay = getChatDisplayConfig(lead);
	const LeadIcon = leadDisplay.icon;
	const colors = card.color ? CARD_COLOR_CLASS[card.color] : undefined;
	const titleClass = cn(
		"text-[14px] font-medium leading-[19px] tracking-[-0.005em]",
		single && openChatIds.has(lead.id) && OPEN_TITLE_CLASS,
	);
	return (
		<article
			ref={setRefs}
			className={cn(
				"relative flex flex-col overflow-hidden rounded-lg border border-border bg-surface-primary text-sm shadow-[0_1px_2px_rgba(0,0,0,0.04)] transition-shadow hover:shadow-[0_2px_8px_rgba(0,0,0,0.08)]",
				colors && cn("border-l-[3px]", colors.accent),
				// The moving copy is drawn by DragGhost inside DragOverlay; the
				// source stays put, faded, so column layout does not shift mid-drag.
				isDragging && "opacity-40",
				isMergeTarget && "border-content-link ring-1 ring-content-link",
			)}
		>
			{/*
			  The stripe shows the color, so the stripe is where you change it. The
			  swatches float beside it rather than reflowing the header.
			*/}
			<Popover open={pickingColor} onOpenChange={setPickingColor}>
				<PopoverTrigger asChild>
					<button
						type="button"
						title="Card color"
						aria-label={`Color of ${card.title}`}
						className="absolute inset-y-0 left-0 z-[1] w-2 border-0 bg-transparent p-0 hover:bg-content-primary/10 data-[state=open]:bg-content-primary/10"
					/>
				</PopoverTrigger>
				<PopoverContent
					side="right"
					align="start"
					sideOffset={6}
					className="w-auto p-2"
					onPointerDown={(e) => e.stopPropagation()}
				>
					<ColorSwatches
						value={card.color}
						onChange={(color) => {
							setPickingColor(false);
							onSetColor(color);
						}}
					/>
				</PopoverContent>
			</Popover>
			{/*
			  Same anatomy for every card: [icon] title [meta]. A single chat is
			  its own card, so its title is the chat title and there are no rows;
			  a group shows a stack icon, the card title, and one row per chat.
			  Click the title text to rename it (the board decides whether that
			  renames the chat or the card); click anywhere else on a single
			  card's header to open the chat, or rest on its chat icon to preview
			  it. The band is washed with the accent.
			*/}
			<header
				className={cn(
					"relative grid cursor-grab touch-none grid-cols-[14px_minmax(0,1fr)_auto] gap-x-2 px-3 pt-2.5 active:cursor-grabbing",
					single ? "pb-2.5" : "pb-1.5",
					colors?.tint,
				)}
				{...dragHandleListeners(listeners)}
				{...attributes}
				ref={setActivatorNodeRef}
			>
				{single && (
					<OpenChatSurface
						chat={lead}
						isDragging={isDragging}
						onOpen={onOpen}
					/>
				)}
				<span className="flex h-[19px] items-center justify-center">
					{single ? (
						<LeadIcon
							className={cn("size-[13px]", leadDisplay.className)}
							aria-label={leadDisplay.label}
						/>
					) : (
						<CopyIcon
							className="size-[13px] text-content-secondary"
							aria-label={`Group of ${card.members.length} chats`}
						/>
					)}
				</span>
				<EditableTitle
					value={card.title}
					renaming={renaming}
					className={titleClass}
					onEdit={() => setRenaming(true)}
					onRenamed={(title) => {
						setRenaming(false);
						onSetTitle(title);
					}}
					onCancel={() => setRenaming(false)}
				/>
				<div className="flex h-[19px] items-center gap-1.5">
					{single ? (
						<>
							{lead.has_unread && <UnreadDot />}
							<Age at={lead.updated_at} />
							<ChatInfoPopover chat={lead} />
							<ChatOpener
								chat={lead}
								onOpen={onOpen}
								onPreview={onPreview}
								onPreviewEnd={onPreviewEnd}
							/>
						</>
					) : (
						<span className="text-[11px] text-content-secondary/70">
							{card.members.length} chats
						</span>
					)}
					<ActionsMenu
						label={card.title}
						permanent
						items={[
							{ label: "Assistant", icon: BotIcon, onSelect: onAssistant },
							{
								label: "Rename",
								icon: PencilIcon,
								onSelect: () => setRenaming(true),
							},
						]}
					/>
				</div>
				{single && (
					<div className="col-start-2 col-end-[-1] mt-0.5">
						<ChatStatusLine chat={lead} />
					</div>
				)}
			</header>

			{!single && (
				<ul className="m-0 flex list-none flex-col px-1 pb-1">
					{card.members.map((chat) => (
						<ChatRow
							key={chat.id}
							chat={chat}
							card={card}
							open={openChatIds.has(chat.id)}
							onRename={(title) => onRenameChat(chat, title)}
							onRemove={() => onRemoveFromGroup(chat)}
							onOpen={onOpen}
							onPreview={onPreview}
							onPreviewEnd={onPreviewEnd}
						/>
					))}
				</ul>
			)}

			<NotesSection
				card={card}
				noteDrop={noteDrop}
				onAdd={onAddNote}
				onEdit={onEditNote}
				onRemove={onRemoveNote}
			/>
		</article>
	);
};

const UnreadDot: FC = () => (
	<span
		role="img"
		className="size-[7px] shrink-0 rounded-full bg-content-link"
		aria-label="Unread"
	/>
);

const Age: FC<{ readonly at: string }> = ({ at }) => (
	<span className="text-[11px] tabular-nums text-content-secondary/70">
		{shortRelativeTime(at)}
	</span>
);

interface OpenChatSurfaceProps {
	readonly chat: Chat;
	readonly isDragging: boolean;
	readonly onOpen: (chat: Chat, anchor: DOMRect) => void;
}

/**
 * Invisible surface under a card header or row: a click that lands on no
 * control opens the chat. Controls that keep their own click sit above it
 * with `relative z-[1]`.
 */
const OpenChatSurface: FC<OpenChatSurfaceProps> = ({
	chat,
	isDragging,
	onOpen,
}) => {
	// A drop that ends where the drag began also fires a click; only a
	// plain click may open.
	const dragged = useRef(false);
	useEffect(() => {
		if (isDragging) dragged.current = true;
	}, [isDragging]);
	return (
		<button
			type="button"
			aria-label={`Open ${chat.title}`}
			className="absolute inset-0 cursor-pointer border-0 bg-transparent p-0"
			onPointerDown={() => {
				dragged.current = false;
			}}
			onClick={(e) => {
				if (!dragged.current) onOpen(chat, anchorOf(e.currentTarget));
			}}
		/>
	);
};

// The header or row the control sits in; the window opens beside it.
const anchorOf = (el: HTMLElement) =>
	(el.closest("header, li") ?? el).getBoundingClientRect();

interface ChatOpenerProps extends ChatOpenHandlers {
	readonly chat: Chat;
}

/** The chat icon: resting on it previews the chat, clicking it pins the window. */
const ChatOpener: FC<ChatOpenerProps> = ({
	chat,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => (
	<button
		type="button"
		aria-label={`Open ${chat.title}`}
		title="Open chat"
		className="relative z-[1] grid size-4 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/60 hover:text-content-primary"
		onPointerDown={(e) => e.stopPropagation()}
		onPointerEnter={(e) => onPreview(chat, anchorOf(e.currentTarget))}
		onPointerLeave={onPreviewEnd}
		onClick={(e) => onOpen(chat, anchorOf(e.currentTarget))}
	>
		<MessageSquareIcon className="size-3.5" />
	</button>
);

interface EditableTitleProps {
	readonly value: string;
	readonly renaming: boolean;
	readonly className: string;
	readonly onEdit: () => void;
	readonly onRenamed: (title: string) => void;
	readonly onCancel: () => void;
}

/** Two-line title; clicking the text (only the text) edits it in place. */
const EditableTitle: FC<EditableTitleProps> = ({
	value,
	renaming,
	className,
	onEdit,
	onRenamed,
	onCancel,
}) => {
	if (renaming) {
		return (
			<InlineEdit
				value={value}
				onSave={onRenamed}
				onDone={onCancel}
				ariaLabel="title"
				className={cn("relative z-[1] text-content-primary", className)}
			/>
		);
	}
	return (
		<button
			type="button"
			title="Click to rename"
			className={cn(
				"relative z-[1] m-0 w-fit max-w-full min-w-0 cursor-text justify-self-start border-0 bg-transparent p-0 text-left text-content-primary",
				className,
			)}
			onClick={onEdit}
		>
			<span className="line-clamp-2 wrap-anywhere [text-wrap:pretty]">
				{value}
			</span>
		</button>
	);
};

interface ColorSwatchesProps {
	readonly value: CardColor | undefined;
	readonly onChange: (color: CardColor | undefined) => void;
}

/** The palette beside the stripe: one swatch per theme accent plus none. */
const ColorSwatches: FC<ColorSwatchesProps> = ({ value, onChange }) => (
	<div className="flex items-center gap-1.5">
		<Swatch
			label="No color"
			selected={value === undefined}
			className="border-border bg-surface-primary"
			onClick={() => onChange(undefined)}
		/>
		{CARD_COLORS.map((name) => (
			<Swatch
				key={name}
				label={name}
				selected={value === name}
				className={cn("border-transparent", CARD_COLOR_CLASS[name].swatch)}
				onClick={() => onChange(name)}
			/>
		))}
	</div>
);

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
			"size-4 rounded-full border p-0 transition-transform hover:scale-110",
			className,
			selected &&
				"ring-2 ring-content-link ring-offset-1 ring-offset-surface-primary",
		)}
		onClick={onClick}
	/>
);

/** PR chip, line stats, and last turn text; shared by single cards and group rows. */
const ChatStatusLine: FC<{ readonly chat: Chat }> = ({ chat }) => {
	const display = getChatDisplayConfig(chat);
	const pr = display.diffStatus;
	const hasLineStats =
		pr !== undefined && (pr.additions > 0 || pr.deletions > 0);
	if (!chat.last_turn_summary && !pr?.url) return null;
	return (
		<div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs leading-4 text-content-secondary">
			{pr?.url && display.prIcon && (
				<a
					href={pr.url}
					target="_blank"
					rel="noreferrer"
					aria-label={display.prIcon.label}
					className="relative z-[1] inline-flex h-4 shrink-0 items-center gap-1 rounded bg-content-primary/5 px-1.5 font-mono text-[11px] text-content-secondary no-underline hover:text-content-primary"
					onPointerDown={(e) => e.stopPropagation()}
				>
					<span
						className={cn(
							"size-1.5 rounded-full bg-current",
							display.prIcon.className,
						)}
					/>
					{pr.pr_number ? `#${pr.pr_number}` : "PR"}
				</a>
			)}
			{hasLineStats && (
				<span className="shrink-0 font-mono text-[11px]">
					<span className="text-git-added-bright">+{pr.additions}</span>{" "}
					<span className="text-git-deleted-bright">&minus;{pr.deletions}</span>
				</span>
			)}
			{chat.last_turn_summary && (
				<span className="min-w-0 flex-1 truncate">
					{chat.last_turn_summary}
				</span>
			)}
		</div>
	);
};

interface DragGhostProps {
	readonly drag: DragData;
}

/** Compact stand-in rendered in the DragOverlay while a card, chat, column, or note moves. */
export const DragGhost: FC<DragGhostProps> = ({ drag }) => {
	if (drag.type === "note") {
		return (
			<div className="w-[276px] cursor-grabbing truncate rounded-md border border-content-link bg-surface-primary px-3 py-1.5 text-xs text-content-primary shadow-lg">
				{drag.note.text}
			</div>
		);
	}
	if (drag.type === "column") {
		return (
			<div className="w-[300px] cursor-grabbing rounded-md border border-content-link bg-surface-primary px-3 py-1.5 text-[13px] font-medium text-content-primary shadow-lg">
				{drag.name}
			</div>
		);
	}
	const title = drag.type === "card" ? drag.card.title : drag.chat.title;
	const detail =
		drag.type === "card" && drag.card.members.length > 1
			? `${drag.card.members.length} chats`
			: undefined;
	return (
		<div
			className={cn(
				"w-[300px] cursor-grabbing rounded-lg border border-content-link bg-surface-primary px-3 py-2 text-sm shadow-lg",
				drag.type === "card" &&
					drag.card.color &&
					cn("border-l-[3px]", CARD_COLOR_CLASS[drag.card.color].accent),
			)}
		>
			<div className="font-medium leading-snug text-content-primary">
				{title}
			</div>
			{detail && <div className="text-xs text-content-secondary">{detail}</div>}
		</div>
	);
};

interface ChatRowProps extends ChatOpenHandlers {
	readonly chat: Chat;
	readonly card: BoardCardModel;
	readonly open: boolean;
	readonly onRename: (title: string) => void;
	readonly onRemove: () => void;
}

// Any member can leave, the primary included: the mutation hands the card
// to the next member, so nothing here needs to know who is primary.
const ChatRow: FC<ChatRowProps> = ({
	chat,
	card,
	open,
	onRename,
	onRemove,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => {
	const dragData: DragData = { type: "chat", chat, card };
	const { setNodeRef, setActivatorNodeRef, listeners, attributes, isDragging } =
		useDraggable({
			id: chatDragId(chat),
			data: dragData,
		});
	const [renaming, setRenaming] = useState(false);
	const display = getChatDisplayConfig(chat);
	const StatusIcon = display.icon;

	return (
		<li
			ref={setNodeRef}
			className={cn(
				"relative grid grid-cols-[14px_minmax(0,1fr)_auto] gap-x-2 rounded-md px-2 py-1.5 hover:bg-content-primary/5",
				isDragging && "opacity-40",
			)}
		>
			<OpenChatSurface chat={chat} isDragging={isDragging} onOpen={onOpen} />
			{/* The status icon doubles as the drag handle so rows need no extra gutter. */}
			<span
				ref={setActivatorNodeRef}
				{...dragHandleListeners(listeners)}
				{...attributes}
				className="relative z-[1] flex h-[18px] cursor-grab touch-none items-center justify-center active:cursor-grabbing"
				title="Drag to move this chat"
			>
				<StatusIcon
					className={cn("size-[13px]", display.className)}
					aria-label={display.label}
				/>
			</span>
			<div className="flex min-w-0 flex-col items-start">
				<EditableTitle
					value={chat.title}
					renaming={renaming}
					className={cn("text-[13px] leading-[18px]", open && OPEN_TITLE_CLASS)}
					onEdit={() => setRenaming(true)}
					onRenamed={(title) => {
						setRenaming(false);
						onRename(title);
					}}
					onCancel={() => setRenaming(false)}
				/>
				<div className="mt-0.5 w-full">
					<ChatStatusLine chat={chat} />
				</div>
			</div>
			<div className="flex h-[18px] items-center gap-1.5">
				{chat.has_unread && <UnreadDot />}
				<Age at={chat.updated_at} />
				<ChatInfoPopover chat={chat} />
				<ChatOpener
					chat={chat}
					onOpen={onOpen}
					onPreview={onPreview}
					onPreviewEnd={onPreviewEnd}
				/>
				<ActionsMenu
					label={chat.title}
					permanent
					items={[
						{
							label: "Remove from group",
							icon: UngroupIcon,
							onSelect: onRemove,
						},
						{
							label: "Rename",
							icon: PencilIcon,
							onSelect: () => setRenaming(true),
						},
					]}
				/>
			</div>
		</li>
	);
};
