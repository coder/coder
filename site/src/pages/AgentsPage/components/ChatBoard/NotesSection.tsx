import { cn } from "cn";
import { PencilIcon, Trash2Icon } from "lucide-react";
import { type FC, useState } from "react";
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
 * Notes are the operator's status log for a card. Newest first with the
 * composer on top, so the latest understanding is the first thing read and
 * a new note lands where the cursor already is.
 */
export const NotesSection: FC<NotesSectionProps> = ({
	notes,
	cardTitle,
	onAdd,
	onEdit,
	onRemove,
}) => {
	const ordered = [...notes].sort((a, b) => b.timestamp - a.timestamp);
	return (
		<div
			className="flex flex-col gap-1.5 border-t border-border px-3 py-2"
			// Typing and selecting text must not start a card drag.
			onPointerDown={(e) => e.stopPropagation()}
		>
			<NoteComposer cardTitle={cardTitle} onSubmit={onAdd} />
			{ordered.map((note) => (
				<Note
					key={note.index}
					note={note}
					onEdit={(text) => onEdit(note.index, text)}
					onRemove={() => onRemove(note.index)}
				/>
			))}
		</div>
	);
};

const NOTE_MARKDOWN_CLASS =
	"text-[13px] leading-5 text-content-primary wrap-anywhere [&_p]:m-0 [&_p+p]:mt-1 [&_ul]:my-0 [&_ol]:my-0 [&_ul]:pl-4 [&_ol]:pl-4 [&_pre]:my-1 [&_pre]:overflow-x-auto [&_pre]:text-xs";

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
				submitLabel="Save"
				onSubmit={(text) => {
					setEditing(false);
					if (text !== note.text) onEdit(text);
				}}
				onCancel={() => setEditing(false)}
			/>
		);
	}

	// Three columns aligned to the first text line: fixed age, flexible body,
	// reserved action space so revealing the actions never shifts layout.
	return (
		<div className="group/note flex items-start gap-2">
			<span className="w-7 shrink-0 text-right text-xs leading-5 tabular-nums text-content-secondary">
				{note.timestamp ? shortRelativeTime(note.timestamp) : ""}
			</span>
			<Markdown className={cn("min-w-0 flex-1", NOTE_MARKDOWN_CLASS)}>
				{note.text}
			</Markdown>
			<span className="flex h-5 shrink-0 items-center opacity-0 group-hover/note:opacity-100 has-[:focus-visible]:opacity-100 has-[[data-state=open]]:opacity-100">
				<Button
					variant="subtle"
					size="icon"
					aria-label="Edit note"
					className="size-5 text-content-secondary"
					onClick={() => setEditing(true)}
				>
					<PencilIcon className="size-3.5" />
				</Button>
				<DeleteNoteButton onConfirm={onRemove} />
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
					className="size-5 text-content-secondary"
				>
					<Trash2Icon className="size-3.5" />
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

interface NoteComposerProps {
	readonly cardTitle: string;
	readonly onSubmit: (text: string) => void;
}

const NoteComposer: FC<NoteComposerProps> = ({ cardTitle, onSubmit }) => {
	const [expanded, setExpanded] = useState(false);
	if (!expanded) {
		return (
			<button
				type="button"
				aria-label={`Add a note to ${cardTitle}`}
				className="-mx-1 rounded-md border-0 bg-transparent px-1 py-0.5 text-left text-[13px] leading-5 text-content-secondary hover:bg-surface-tertiary/60 hover:text-content-primary"
				onClick={() => setExpanded(true)}
			>
				Add a note...
			</button>
		);
	}
	return (
		<NoteEditor
			initial=""
			submitLabel="Add note"
			onSubmit={(text) => {
				onSubmit(text);
				setExpanded(false);
			}}
			onCancel={() => setExpanded(false)}
		/>
	);
};

interface NoteEditorProps {
	readonly initial: string;
	readonly submitLabel: string;
	readonly onSubmit: (text: string) => void;
	readonly onCancel: () => void;
}

const NoteEditor: FC<NoteEditorProps> = ({
	initial,
	submitLabel,
	onSubmit,
	onCancel,
}) => {
	const [draft, setDraft] = useState(initial);
	const trimmed = draft.trim();
	const submit = () => {
		if (trimmed) onSubmit(trimmed);
	};
	// Grows with explicit lines; capped so a long note does not take over the column.
	const rows = Math.min(10, Math.max(2, draft.split("\n").length));

	return (
		<div className="flex flex-col gap-1.5">
			<textarea
				// biome-ignore lint/a11y/noAutofocus: the editor replaces the control the user just clicked.
				autoFocus
				aria-label="Note text"
				placeholder="Status, links, what changed..."
				value={draft}
				rows={rows}
				className={cn(
					"w-full resize-none rounded-md border border-border bg-surface-primary px-2 py-1.5 text-[13px] leading-5 text-content-primary outline-none",
					"placeholder:text-content-secondary focus:border-content-link",
				)}
				onChange={(e) => setDraft(e.target.value)}
				onKeyDown={(e) => {
					if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
						e.preventDefault();
						submit();
					}
					if (e.key === "Escape") onCancel();
				}}
			/>
			<div className="flex items-center gap-2">
				<Button size="sm" disabled={!trimmed} onClick={submit}>
					{submitLabel}
				</Button>
				<Button size="sm" variant="subtle" onClick={onCancel}>
					Cancel
				</Button>
				<span className="ml-auto text-xs text-content-secondary">
					Markdown · ⌘/Ctrl+Enter
				</span>
			</div>
		</div>
	);
};
