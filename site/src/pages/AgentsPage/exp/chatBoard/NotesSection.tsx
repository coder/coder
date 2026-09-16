import { cn } from "cn";
import { ArrowUpIcon, PencilIcon, Trash2Icon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Markdown } from "#/components/Markdown/Markdown";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { shortRelativeTime } from "#/utils/time";
import type { BoardNote } from "./boardLabels";

interface NotesSectionProps {
	readonly notes: readonly BoardNote[];
	readonly cardTitle: string;
	readonly onAdd: (text: string) => void;
	readonly onEdit: (index: number, text: string) => void;
	readonly onRemove: (index: number) => void;
}

/**
 * Notes are the operator's status log for a card, oldest first, with a
 * plain composer line at the bottom that appends.
 */
export const NotesSection: FC<NotesSectionProps> = ({
	notes,
	cardTitle,
	onAdd,
	onEdit,
	onRemove,
}) => {
	// Oldest first, so the composer line below continues the log.
	const ordered = [...notes].sort((a, b) => a.timestamp - b.timestamp);
	return (
		// Always present, so the divider above is stable and edge to edge.
		// Typing and selecting text must not start a card drag.
		<div
			className="flex flex-col border-t border-border px-3 py-1"
			onPointerDown={(e) => e.stopPropagation()}
		>
			{ordered.map((note) => (
				<Note
					key={note.index}
					note={note}
					onEdit={(text) => onEdit(note.index, text)}
					onRemove={() => onRemove(note.index)}
				/>
			))}
			<NoteEditor
				key={notes.length}
				initial=""
				placeholder="Add a note..."
				ariaLabel={`Add a note to ${cardTitle}`}
				onSubmit={onAdd}
			/>
		</div>
	);
};

/** Tightens Markdown block spacing for small text inside a card or popover. */
export const COMPACT_MARKDOWN_CLASS =
	"wrap-anywhere [text-wrap:pretty] [&_p]:mt-0 [&_p]:mb-0 [&_p+p]:mt-1 [&_ul]:my-1 [&_ol]:my-1 [&_ul]:gap-0.5 [&_ol]:gap-0.5 [&_ul]:list-disc [&_ol]:list-decimal [&_ul]:pl-4 [&_ol]:pl-4 [&_li>ul]:mt-0.5 [&_li>ol]:mt-0.5 [&_code]:text-[length:inherit] [&_pre]:my-1 [&_pre]:overflow-x-auto [&_pre]:text-[11px]";

const NOTE_MARKDOWN_CLASS = cn(
	"text-xs leading-[17px] text-content-primary/80",
	COMPACT_MARKDOWN_CLASS,
);

interface NoteProps {
	readonly note: BoardNote;
	readonly onEdit: (text: string) => void;
	readonly onRemove: () => void;
}

const Note: FC<NoteProps> = ({ note, onEdit, onRemove }) => {
	const [editing, setEditing] = useState(false);

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
	return (
		<div className="group/note flex items-start gap-2.5 py-[3px]">
			<Markdown className={cn("min-w-0 flex-1", NOTE_MARKDOWN_CLASS)}>
				{note.text}
			</Markdown>
			<span className="relative h-[17px] w-10 shrink-0">
				<span className="absolute inset-0 flex items-center justify-end text-[11px] tabular-nums text-content-secondary/70 group-hover/note:hidden group-has-[[data-state=open]]/note:hidden">
					{note.timestamp ? shortRelativeTime(note.timestamp) : ""}
				</span>
				<span className="-mr-1 absolute inset-0 hidden items-center justify-end gap-0.5 group-hover/note:flex group-has-[[data-state=open]]/note:flex has-[:focus-visible]:flex">
					<Button
						variant="subtle"
						size="icon"
						aria-label="Edit note"
						className="size-[18px] text-content-secondary"
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

const DeleteNoteButton: FC<{ readonly onConfirm: () => void }> = ({
	onConfirm,
}) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Delete note"
					className="size-[18px] text-content-secondary hover:bg-surface-destructive hover:text-content-destructive"
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

interface NoteEditorProps {
	readonly initial: string;
	readonly ariaLabel: string;
	readonly placeholder?: string;
	readonly onSubmit: (text: string) => void;
	/** Absent for the composer, which just clears on Escape. */
	readonly onCancel?: () => void;
}

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
	// Escape may unmount the field, which can fire a trailing blur; ignore it.
	const cancelled = useRef(false);
	const commit = () => {
		if (cancelled.current) return;
		const text = draft.trim();
		if (text) onSubmit(text);
		else onCancel?.();
	};
	const cancel = () => {
		if (onCancel) {
			cancelled.current = true;
			onCancel();
		} else {
			setDraft("");
		}
	};
	// Sized by its content, so a wrapped note keeps its shape while edited;
	// capped so a long note does not take over the column.
	const composer = onCancel === undefined;
	return (
		<div className="-mx-1 flex items-start gap-1.5">
			<textarea
				// biome-ignore lint/a11y/noAutofocus: an existing note's editor replaces the text the user chose to edit.
				autoFocus={!composer}
				aria-label={ariaLabel}
				placeholder={placeholder}
				value={draft}
				rows={1}
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
					if (e.key === "Escape") cancel();
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
