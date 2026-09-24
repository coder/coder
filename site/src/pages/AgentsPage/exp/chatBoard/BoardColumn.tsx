import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cva } from "class-variance-authority";
import { cn } from "cn";
import { Trash2Icon } from "lucide-react";
import { type FC, useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import { ActionsMenu } from "./ActionsMenu";
import {
	BoardCard,
	type ChatOpenHandlers,
	type DragData,
	type DropData,
} from "./BoardCard";
import type { DropTarget } from "./boardDrag";
import type {
	BoardCard as BoardCardModel,
	BoardColumn as BoardColumnModel,
	CardColor,
} from "./boardLabels";
import { columnHue, INBOX_COLUMN } from "./boardLabels";
import { dragHandleListeners } from "./dragHandle";
import { InlineEdit } from "./InlineEdit";

const columnShell = cva("relative flex min-h-0 w-[300px] shrink-0 flex-col", {
	variants: { dragging: { true: "opacity-40" } },
});

// The header is its own hover group so its controls do not light up while
// hovering cards below it. It is also the handle for reordering columns.
const columnHeader = cva(
	"group/column flex items-center gap-2 px-1.5 pt-0.5 pb-2.5 text-[13px] font-medium text-content-primary",
	{
		variants: {
			draggable: { true: "cursor-grab touch-none active:cursor-grabbing" },
		},
	},
);

type BoardColumnProps = {
	readonly column: BoardColumnModel;
	readonly openChatIds: ReadonlySet<string>;
	readonly dropTarget: DropTarget | null;
	readonly onRename: (to: string) => void;
	readonly onDelete: () => void;
	readonly onSetCardTitle: (card: BoardCardModel, title: string) => void;
	readonly onSetCardColor: (
		card: BoardCardModel,
		color: CardColor | undefined,
	) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAssistant: (card: BoardCardModel) => void;
	readonly onRemoveFromGroup: (chat: Chat, card: BoardCardModel) => void;
	readonly onAddNote: (card: BoardCardModel, text: string) => void;
	readonly onEditNote: (
		card: BoardCardModel,
		index: number,
		text: string,
	) => void;
	readonly onRemoveNote: (card: BoardCardModel, index: number) => void;
} & ChatOpenHandlers;

export const BoardColumn: FC<BoardColumnProps> = ({
	column,
	openChatIds,
	dropTarget,
	onRename,
	onDelete,
	onSetCardTitle,
	onSetCardColor,
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
	const dropData: DropData = { type: "column", name: column.name };
	const { setNodeRef } = useDroppable({
		id: `column:${column.name}`,
		data: dropData,
	});
	const dragData: DragData = { type: "column", name: column.name };
	// The header is both the draggable node and its handle; without a node
	// rect dnd-kit measures nothing and never runs collision detection.
	const {
		setNodeRef: setDragNodeRef,
		setActivatorNodeRef,
		listeners,
		attributes,
		isDragging,
	} = useDraggable({ id: `column-drag:${column.name}`, data: dragData });
	const setHeaderRefs = (node: HTMLElement | null) => {
		setDragNodeRef(node);
		setActivatorNodeRef(node);
	};
	const isInbox = column.name === INBOX_COLUMN;
	const insertBefore =
		dropTarget?.kind === "insert" && dropTarget.column === column.name
			? dropTarget.beforeCardId
			: undefined;
	// A note dropped on a card (not on one of its notes) is appended; the
	// card shows the same ring as a merge target.
	const mergeTargetId =
		dropTarget?.kind === "merge" || dropTarget?.kind === "noteCard"
			? dropTarget.card.id
			: undefined;
	const noteDrop =
		dropTarget?.kind === "note"
			? { card: dropTarget.card.id, slot: dropTarget.slot }
			: undefined;
	const columnSide =
		dropTarget?.kind === "column" && dropTarget.name === column.name
			? dropTarget.side
			: undefined;
	const [renaming, setRenaming] = useState(false);

	// Same convention as card titles: click the text to rename it. Inbox is
	// the implicit column and keeps its name.
	let title = (
		<button
			type="button"
			title="Click to rename"
			className="m-0 max-w-full min-w-0 cursor-text truncate border-0 bg-transparent p-0 text-left text-inherit"
			onClick={() => setRenaming(true)}
		>
			{column.name}
		</button>
	);
	if (renaming) {
		title = (
			<InlineEdit
				value={column.name}
				onSave={onRename}
				onDone={() => setRenaming(false)}
				ariaLabel={`${column.name} column name`}
				className="flex-1"
			/>
		);
	} else if (isInbox) {
		title = <span className="min-w-0 flex-1 truncate">{column.name}</span>;
	}

	return (
		<section
			ref={setNodeRef}
			aria-label={`${column.name} column`}
			className={columnShell({ dragging: isDragging })}
		>
			{/* Occupies the column gap, so showing it does not shift layout. */}
			{columnSide && (
				<div
					className={cn(
						"absolute inset-y-0 w-0.5 rounded bg-content-link",
						columnSide === "before" ? "-left-[9px]" : "-right-[9px]",
					)}
				/>
			)}
			<header
				ref={setHeaderRefs}
				className={columnHeader({ draggable: true })}
				{...dragHandleListeners(listeners)}
				{...attributes}
			>
				<ColumnDot name={column.name} />
				{title}
				<span className="ml-auto text-[11px] text-content-secondary/70 tabular-nums">
					{column.cards.length}
				</span>
				{!isInbox && (
					<ActionsMenu
						label={`${column.name} column`}
						items={[
							{
								label: "Delete column",
								icon: Trash2Icon,
								destructive: true,
								onSelect: onDelete,
							},
						]}
					/>
				)}
			</header>
			<div className="flex min-h-16 flex-1 flex-col overflow-y-auto pb-2">
				{column.cards.map((card) => (
					<div key={card.id} className="flex flex-col">
						<InsertionLine visible={insertBefore === card.id} />
						<BoardCard
							card={card}
							openChatIds={openChatIds}
							isMergeTarget={mergeTargetId === card.id}
							noteDrop={noteDrop?.card === card.id ? noteDrop.slot : undefined}
							onSetTitle={(title) => onSetCardTitle(card, title)}
							onSetColor={(color) => onSetCardColor(card, color)}
							onRenameChat={onRenameChat}
							onAssistant={() => onAssistant(card)}
							onRemoveFromGroup={(chat) => onRemoveFromGroup(chat, card)}
							onOpen={onOpen}
							onPreview={onPreview}
							onPreviewEnd={onPreviewEnd}
							onAddNote={(text) => onAddNote(card, text)}
							onEditNote={(index, text) => onEditNote(card, index, text)}
							onRemoveNote={(index) => onRemoveNote(card, index)}
						/>
					</div>
				))}
				<InsertionLine visible={insertBefore === null} />
			</div>
		</section>
	);
};

const dot = cva("size-2 shrink-0 rounded-[2px]", {
	variants: {
		hue: {
			neutral: "bg-content-secondary/40",
			purple: "bg-highlight-purple",
			sky: "bg-highlight-sky",
			green: "bg-highlight-green",
			orange: "bg-highlight-orange",
			magenta: "bg-highlight-magenta",
			red: "bg-highlight-red",
		},
	},
});

type ColumnDotProps = {
	readonly name: string;
};

const ColumnDot: FC<ColumnDotProps> = ({ name }) => (
	<span className={dot({ hue: columnHue(name) })} />
);

type InsertionLineProps = {
	readonly visible: boolean;
};

// Occupies the gap between cards, so showing it does not shift layout.
const InsertionLine: FC<InsertionLineProps> = ({ visible }) => (
	<div className="flex h-2.5 items-center">
		<div
			className={cn(
				"h-0.5 w-full rounded bg-content-link",
				!visible && "invisible",
			)}
		/>
	</div>
);

type NewColumnProps = {
	readonly onCreate: (name: string) => void;
	readonly onCancel: () => void;
};

/** A column shell with its title in edit mode, so creating looks like renaming. */
export const NewColumn: FC<NewColumnProps> = ({ onCreate, onCancel }) => (
	<section
		aria-label="New column"
		className={columnShell({ className: "min-h-24" })}
	>
		<header className={columnHeader()}>
			<span className="size-2 shrink-0 rounded-[2px] bg-content-secondary/40" />
			<InlineEdit
				value=""
				placeholder="Column name"
				ariaLabel="New column name"
				className="flex-1"
				onSave={onCreate}
				onDone={onCancel}
			/>
		</header>
	</section>
);
