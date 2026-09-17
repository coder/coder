import { cn } from "cn";
import type { FC } from "react";
import { InlineEdit } from "./InlineEdit";

interface EditableTitleProps {
	readonly value: string;
	readonly renaming: boolean;
	/** The chat is open in a floating window; the title alone marks it, no chrome. */
	readonly open: boolean;
	readonly className: string;
	readonly onEdit: () => void;
	readonly onRenamed: (title: string) => void;
	readonly onCancel: () => void;
}

/** Two-line title; clicking the text (only the text) edits it in place. */
export const EditableTitle: FC<EditableTitleProps> = ({
	value,
	renaming,
	open,
	className,
	onEdit,
	onRenamed,
	onCancel,
}) => {
	const textClass = cn(className, open && "font-medium text-highlight-purple");
	if (renaming) {
		return (
			<InlineEdit
				value={value}
				onSave={onRenamed}
				onDone={onCancel}
				ariaLabel="title"
				className={cn("relative z-[1] text-content-primary", textClass)}
			/>
		);
	}
	return (
		<button
			type="button"
			title="Click to rename"
			className={cn(
				"relative z-[1] m-0 w-fit max-w-full min-w-0 cursor-text justify-self-start border-0 bg-transparent p-0 text-left text-content-primary",
				textClass,
			)}
			onClick={onEdit}
		>
			<span className="line-clamp-2 wrap-anywhere [text-wrap:pretty]">
				{value}
			</span>
		</button>
	);
};
