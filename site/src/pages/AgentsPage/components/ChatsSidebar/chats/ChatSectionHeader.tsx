import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "#/utils/cn";

export const PINNED_SECTION_KEY = "Pinned";

export const getSectionToggleTestId = (sectionKey: string) =>
	`agents-section-toggle-${sectionKey.replaceAll(" ", "-")}`;

interface ChatSectionHeaderProps {
	readonly label: string;
	readonly count: number;
	readonly expanded: boolean;
	readonly onToggle: () => void;
	readonly testId: string;
}

export const ChatSectionHeader: FC<ChatSectionHeaderProps> = ({
	label,
	count,
	expanded,
	onToggle,
	testId,
}) => {
	const actionLabel = expanded ? "Collapse" : "Expand";
	return (
		<div className="group/header mb-1 ml-2.5 mr-2 flex h-7 items-center text-xs font-medium text-content-secondary">
			<button
				type="button"
				className="flex h-7 min-w-0 flex-1 cursor-pointer appearance-none items-center rounded-md border-0 bg-transparent p-0 text-left font-sans text-xs font-medium text-current focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link [@media(hover:hover)]:group-hover/header:text-content-primary"
				aria-expanded={expanded}
				aria-label={`${actionLabel} ${label} section`}
				data-testid={testId}
				onClick={onToggle}
			>
				<span className="min-w-0 flex-1 truncate">
					{label} ({count})
				</span>
				<span className="flex h-6 w-7 shrink-0 items-center justify-end">
					<ChevronDownIcon
						aria-hidden="true"
						className={cn(
							"size-3.5 text-current transition-transform",
							expanded && "rotate-180",
						)}
					/>
				</span>
			</button>
		</div>
	);
};
