import { ChevronUpIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { assistantChats } from "#/api/queries/chats";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";
import { ChatTreeNode } from "../tree/ChatTreeNode";
import { getSectionToggleTestId } from "./ChatSectionHeader";

const ASSISTANT_SECTION_KEY = "Assistant";
const EXPANDED_STORAGE_KEY = "coder_assistant_history_expanded";

function readStoredExpanded(): boolean {
	try {
		// Default to collapsed when no preference has been saved.
		return localStorage.getItem(EXPANDED_STORAGE_KEY) === "true";
	} catch {
		return false;
	}
}

function writeStoredExpanded(expanded: boolean): void {
	try {
		localStorage.setItem(EXPANDED_STORAGE_KEY, String(expanded));
	} catch {
		// Silently ignore storage errors (e.g. private browsing
		// quota exceeded).
	}
}

/**
 * Drawer docked at the bottom of the chats sidebar, directly above
 * the user footer, that holds Coder Assistant conversations (chats
 * labeled coder-assistant=true). Those chats are excluded from the main
 * browsing list and only fetched here, on demand, when the drawer is
 * expanded. The drawer opens upward: chat rows render above the
 * toggle row with their own internal scroll, so the drawer never
 * scrolls with the main chat list. Must be rendered inside a
 * ChatTreeContext so rows match the main list.
 */
export const AssistantChatsSection: FC = () => {
	const [expanded, setExpanded] = useState(readStoredExpanded);
	const toggleExpanded = () => {
		setExpanded((prev) => {
			const next = !prev;
			writeStoredExpanded(next);
			return next;
		});
	};
	const chatsQuery = useQuery({
		...assistantChats(),
		enabled: expanded,
	});
	const chats = chatsQuery.data ?? [];
	const actionLabel = expanded ? "Collapse" : "Expand";

	return (
		<div className="border-0 border-t border-solid border-border-default">
			{expanded && (
				<div className="max-h-[40vh] overflow-y-auto px-2 pt-2">
					{chatsQuery.isLoading ? (
						<div className="flex flex-col gap-0.5">
							{Array.from({ length: 3 }, (_, i) => (
								<div
									key={i}
									className="flex items-start gap-2 rounded-md px-2 py-1"
								>
									<Skeleton className="mt-0.5 size-5 shrink-0 rounded-md" />
									<div className="min-w-0 flex-1 space-y-1.5">
										<Skeleton
											className="h-3.5"
											style={{ width: `${55 + ((i * 17) % 35)}%` }}
										/>
										<Skeleton className="h-3 w-20" />
									</div>
								</div>
							))}
						</div>
					) : chats.length === 0 ? (
						<p className="m-0 ml-2.5 text-xs text-content-secondary">
							No assistant conversations yet
						</p>
					) : (
						<div className="flex flex-col gap-0.5">
							{chats.map((chat) => (
								<ChatTreeNode key={chat.id} chat={chat} isChildNode={false} />
							))}
						</div>
					)}
				</div>
			)}
			<div className="group/header mx-2 flex h-9 items-center pl-2.5 text-xs font-medium text-content-secondary">
				<button
					type="button"
					className="flex h-7 min-w-0 flex-1 cursor-pointer appearance-none items-center rounded-md border-0 bg-transparent p-0 text-left font-sans text-xs font-medium text-current focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link [@media(hover:hover)]:group-hover/header:text-content-primary"
					aria-expanded={expanded}
					aria-label={`${actionLabel} ${ASSISTANT_SECTION_KEY} section`}
					data-testid={getSectionToggleTestId(ASSISTANT_SECTION_KEY)}
					onClick={toggleExpanded}
				>
					<span className="min-w-0 flex-1 truncate">
						{chatsQuery.data
							? `${ASSISTANT_SECTION_KEY} (${chats.length})`
							: ASSISTANT_SECTION_KEY}
					</span>
					<span className="flex h-6 w-7 shrink-0 items-center justify-end">
						<ChevronUpIcon
							aria-hidden="true"
							className={cn(
								"size-3.5 text-current transition-transform",
								expanded && "rotate-180",
							)}
						/>
					</span>
				</button>
			</div>
		</div>
	);
};
