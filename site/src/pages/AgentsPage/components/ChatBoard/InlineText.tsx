import { cn } from "cn";
import { type FC, useState } from "react";

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
	const commit = () => {
		const next = draft.trim();
		onDone();
		if (next && next !== value) onSave(next);
	};

	return (
		<input
			// biome-ignore lint/a11y/noAutofocus: the input replaces the text the user just clicked.
			autoFocus
			aria-label={ariaLabel}
			className={cn(
				"min-w-0 rounded border border-border bg-surface-primary px-1 py-0 text-inherit outline-none",
				className,
			)}
			value={draft}
			onChange={(e) => setDraft(e.target.value)}
			onBlur={commit}
			// Typing must not start a drag on the surrounding card.
			onPointerDown={(e) => e.stopPropagation()}
			onKeyDown={(e) => {
				if (e.key === "Enter") commit();
				if (e.key === "Escape") onDone();
			}}
		/>
	);
};

interface InlineTextProps {
	readonly value: string;
	readonly onSave: (next: string) => void;
	readonly className?: string;
	readonly ariaLabel: string;
}

/** Text that turns into an InlineInput on click. */
export const InlineText: FC<InlineTextProps> = ({
	value,
	onSave,
	className,
	ariaLabel,
}) => {
	const [editing, setEditing] = useState(false);

	if (editing) {
		return (
			<InlineInput
				value={value}
				onSave={onSave}
				onDone={() => setEditing(false)}
				ariaLabel={ariaLabel}
				className={className}
			/>
		);
	}

	return (
		<button
			type="button"
			aria-label={`Edit ${ariaLabel}`}
			className={cn(
				"min-w-0 truncate rounded border-0 bg-transparent p-0 text-left text-inherit hover:underline decoration-dotted",
				className,
			)}
			onClick={() => setEditing(true)}
			onPointerDown={(e) => e.stopPropagation()}
		>
			{value}
		</button>
	);
};
