import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";

export const PINNED_SECTION_KEY = "Pinned";

export const getSectionToggleTestId = (sectionKey: string) =>
	`agents-section-toggle-${sectionKey.replaceAll(" ", "-")}`;

type ChatSectionHeaderProps = {
	readonly label: string;
	readonly count: number;
	readonly expanded: boolean;
	readonly onToggle: () => void;
	readonly testId: string;
	/** Shows an unread marker while the section is collapsed. */
	readonly hasUnread?: boolean;
};

export const ChatSectionHeader: FC<ChatSectionHeaderProps> = ({
	label,
	count,
	expanded,
	onToggle,
	testId,
	hasUnread = false,
}) => {
	const actionLabel = expanded ? "Collapse" : "Expand";
	return (
		<div className="group/header flex h-7 items-center pl-1 pr-2 text-content-secondary">
			<button
				type="button"
				className="flex h-7 min-w-0 flex-1 cursor-pointer appearance-none items-center gap-1.5 rounded-md border-0 bg-transparent p-0 text-left font-sans text-[13px] text-current focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link [@media(hover:hover)]:group-hover/header:text-content-primary"
				aria-expanded={expanded}
				aria-label={`${actionLabel} ${label} section`}
				data-testid={testId}
				onClick={onToggle}
			>
				<span className="flex size-5 shrink-0 items-center justify-center">
					<ChevronRightIcon
						aria-hidden="true"
						className={cn(
							"size-3.5 text-current transition-transform",
							expanded && "rotate-90",
						)}
					/>
				</span>
				<span className="min-w-0 truncate text-content-primary/85 [@media(hover:hover)]:group-hover/header:text-content-primary">
					{label}
				</span>
				<span className="shrink-0 tabular-nums"> ({count})</span>
				{hasUnread && !expanded && (
					<span
						className="ml-1 size-2 shrink-0 rounded-full bg-content-link"
						aria-hidden="true"
					/>
				)}
			</button>
		</div>
	);
};
