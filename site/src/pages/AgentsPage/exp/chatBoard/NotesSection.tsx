import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import {
	ArrowUpIcon,
	GripVerticalIcon,
	PencilIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { shortRelativeTime } from "#/utils/time";
import type { DragData, DropData } from "./BoardCard";
import type { NoteSlot } from "./boardApi";
import type { BoardCard, BoardNote } from "./boardLabels";
import { CompactMarkdown } from "./CompactMarkdown";
import { dragHandleListeners } from "./dragHandle";

type NotesSectionProps = {
	readonly card: BoardCard;
	/** Where a dragged note would land in this card, if it is over one of these notes. */
	readonly noteDrop: NoteSlot | undefined;
	readonly onAdd: (text: string) => void;
	readonly onEdit: (index: number, text: string) => void;
	readonly onRemove: (index: number) => void;
};

/**
 * Notes are the operator's status log for a card, in index order, with a
 * plain composer line at the bottom that appends. Notes drag to reorder
 * within the card or to move to another card.
 */
export const NotesSection: FC<NotesSectionProps> = ({
	card,
	noteDrop,
	onAdd,
	onEdit,
	onRemove,
}) => {
	const notes = card.comments;
	const keys = noteKeys(notes);
	return (
		// Always present, so the divider above is stable and edge to edge.
		// Typing and selecting text must not start a card drag.
		<div
			className="flex flex-col border-t border-border px-3 py-1"
			onPointerDown={(e) => e.stopPropagation()}
		>
			{notes.map((note, i) => (
				<Note
					key={keys[i]}
					card={card}
					note={note}
					dropSide={noteDrop?.index === note.index ? noteDrop.side : undefined}
					onEdit={(text) => onEdit(note.index, text)}
					onRemove={() => onRemove(note.index)}
				/>
			))}
			<NoteEditor
				initial=""
				placeholder="Add a note..."
				ariaLabel={`Add a note to ${card.title}`}
				onSubmit={onAdd}
			/>
		</div>
	);
};

type NoteProps = {
	readonly card: BoardCard;
	readonly note: BoardNote;
	readonly dropSide: "before" | "after" | undefined;
	readonly onEdit: (text: string) => void;
	readonly onRemove: () => void;
};

// Notes have no id. Reordering renumbers indices, so an index key would hand
// an open editor to a different note; the timestamp survives edits and moves,
// so it is the key. Equal timestamps (notes stored without one) are told
// apart by rank, which is display order for those notes only.
const noteKeys = (notes: readonly BoardNote[]): string[] => {
	const seen = new Map<number, number>();
	return notes.map((note) => {
		const rank = seen.get(note.timestamp) ?? 0;
		seen.set(note.timestamp, rank + 1);
		return `${note.timestamp}.${rank}`;
	});
};

const Note: FC<NoteProps> = ({ card, note, dropSide, onEdit, onRemove }) => {
	const [editing, setEditing] = useState(false);
	const dragData: DragData = { type: "note", card, note };
	const dropData: DropData = { type: "note", card, note };
	const {
		setNodeRef: setDragRef,
		setActivatorNodeRef,
		listeners,
		attributes,
		isDragging,
	} = useDraggable({ id: `note:${card.id}:${note.index}`, data: dragData });
	const { setNodeRef: setDropRef } = useDroppable({
		id: `drop-note:${card.id}:${note.index}`,
		data: dropData,
	});
	const setRefs = (node: HTMLElement | null) => {
		setDragRef(node);
		setDropRef(node);
	};

	if (editing) {
		return (
			<NoteEditor
				initial={note.text}
				ariaLabel="Note text"
				onSubmit={(text) => {
					setEditing(false);
					if (text !== note.text) onEdit(text);
				}}
				onCancel={() => setEditing(false)}
			/>
		);
	}

	// Age on the right; hover swaps it for the actions without moving text.
	// The drop indicator is an inset shadow so the list does not shift.
	return (
		<div
			ref={setRefs}
			className={cn(
				"group/note flex items-start gap-2.5 py-[3px]",
				isDragging && "opacity-40",
				dropSide === "before" &&
					"shadow-[inset_0_2px_0_0_var(--color-content-link)]",
				dropSide === "after" &&
					"shadow-[inset_0_-2px_0_0_var(--color-content-link)]",
			)}
		>
			<CompactMarkdown className="min-w-0 flex-1 text-xs leading-[17px] text-content-primary/80">
				{note.text}
			</CompactMarkdown>
			<span className="relative h-[17px] w-12 shrink-0">
				<span className="absolute inset-0 flex items-center justify-end text-[11px] tabular-nums text-content-secondary/70 group-hover/note:hidden group-has-[[data-state=open]]/note:hidden">
					{note.timestamp ? shortRelativeTime(note.timestamp) : ""}
				</span>
				<span className="-mr-1 absolute inset-0 hidden items-center justify-end group-hover/note:flex group-has-[[data-state=open]]/note:flex has-[:focus-visible]:flex">
					<button
						type="button"
						ref={setActivatorNodeRef}
						{...dragHandleListeners(listeners)}
						{...attributes}
						aria-label="Drag note"
						title="Drag to reorder or move to another card"
						className="grid size-4 cursor-grab touch-none place-items-center rounded border-0 bg-transparent p-0 text-content-secondary active:cursor-grabbing"
					>
						<GripVerticalIcon className="size-3" />
					</button>
					<Button
						variant="subtle"
						size="icon"
						aria-label="Edit note"
						className="size-4 text-content-secondary"
						onClick={() => setEditing(true)}
					>
						<PencilIcon className="size-3" />
					</Button>
					<DeleteNoteButton onConfirm={onRemove} />
				</span>
			</span>
		</div>
	);
};

type DeleteNoteButtonProps = {
	readonly onConfirm: () => void;
};

const DeleteNoteButton: FC<DeleteNoteButtonProps> = ({ onConfirm }) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Delete note"
					className="size-4 text-content-secondary hover:bg-surface-destructive hover:text-content-destructive"
				>
					<Trash2Icon className="size-3" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="flex w-auto items-center gap-2 p-2 text-xs"
			>
				Delete this note?
				<Button
					size="sm"
					variant="destructive"
					onClick={() => {
						setOpen(false);
						onConfirm();
					}}
				>
					Delete
				</Button>
			</PopoverContent>
		</Popover>
	);
};

