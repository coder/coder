import { useState } from "react";
import { useQuery, useQueryClient } from "react-query";
import { chat, chatAttachments, chatCost } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { ChatAttachments } from "./ChatAttachments";
import { getChatCostTreeID } from "./ChatConversation/chatHelpers";
import { type ChatStore, useChatMessages } from "./ChatConversation/chatStore";
import {
	deriveEvictedFileIds,
	getEarlierMessageAttachments,
	getMessageAttachments,
	partFileIds,
} from "./ChatConversation/messageHelpers";
import type { EarlierMessagesProps } from "./ChatMessageScroller";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryPanelProps = {
	chatId: string;
	/** Gate reads on tab visibility so the chat, cost, and attachments queries don't run while the tab is hidden. */
	isVisible: boolean;
	store: ChatStore;
} & Pick<EarlierMessagesProps, "hasMoreMessages" | "isHydratingMessages">;

export const ChatSummaryPanel: React.FC<ChatSummaryPanelProps> = ({
	chatId,
	isVisible,
	store,
	hasMoreMessages,
	isHydratingMessages,
}) => {
	const queryClient = useQueryClient();
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const chatQuery = useQuery({ ...chat(chatId), enabled: isVisible });

	const chatData = chatQuery.data;
	const rootChatId = getChatCostTreeID(chatData) ?? chatId;
	const costQuery = useQuery({
		...chatCost(rootChatId),
		enabled: isVisible && showCost && chatData !== undefined,
	});

	const messages = useChatMessages(store);
	const oldestLoadedMessageId = messages.at(0)?.id;
	const earlierAttachmentsQuery = useQuery({
		...chatAttachments(queryClient, chatId, oldestLoadedMessageId),
		enabled: isVisible && hasMoreMessages,
	});
	// The final transcript page clears hasMoreMessages before the store holds
	// its messages, so the cached list covers them until hydration catches up.
	const earlierAttachments =
		(hasMoreMessages || isHydratingMessages) && earlierAttachmentsQuery.data
			? getEarlierMessageAttachments(earlierAttachmentsQuery.data.attachments)
			: [];
	// A retry clears the error of a query with no data, so keep the retried
	// error until the retry settles, unless older messages have loaded since
	// and moved the query to a new `beforeId`.
	const [retriedFailure, setRetriedFailure] = useState<{
		beforeId: number | undefined;
		error: unknown;
	}>();
	const retriedError =
		retriedFailure && retriedFailure.beforeId === oldestLoadedMessageId
			? retriedFailure.error
			: null;
	const earlierAttachmentsError =
		hasMoreMessages && !earlierAttachmentsQuery.data
			? (earlierAttachmentsQuery.error ?? retriedError)
			: null;
	const isLoadingEarlierAttachments =
		hasMoreMessages &&
		!earlierAttachmentsQuery.data &&
		!earlierAttachmentsError;

	// Gating on isHydratingMessages alone would blank the list on every live
	// message.
	const attachments =
		isHydratingMessages && messages.length === 0
			? undefined
			: [...getMessageAttachments(messages), ...earlierAttachments];

	const evictedFileIds = deriveEvictedFileIds(
		messages.map((message) => ({ message })),
		chatData?.files,
		earlierAttachments.flatMap(({ file }) => partFileIds(file)),
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
				attachments={attachments}
				evictedFileIds={evictedFileIds}
				isLoadingEarlierAttachments={isLoadingEarlierAttachments}
				earlierAttachmentsError={earlierAttachmentsError}
				isRetrying={earlierAttachmentsQuery.isFetching}
				onRetry={() => {
					setRetriedFailure({
						beforeId: oldestLoadedMessageId,
						error: earlierAttachmentsError,
					});
					void earlierAttachmentsQuery
						.refetch()
						.finally(() => setRetriedFailure(undefined));
				}}
			/>
		</div>
	);
};
