import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { ChatAttachments } from "./ChatAttachments";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import { type ChatStore, useChatMessages } from "./ChatConversation/chatStore";
import {
	deriveEvictedFileIds,
	getLoadedAgentAttachments,
} from "./ChatConversation/messageHelpers";
import type { EarlierMessagesProps } from "./ChatMessageScroller";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so the chat and cost queries don't run while the tab is hidden. */
	isVisible: boolean;
	store: ChatStore;
} & Pick<
	EarlierMessagesProps,
	| "hasMoreMessages"
	| "isFetchingMoreMessages"
	| "isHydratingMessages"
	| "hasFetchMoreError"
	| "onFetchMoreMessages"
>;

export const ChatSummaryPanel: React.FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
	store,
	isHydratingMessages,
	...earlierMessages
}) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });

	const chatData = chatQuery.data;
	const rootChatId = getChatCostTreeID(chatData) ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});

	const messages = useChatMessages(store);
	const evictedFileIds = deriveEvictedFileIds(
		messages.map((message) => ({ message })),
		chatData?.files,
	);

	let content: React.ReactNode = null;
	if (chatQuery.isError) {
		content = <ErrorAlert error={chatQuery.error} />;
	} else if (chatData) {
		content = (
			<ChatSummary
				summary={chatData.summary}
				isSubagent={Boolean(chatData.parent_chat_id)}
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
		<div className="flex h-full min-h-0 flex-col gap-6 overflow-y-auto p-4">
			{content}
			<ChatAttachments
				attachments={getLoadedAgentAttachments(messages, isHydratingMessages)}
				evictedFileIds={evictedFileIds}
				{...earlierMessages}
			/>
		</div>
	);
};
