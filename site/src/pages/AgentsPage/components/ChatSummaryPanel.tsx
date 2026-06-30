import type { FC } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so other tabs don't fetch every chat's cost. */
	isVisible: boolean;
};

/** Data wrapper for the Summary tab; owns the React Query reads so ChatSummary stays presentational. */
export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
}) => {
	// chat() stays live via AgentsPage's watchChats merge; cost is fetched on demand.
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });
	const costQuery = useQuery({ ...chatCost(chatId), enabled: isVisible });

	const chatData = chatQuery.data;

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
			{chatQuery.isError ? (
				<ErrorAlert error={chatQuery.error} />
			) : chatData ? (
				<ChatSummary
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
