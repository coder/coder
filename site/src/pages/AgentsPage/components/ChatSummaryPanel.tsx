import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so the chat and cost queries don't run while the tab is hidden. */
	isVisible: boolean;
	/** Set only after the server reports that summary generation started. */
	isGenerating?: boolean;
};

export const ChatSummaryPanel: FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
	isGenerating,
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });

	const chatData = chatQuery.data;
	const rootChatId = getChatCostTreeID(chatData) ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});

	let content: ReactNode = null;
	if (chatQuery.isLoading) {
		content = (
			<div className="space-y-4 p-4">
				<Skeleton aria-label="Loading summary" className="h-4 w-3/4" />
				<Skeleton aria-hidden className="h-3 w-full" />
				<Skeleton aria-hidden className="h-3 w-2/3" />
			</div>
		);
	} else if (chatQuery.isError) {
		content = (
			<div className="p-4">
				<ErrorAlert error={chatQuery.error} />
			</div>
		);
	} else if (chatData) {
		content = (
			<ChatSummary
				summary={chatData.summary}
				isSubagent={Boolean(chatData.parent_chat_id)}
				isGenerating={isGenerating}
				createdAt={chatData.created_at}
				updatedAt={chatData.updated_at}
				costMicros={costQuery.data?.total_cost_micros}
				unpricedRequestCount={costQuery.data?.unpriced_request_count}
				showCost={showCost}
				isCostLoading={costQuery.isLoading}
				costError={costQuery.isError}
			/>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col overflow-y-auto">
			{content}
		</div>
	);
};
