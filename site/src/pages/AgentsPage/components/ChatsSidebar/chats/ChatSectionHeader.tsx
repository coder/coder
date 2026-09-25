import { cn } from "cn";
import { ChevronDownIcon } from "lucide-react";
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
	/** Flags a collapsed section that hides unread chats. */
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
	const showUnreadDot = hasUnread && !expanded;
	return (
		// The chevron centers on the section's guide line below it.
		<div className="group/header mb-1 ml-1.5 mr-2 flex h-7 items-center text-xs font-medium text-content-secondary">
			<button
				type="button"
				className="flex h-7 min-w-0 flex-1 cursor-pointer appearance-none items-center gap-1.5 rounded-md border-0 bg-transparent p-0 text-left font-sans text-xs font-medium text-current focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link [@media(hover:hover)]:group-hover/header:text-content-primary"
				aria-expanded={expanded}
				aria-label={`${actionLabel} ${label} section${showUnreadDot ? ", has unread chats" : ""}`}
				data-testid={testId}
				onClick={onToggle}
			>
				<ChevronDownIcon
					aria-hidden="true"
					className={cn(
						"size-3.5 shrink-0 text-current transition-transform",
						!expanded && "-rotate-90",
					)}
				/>
				<span className="min-w-0 truncate">
					{label} ({count})
				</span>
				{showUnreadDot && (
					<span
						aria-hidden="true"
						className="size-1.5 shrink-0 self-start mt-1.5 rounded-full bg-content-link"
					/>
				)}
			</button>
		</div>
	);
};
