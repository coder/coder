import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import {
	BotIcon,
	CopyIcon,
	MessageSquareIcon,
	MessageSquarePlusIcon,
	PencilIcon,
	TagsIcon,
	UngroupIcon,
} from "lucide-react";
import { type FC, type RefObject, useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import {
	DropdownMenuCheckboxItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";
import { shortRelativeTime } from "#/utils/time";
import { getChatDisplayConfig } from "../../components/ChatsSidebar/tree/statusConfig";
import { ActionsMenu } from "./ActionsMenu";
import type { NoteSlot } from "./boardApi";
import type {
	BoardCard as BoardCardModel,
	BoardNote,
	CardColor,
} from "./boardLabels";
import { CardColorPicker } from "./CardColorPicker";
import { ChatInfoPopover } from "./ChatInfo";
import { ChatStatusLine } from "./ChatStatusLine";
import { cardAccent, cardTint } from "./cardColor";
import { dragHandleListeners } from "./dragHandle";
import { EditableTitle } from "./EditableTitle";
import { IconButton } from "./IconButton";
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

/** How a card hands a chat to the board: with the element to place a window beside. */
export type ChatOpenHandlers = {
	readonly onOpen: (chat: Chat, anchor: DOMRect) => void;
	readonly onPreview: (chat: Chat, anchor: DOMRect) => void;
	readonly onPreviewEnd: () => void;
};

/** Inside a card or row the anchor is fixed, so its openers pass only the chat. */
type ChatOpeners = {
	readonly onOpen: (chat: Chat) => void;
	readonly onPreview: (chat: Chat) => void;
	readonly onPreviewEnd: () => void;
};

// dnd-kit fills the node ref on mount; before that there is nothing to
// place a window beside.
const rectOf = (node: RefObject<HTMLElement | null>) =>
	node.current?.getBoundingClientRect() ?? new DOMRect();

type BoardCardProps = {
	readonly card: BoardCardModel;
	readonly openChatIds: ReadonlySet<string>;
	readonly isDropTarget: boolean;
	/** A dragged note hovering one of this card's notes. */
	readonly noteDrop: NoteSlot | undefined;
	/** Every effort on the board, offered as checkboxes. */
	readonly knownEfforts: readonly string[];
	readonly onSetTitle: (title: string) => void;
	readonly onSetColor: (color: CardColor | undefined) => void;
	readonly onSetEfforts: (names: readonly string[]) => void;
	/** A tag click narrows the board to that effort. */
	readonly onFilterEffort: (name: string) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAssistant: () => void;
	readonly onNewChat: () => void;
	readonly onRemoveFromGroup: (chat: Chat) => void;
	readonly onAddNote: (text: string) => void;
	readonly onEditNote: (index: number, text: string) => void;
	readonly onRemoveNote: (index: number) => void;
} & ChatOpenHandlers;

export const BoardCard: FC<BoardCardProps> = ({
	card,
	openChatIds,
	isDropTarget,
	noteDrop,
	knownEfforts,
	onSetTitle,
	onSetColor,
	onSetEfforts,
	onFilterEffort,
	onRenameChat,
	onAssistant,
	onNewChat,
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
		node,
		setActivatorNodeRef,
		listeners,
		attributes,
		isDragging,
	} = useDraggable({ id: `card:${card.id}`, data: dragData });
	const { setNodeRef: setDropRef } = useDroppable({
		id: `drop-card:${card.id}`,
		data: dropData,
	});
	// A single chat's window opens beside the card; a row's beside the row.
	const open = (chat: Chat) => onOpen(chat, rectOf(node));
	const preview = (chat: Chat) => onPreview(chat, rectOf(node));
	const setRefs = (el: HTMLElement | null) => {
		setDragRef(el);
		setDropRef(el);
	};
	const [renaming, setRenaming] = useState(false);
	const toggleEffort = (name: string, on: boolean) =>
		onSetEfforts(
			on
				? [...card.efforts, name]
				: card.efforts.filter((other) => other !== name),
		);

	const single = card.members.length === 1;
	const lead = card.primary;
	const leadDisplay = getChatDisplayConfig(lead);
	const LeadIcon = leadDisplay.icon;
	return (
		<article
			ref={setRefs}
			className={cn(
				"relative flex flex-col overflow-hidden rounded-lg border border-border bg-surface-primary text-sm shadow-[0_1px_2px_rgba(0,0,0,0.04)] transition-shadow hover:shadow-[0_2px_8px_rgba(0,0,0,0.08)]",
				cardAccent({ color: card.color }),
				// The moving copy is drawn by DragGhost inside DragOverlay; the
				// source stays put, faded, so column layout does not shift mid-drag.
				isDragging && "opacity-40",
				isDropTarget && "border-content-link ring-1 ring-content-link",
			)}
		>
			<CardColorPicker
				title={card.title}
				value={card.color}
				onChange={onSetColor}
			/>
			{/*
			  Clicking the title text renames; the board decides whether that renames
			  the chat or the card.
			*/}
			<header
				className={cn(
					"relative grid cursor-grab touch-none grid-cols-[14px_minmax(0,1fr)_auto] gap-x-2 px-3 pt-2.5 active:cursor-grabbing",
					single ? "pb-2.5" : "pb-1.5",
					cardTint({ color: card.color }),
				)}
				{...dragHandleListeners(listeners)}
				{...attributes}
				ref={setActivatorNodeRef}
			>
				{single && (
					<OpenChatSurface chat={lead} isDragging={isDragging} onOpen={open} />
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
					open={single && openChatIds.has(lead.id)}
					className="text-[14px] font-medium leading-[19px] tracking-[-0.005em]"
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
								onOpen={open}
								onPreview={preview}
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
								label: "New chat in card",
								icon: MessageSquarePlusIcon,
								onSelect: onNewChat,
							},
							{
								label: "Efforts",
								icon: TagsIcon,
								children: (
									<EffortsSubMenu
										selected={card.efforts}
										known={knownEfforts}
										onToggle={toggleEffort}
									/>
								),
							},
							{
								label: "Rename",
								icon: PencilIcon,
								onSelect: () => setRenaming(true),
							},
						]}
					/>
				</div>
				{card.efforts.length > 0 && (
					<div className="col-start-2 col-end-[-1] mt-1 flex flex-wrap gap-1">
						{card.efforts.map((name) => (
							// Neutral on purpose: an effort spans columns, so it must never
							// read as a stage the way a hued ColumnTag does.
							<button
								key={name}
								type="button"
								aria-label={`Filter by ${name}`}
								className="relative z-[1] inline-flex h-4 max-w-28 shrink-0 items-center truncate rounded border-0 bg-content-primary/5 px-1.5 text-[11px] text-content-secondary hover:text-content-primary"
								onPointerDown={(e) => e.stopPropagation()}
								onClick={() => onFilterEffort(name)}
							>
								{name}
							</button>
						))}
					</div>
				)}
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

interface EffortsSubMenuProps {
	readonly selected: readonly string[];
	readonly known: readonly string[];
	readonly onToggle: (name: string, on: boolean) => void;
}

/** One checkbox per effort on the board, then a line to coin a new one; each change saves. */
const EffortsSubMenu: FC<EffortsSubMenuProps> = ({
	selected,
	known,
	onToggle,
}) => (
	<>
		{[...new Set([...known, ...selected])].map((name) => (
			<DropdownMenuCheckboxItem
				key={name}
				checked={selected.includes(name)}
				onCheckedChange={(on) => onToggle(name, on === true)}
				// The menu stays open so several efforts can be toggled in a row.
				onSelect={(e) => e.preventDefault()}
			>
				{name}
			</DropdownMenuCheckboxItem>
		))}
		<DropdownMenuSeparator />
		{/*
		  Typing must not reach the menu: its typeahead would move focus to a
		  matching item mid-word. Escape still bubbles so the menu closes.
		*/}
		<div
			className="px-2 py-1.5"
			onKeyDown={(e) => {
				if (e.key !== "Escape") e.stopPropagation();
			}}
		>
			<InlineEdit
				key={selected.length}
				value=""
				placeholder="New effort"
				ariaLabel="New effort"
				className="text-xs text-content-primary"
				onSave={(name) => onToggle(name, true)}
				onDone={() => undefined}
			/>
		</div>
	</>
);

const UnreadDot: FC = () => (
	<span
		role="img"
		className="size-[7px] shrink-0 rounded-full bg-content-link"
		aria-label="Unread"
	/>
);

type AgeProps = {
	readonly at: string;
};

const Age: FC<AgeProps> = ({ at }) => (
	<span className="text-[11px] tabular-nums text-content-secondary/70">
		{shortRelativeTime(at)}
	</span>
);

type OpenChatSurfaceProps = {
	readonly chat: Chat;
	readonly isDragging: boolean;
	readonly onOpen: (chat: Chat) => void;
};

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
	// plain click may open. Remembered from render because isDragging is
	// already false again by the time that click arrives.
	const [wasDragging, setWasDragging] = useState(false);
	if (isDragging && !wasDragging) setWasDragging(true);
	return (
		<button
			type="button"
			aria-label={`Open ${chat.title}`}
			className="absolute inset-0 cursor-pointer border-0 bg-transparent p-0"
			onPointerDown={() => setWasDragging(false)}
			onClick={() => {
				if (!wasDragging) onOpen(chat);
			}}
		/>
	);
};

type ChatOpenerProps = {
	readonly chat: Chat;
} & ChatOpeners;

/** The chat icon: resting on it previews the chat, clicking it pins the window. */
const ChatOpener: FC<ChatOpenerProps> = ({
	chat,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => (
	<IconButton
		aria-label={`Open ${chat.title}`}
		title="Open chat"
		onPointerEnter={() => onPreview(chat)}
		onPointerLeave={onPreviewEnd}
		onClick={() => onOpen(chat)}
	>
		<MessageSquareIcon className="size-3.5" />
	</IconButton>
);

type ChatRowProps = {
	readonly chat: Chat;
	readonly card: BoardCardModel;
	readonly open: boolean;
	readonly onRename: (title: string) => void;
	readonly onRemove: () => void;
} & ChatOpenHandlers;

// Any member can leave, the primary included: the mutation hands the card
// to the next member, so nothing here needs to know who is primary.
const ChatRow: FC<ChatRowProps> = ({
	chat,
	card,
	open: isOpen,
	onRename,
	onRemove,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => {
	const dragData: DragData = { type: "chat", chat, card };
	const {
		setNodeRef,
		node,
		setActivatorNodeRef,
		listeners,
		attributes,
		isDragging,
	} = useDraggable({ id: `chat:${chat.id}`, data: dragData });
	const open = () => onOpen(chat, rectOf(node));
	const preview = () => onPreview(chat, rectOf(node));
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
			<OpenChatSurface chat={chat} isDragging={isDragging} onOpen={open} />
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
					open={isOpen}
					className="text-[13px] leading-[18px]"
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
					onOpen={open}
					onPreview={preview}
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
