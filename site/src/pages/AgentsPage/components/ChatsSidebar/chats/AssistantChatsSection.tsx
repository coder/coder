import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { assistantChats } from "#/api/queries/chats";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { ChatTreeNode } from "../tree/ChatTreeNode";
import { ChatSectionHeader, getSectionToggleTestId } from "./ChatSectionHeader";

const ASSISTANT_SECTION_KEY = "Assistant";
const EXPANDED_STORAGE_KEY = "coder_agent_history_expanded";

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
 * Collapsible drawer at the bottom of the chats sidebar that holds
 * Coder Assistant conversations (chats labeled coder-agent=true).
 * Those chats are excluded from the main browsing list and only
 * fetched here, on demand, when the drawer is expanded. Must be
 * rendered inside a ChatTreeContext so rows match the main list.
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

	return (
		<div className="[&:not(:first-child)]:mt-3">
			<ChatSectionHeader
				label={ASSISTANT_SECTION_KEY}
				count={chatsQuery.data ? chats.length : undefined}
				expanded={expanded}
				onToggle={toggleExpanded}
				testId={getSectionToggleTestId(ASSISTANT_SECTION_KEY)}
			/>
			{expanded &&
				(chatsQuery.isLoading ? (
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
				))}
		</div>
	);
};
