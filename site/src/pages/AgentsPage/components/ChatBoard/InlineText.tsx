import { cn } from "cn";
import { PencilIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";

interface InlineInputProps {
	readonly value: string;
	readonly onSave: (next: string) => void;
	readonly onDone: () => void;
	readonly className?: string;
	readonly ariaLabel: string;
}

/** Autofocused input. Enter or blur saves a changed non-empty value, Escape cancels. */
export const InlineInput: FC<InlineInputProps> = ({
	value,
	onSave,
	onDone,
	className,
	ariaLabel,
}) => {
	const [draft, setDraft] = useState(value);
	// Escape unmounts the field, which can fire a trailing blur; ignore it.
	const cancelled = useRef(false);
	const commit = () => {
		if (cancelled.current) return;
		const next = draft.trim();
		onDone();
		if (next && next !== value) onSave(next);
	};
	const cancel = () => {
		cancelled.current = true;
		onDone();
	};

	return (
		<input
			// biome-ignore lint/a11y/noAutofocus: the input replaces the text the user just chose to edit.
			autoFocus
			aria-label={ariaLabel}
			className={cn(
				"min-w-0 rounded border border-border bg-surface-primary px-1 py-0 text-inherit outline-none focus:border-content-link",
				className,
			)}
			value={draft}
			onChange={(e) => setDraft(e.target.value)}
			onBlur={commit}
			// Typing must not start a drag on the surrounding card.
			onPointerDown={(e) => e.stopPropagation()}
			onKeyDown={(e) => {
				if (e.key === "Enter") commit();
				if (e.key === "Escape") cancel();
			}}
		/>
	);
};

interface EditableTextProps {
	readonly value: string;
	readonly onSave: (next: string) => void;
	readonly ariaLabel: string;
	readonly className?: string;
	/** Wrap onto multiple lines instead of truncating. */
	readonly wrap?: boolean;
	/** Tailwind group name whose hover reveals the pencil. */
	readonly revealOn: "card" | "column" | "row";
}

/**
 * The one edit convention on the board: text with a pencil that appears on
 * hover; the pencil opens an inline input. Text itself is never a click target.
 */
export const EditableText: FC<EditableTextProps> = ({
	value,
	onSave,
	ariaLabel,
	className,
	wrap = false,
	revealOn,
}) => {
	const [editing, setEditing] = useState(false);

	if (editing) {
		return (
			<InlineInput
				value={value}
				onSave={onSave}
				onDone={() => setEditing(false)}
				ariaLabel={ariaLabel}
				className={cn("flex-1", className)}
			/>
		);
	}

	return (
		<>
			<span
				className={cn(
					"min-w-0 flex-1",
					wrap ? "whitespace-normal wrap-anywhere" : "truncate",
					className,
				)}
			>
				{value}
			</span>
			<Button
				variant="subtle"
				size="icon"
				aria-label={`Edit ${ariaLabel}`}
				className={cn(
					"size-6 shrink-0 text-content-secondary opacity-0 focus-visible:opacity-100",
					revealOn === "card" && "group-hover/card:opacity-100",
					revealOn === "column" && "group-hover/column:opacity-100",
					revealOn === "row" && "group-hover/row:opacity-100",
				)}
				onPointerDown={(e) => e.stopPropagation()}
				onClick={() => setEditing(true)}
			>
				<PencilIcon className="size-3.5" />
			</Button>
		</>
	);
};