type NoteEditorProps = {
	readonly initial: string;
	readonly ariaLabel: string;
	readonly placeholder?: string;
	readonly onSubmit: (text: string) => void;
	/** Absent for the composer, which just clears on Escape. */
	readonly onCancel?: () => void;
};

// Same keys as every other inline edit on the board: Enter saves, Escape
// cancels, leaving the field saves. Shift+Enter inserts a newline. Editing
// looks exactly like composing: the text stays in place, no box appears.
const NoteEditor: FC<NoteEditorProps> = ({
	initial,
	ariaLabel,
	placeholder,
	onSubmit,
	onCancel,
}) => {
	const [draft, setDraft] = useState(initial);
	const composer = onCancel === undefined;
	// Enter, blur and the Save button commit; Escape cancels an existing
	// note's editor and empties the composer. The parent unmounts the editor
	// on cancel or submit, and React fires no blur for an unmounted field,
	// so nothing commits twice. The composer stays mounted and clears itself.
	const commit = () => {
		const text = draft.trim();
		if (text) {
			onSubmit(text);
			if (composer) setDraft("");
		} else {
			onCancel?.();
		}
	};
	return (
		<div className="-mx-1 flex items-start gap-1.5">
			<textarea
				// biome-ignore lint/a11y/noAutofocus: an existing note's editor replaces the text the user chose to edit.
				autoFocus={!composer}
				aria-label={ariaLabel}
				placeholder={placeholder}
				value={draft}
				rows={1}
				// Sized by its content, so a wrapped note keeps its shape while
				// edited; capped so a long note does not take over the column.
				className={cn(
					"block max-h-60 min-w-0 flex-1 resize-none border-0 bg-transparent px-1 text-xs leading-[17px] text-content-primary/80 outline-none [field-sizing:content] placeholder:text-content-secondary/60",
					composer ? "py-1.5" : "py-[3px]",
				)}
				onChange={(e) => setDraft(e.target.value)}
				onBlur={commit}
				onKeyDown={(e) => {
					if (e.key === "Enter" && !e.shiftKey) {
						e.preventDefault();
						commit();
					}
					if (e.key === "Escape") {
						if (onCancel) onCancel();
						else setDraft("");
					}
				}}
			/>
			<button
				type="button"
				aria-label="Save note"
				className={cn(
					"grid size-5 shrink-0 place-items-center rounded-[5px] border-0 bg-transparent p-0 text-content-secondary/40 hover:bg-content-primary hover:text-surface-primary",
					composer ? "mt-1" : "mt-px",
				)}
				// Keep focus in the textarea, or its blur would commit first.
				onPointerDown={(e) => e.preventDefault()}
				onClick={commit}
			>
				<ArrowUpIcon className="size-3" strokeWidth={2.4} />
			</button>
		</div>
	);
};
