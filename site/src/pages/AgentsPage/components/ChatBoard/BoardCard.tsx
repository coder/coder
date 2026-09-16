import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import { CopyIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { shortRelativeTime } from "#/utils/time";
import { getChatDisplayConfig } from "../ChatsSidebar/tree/statusConfig";
import { ActionsMenu } from "./ActionsMenu";
import type { BoardCard as BoardCardModel, CardColor } from "./boardLabels";
import { ChatInfoPopover } from "./ChatInfo";
import { dragHandleListeners } from "./dragHandle";
import { InlineInput } from "./InlineText";
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

// Faint wash of the accent behind the header of a colored card.
const CARD_TINT_CLASS: Record<CardColor, string> = {
	green: "bg-highlight-green/10",
	orange: "bg-highlight-orange/10",
	sky: "bg-highlight-sky/10",
	red: "bg-highlight-red/10",
	purple: "bg-highlight-purple/10",
	magenta: "bg-highlight-magenta/10",
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
	const single = card.members.length === 1;
	const lead = card.primary;
	const leadDisplay = getChatDisplayConfig(lead);
	const LeadIcon = leadDisplay.icon;
	const [renaming, setRenaming] = useState(false);
	return (
		<article
			ref={setRefs}
			className={cn(
				"flex flex-col overflow-hidden rounded-lg border border-border bg-surface-primary text-sm shadow-[0_1px_2px_rgba(0,0,0,0.04)] transition-shadow hover:shadow-[0_2px_8px_rgba(0,0,0,0.08)]",
				card.color && cn("border-l-[3px]", CARD_ACCENT_CLASS[card.color]),
				isDragging && "opacity-40",
				isMergeTarget && "border-content-link ring-1 ring-content-link",
			)}
		>
			{/*
			  Same anatomy for every card: [icon] title [meta]. A single chat is
			  its own card, so its title is the chat title and there are no rows;
			  a group shows a stack icon, the card title, and one row per chat.
			  The header band is neutral by default and washed with the accent on
			  colored cards. It is its own hover group so its menu does not light
			  up from rows below.
			*/}
			<header
				className={cn(
					"group/card grid cursor-grab touch-none grid-cols-[14px_minmax(0,1fr)_auto] gap-x-2 px-3 pt-[11px] active:cursor-grabbing",
					single ? "pb-[11px]" : "pb-2",
					card.color ? CARD_TINT_CLASS[card.color] : "bg-surface-secondary/60",
				)}
				{...dragHandleListeners(listeners)}
				{...attributes}
				ref={setActivatorNodeRef}
			>
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
				<CardTitle
					value={card.title}
					href={single ? `/agents/board/${lead.id}` : undefined}
					renaming={renaming}
					onRenamed={(title) => {
						setRenaming(false);
						if (single) onRenameChat(lead, title);
						else onSetTitle(title);
					}}
					onCancel={() => setRenaming(false)}
				/>
				<div className="flex h-[19px] items-center gap-1.5">
					{single ? (
						<>
							{lead.has_unread && <UnreadDot />}
							<span className="font-mono text-[11px] tabular-nums text-content-secondary/70">
								{shortRelativeTime(lead.updated_at)}
							</span>
							<ChatInfoPopover chat={lead} />
						</>
					) : (
						<span className="font-mono text-[11px] text-content-secondary/70">
							{card.members.length} chats
						</span>
					)}
					<ActionsMenu
						label={card.title}
						revealOn="card"
						onRename={() => setRenaming(true)}
						color={{ value: card.color, onChange: onSetColor }}
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
							draggable={chat.id !== card.id}
							active={openChatIds.has(chat.id)}
							onRename={(title) => onRenameChat(chat, title)}
						/>
					))}
				</ul>
			)}

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

const UnreadDot: FC = () => (
	<span
		role="img"
		className="size-[7px] shrink-0 rounded-full bg-content-link"
		aria-label="Unread"
	/>
);

interface CardTitleProps {
	readonly value: string;
	readonly href: string | undefined;
	readonly renaming: boolean;
	readonly onRenamed: (title: string) => void;
	readonly onCancel: () => void;
}

/** Two-line card title; a link when the card is a single chat. */
const CardTitle: FC<CardTitleProps> = ({
	value,
	href,
	renaming,
	onRenamed,
	onCancel,
}) => {
	if (renaming) {
		return (
			<InlineInput
				value={value}
				onSave={onRenamed}
				onDone={onCancel}
				ariaLabel="card title"
				className="min-w-0 text-[14px] font-medium text-content-primary"
			/>
		);
	}
	const className =
		"line-clamp-2 min-w-0 text-[14px] font-medium leading-[19px] tracking-[-0.005em] text-content-primary wrap-anywhere [text-wrap:pretty]";
	return href ? (
		<Link to={href} className={cn(className, "no-underline")}>
			{value}
		</Link>
	) : (
		<span className={className}>{value}</span>
	);
};

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
					className="inline-flex h-4 shrink-0 items-center gap-1 rounded bg-content-primary/5 px-1.5 font-mono text-[11px] text-content-secondary no-underline hover:text-content-primary"
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
				"w-[300px] cursor-grabbing rounded-lg border border-content-link bg-surface-primary px-3 py-2 text-sm shadow-lg",
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

	return (
		<li
			ref={setNodeRef}
			className={cn(
				"group/row grid grid-cols-[14px_minmax(0,1fr)_auto] gap-x-2 rounded-md px-2 py-1.5 hover:bg-content-link/5",
				active && "bg-content-link/10 hover:bg-content-link/10",
				isDragging && "opacity-40",
			)}
		>
			{/* The status icon doubles as the drag handle so rows need no extra gutter. */}
			<span
				ref={draggable ? setActivatorNodeRef : undefined}
				{...(draggable
					? { ...dragHandleListeners(listeners), ...attributes }
					: {})}
				className={cn(
					"flex h-[18px] items-center justify-center",
					draggable && "cursor-grab touch-none active:cursor-grabbing",
				)}
				title={draggable ? "Drag to move this chat" : undefined}
			>
				<StatusIcon
					className={cn("size-[13px]", display.className)}
					aria-label={display.label}
				/>
			</span>
			<div className="min-w-0">
				{renaming ? (
					<InlineInput
						value={chat.title}
						onSave={onRename}
						onDone={() => setRenaming(false)}
						ariaLabel={`title of ${chat.title}`}
						className="w-full text-[13px] text-content-primary"
					/>
				) : (
					<Link
						to={`/agents/board/${chat.id}`}
						className="line-clamp-2 min-w-0 text-[13px] leading-[18px] text-content-primary no-underline wrap-anywhere"
					>
						{chat.title}
					</Link>
				)}
				<div className="mt-0.5">
					<ChatStatusLine chat={chat} />
				</div>
			</div>
			<div className="flex h-[18px] items-center gap-1.5">
				{chat.has_unread && <UnreadDot />}
				<span className="font-mono text-[11px] tabular-nums text-content-secondary/70">
					{shortRelativeTime(chat.updated_at)}
				</span>
				<ChatInfoPopover chat={chat} />
				<ActionsMenu
					label={chat.title}
					revealOn="row"
					onRename={() => setRenaming(true)}
				/>
			</div>
		</li>
	);
};
