import { cn } from "cn";
import { type FC, useState } from "react";

type InlineEditProps = {
	readonly value: string;
	readonly onSave: (next: string) => void;
	readonly onDone: () => void;
	/** Typography of the text being edited, so the field is indistinguishable from it. */
	readonly className?: string;
	readonly ariaLabel: string;
	readonly placeholder?: string;
};

/**
 * Edits text where it stands: no box, the same font, sized by its content.
 * Enter or blur saves a changed non-empty value, Escape cancels.
 */
export const InlineEdit: FC<InlineEditProps> = ({
	value,
	onSave,
	onDone,
	className,
	ariaLabel,
	placeholder,
}) => {
	const [draft, setDraft] = useState(value);
	// Enter and blur commit; Escape only reports done. The parent unmounts
	// the field on done, and React fires no blur for an unmounted field, so
	// nothing commits after a cancel.
	const commit = () => {
		const next = draft.trim();
		onDone();
		if (next && next !== value) onSave(next);
	};

	return (
		<textarea
			// biome-ignore lint/a11y/noAutofocus: the field replaces the text the user just chose to edit.
			autoFocus
			rows={1}
			aria-label={ariaLabel}
			placeholder={placeholder}
			className={cn(
				"block w-full resize-none border-0 bg-transparent p-0 text-inherit outline-none [field-sizing:content] placeholder:text-content-secondary/60",
				className,
			)}
			value={draft}
			onChange={(e) => setDraft(e.target.value)}
			// Caret at the end, as if the user had clicked after the last word.
			onFocus={(e) => {
				const end = e.currentTarget.value.length;
				e.currentTarget.setSelectionRange(end, end);
			}}
			onBlur={commit}
			// Typing must not start a drag on the surrounding card.
			onPointerDown={(e) => e.stopPropagation()}
			onKeyDown={(e) => {
				if (e.key === "Enter") {
					e.preventDefault();
					commit();
				}
				if (e.key === "Escape") onDone();
			}}
		/>
	);
};
