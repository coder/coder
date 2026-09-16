import { cn } from "cn";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Markdown } from "#/components/Markdown/Markdown";
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
 * Notes record the operator's own understanding of a card ("waiting on
 * Mike", "repurposed to X"). They are not a conversation, so they read as a
 * plain list with a composer at the bottom.
 */
export const NotesSection: FC<NotesSectionProps> = ({
	notes,
	cardTitle,
	onAdd,
	onEdit,
	onRemove,
}) => (
	<div
		className="flex flex-col gap-3 border-t border-border px-3 pt-2.5 pb-2.5"
		// Typing and selecting text must not start a card drag.
		onPointerDown={(e) => e.stopPropagation()}
	>
		{notes.map((note) => (
			<Note
				key={note.index}
				note={note}
				onEdit={(text) => onEdit(note.index, text)}
				onRemove={() => onRemove(note.index)}
			/>
		))}
		<NoteComposer cardTitle={cardTitle} onSubmit={onAdd} />
	</div>
);

const NOTE_MARKDOWN_CLASS =
	"text-[13px] leading-snug text-content-primary wrap-anywhere [&_p]:m-0 [&_p+p]:mt-1 [&_ul]:my-0 [&_ol]:my-0 [&_ul]:pl-4 [&_ol]:pl-4 [&_pre]:my-1 [&_pre]:overflow-x-auto [&_pre]:text-xs";

interface NoteProps {
	readonly note: BoardNote;
	readonly onEdit: (text: string) => void;
	readonly onRemove: () => void;
}

const Note: FC<NoteProps> = ({ note, onEdit, onRemove }) => {
	const [mode, setMode] = useState<"view" | "edit" | "confirm-delete">("view");

	if (mode === "edit") {
		return (
			<NoteEditor
				initial={note.text}
				submitLabel="Save"
				onSubmit={(text) => {
					setMode("view");
					if (text !== note.text) onEdit(text);
				}}
				onCancel={() => setMode("view")}
			/>
		);
	}

	return (
		<div className="group/note flex flex-col gap-0.5">
			<Markdown className={NOTE_MARKDOWN_CLASS}>{note.text}</Markdown>
			<div className="flex h-5 items-center gap-2 text-xs text-content-secondary/70">
				<span className="tabular-nums">
					{note.timestamp ? shortRelativeTime(note.timestamp) : ""}
				</span>
				{mode === "confirm-delete" ? (
					<span className="flex items-center gap-2">
						Delete note?
						<NoteAction onClick={onRemove}>Delete</NoteAction>
						<NoteAction onClick={() => setMode("view")}>Cancel</NoteAction>
					</span>
				) : (
					<span className="flex items-center gap-2 opacity-0 group-hover/note:opacity-100 has-[:focus-visible]:opacity-100">
						<NoteAction onClick={() => setMode("edit")}>Edit</NoteAction>
						<NoteAction onClick={() => setMode("confirm-delete")}>
							Delete
						</NoteAction>
					</span>
				)}
			</div>
		</div>
	);
};

const NoteAction: FC<{
	readonly onClick: () => void;
	readonly children: string;
}> = ({ onClick, children }) => (
	<button
		type="button"
		className="border-0 bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary hover:underline"
		onClick={onClick}
	>
		{children}
	</button>
);

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
				className="w-full rounded-md border border-transparent bg-transparent px-2 py-1 text-left text-[13px] text-content-secondary/70 hover:border-border hover:text-content-secondary"
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
			autoFocus
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
	readonly autoFocus?: boolean;
	readonly onSubmit: (text: string) => void;
	readonly onCancel: () => void;
}

const NoteEditor: FC<NoteEditorProps> = ({
	initial,
	submitLabel,
	autoFocus,
	onSubmit,
	onCancel,
}) => {
	const [draft, setDraft] = useState(initial);
	const trimmed = draft.trim();
	const submit = () => {
		if (trimmed) onSubmit(trimmed);
	};
	// Grows with content instead of scrolling; capped so a long note does
	// not push the rest of the column away.
	const rows = Math.min(10, Math.max(2, draft.split("\n").length));

	return (
		<div className="flex flex-col gap-1.5">
			<textarea
				// biome-ignore lint/a11y/noAutofocus: the editor replaces the control the user just clicked.
				autoFocus={autoFocus ?? true}
				aria-label="Note text"
				placeholder="Add a note..."
				value={draft}
				rows={rows}
				className={cn(
					"w-full resize-none rounded-md border border-border bg-surface-secondary px-2 py-1.5 text-[13px] leading-snug text-content-primary outline-none",
					"placeholder:text-content-secondary/60 focus:border-content-link",
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
				<span className="ml-auto text-xs text-content-secondary/60">
					Markdown · Ctrl/Cmd+Enter to save
				</span>
			</div>
		</div>
	);
};
