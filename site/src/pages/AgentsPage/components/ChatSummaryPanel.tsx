import type { FC } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/**
	 * Gate the reads on tab visibility so opening a different right-panel tab
	 * does not fetch the cost for every chat. Cached chat data still renders
	 * immediately when the tab becomes visible.
	 */
	isVisible: boolean;
};

/**
 * ChatSummaryPanel is the data wrapper for the Summary right-panel tab. It owns
 * the React Query reads so ChatSummary stays presentational, and renders error,
 * loading, and empty states inline.
 */
export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
}) => {
	// Created/Updated come from the cached chat query, which the AgentsPage keeps
	// live via its watchChats merge. A background refetch may still run when the
	// tab becomes visible, but cached data renders without a loading flash.
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });
	// Cost is dynamic and not part of the cached chat, so it is fetched on demand
	// only while the Summary tab is visible.
	const costQuery = useQuery({ ...chatCost(chatId), enabled: isVisible });

	const chatData = chatQuery.data;

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
			{chatQuery.isError ? (
				<ErrorAlert error={chatQuery.error} />
			) : chatData ? (
				<ChatSummary
					// The persisted whole-chat summary is generated asynchronously and
					// may be null until the first summary is produced; the component
					// renders a muted empty state in that case.
					summary={chatData.summary}
					createdAt={chatData.created_at}
					updatedAt={chatData.updated_at}
					costMicros={costQuery.data?.total_cost_micros}
					unpricedMessageCount={costQuery.data?.unpriced_message_count}
					isCostLoading={costQuery.isLoading}
					costError={costQuery.isError}
				/>
			) : (
				<div
					role="status"
					className="flex flex-1 items-center justify-center text-content-secondary"
				>
					<Spinner loading />
				</div>
			)}
		</div>
	);
};
