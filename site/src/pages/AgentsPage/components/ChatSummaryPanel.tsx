import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so other tabs don't fetch every chat's cost. */
	isVisible: boolean;
};

export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
}) => {
	// chat() stays live via AgentsPage's watchChats merge; cost is fetched on demand.
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });
	const costQuery = useQuery({ ...chatCost(chatId), enabled: isVisible });

	const chatData = chatQuery.data;

	let content: ReactNode = null;
	if (chatQuery.isError) {
		content = <ErrorAlert error={chatQuery.error} />;
	} else if (chatData) {
		content = (
			<ChatSummary
				summary={chatData.summary}
				createdAt={chatData.created_at}
				updatedAt={chatData.updated_at}
				costMicros={costQuery.data?.total_cost_micros}
				unpricedMessageCount={costQuery.data?.unpriced_message_count}
				isCostLoading={costQuery.isLoading}
				costError={costQuery.isError}
			/>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto p-4">
			{content}
		</div>
	);
};
