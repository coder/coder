import { cn } from "cn";
import { type FC, useRef, useState } from "react";

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
