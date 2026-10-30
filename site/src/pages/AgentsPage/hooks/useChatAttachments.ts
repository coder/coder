import { useState } from "react";
import { useQuery, useQueryClient } from "react-query";
import { chatAttachments } from "#/api/queries/chats";
import {
	type ChatStore,
	useChatMessages,
} from "../components/ChatConversation/chatStore";
import {
	getEarlierMessageAttachments,
	getMessageAttachments,
} from "../components/ChatConversation/messageHelpers";

type UseChatAttachmentsOptions = {
	chatId: string;
	store: ChatStore;
	hasMoreMessages: boolean;
	isHydratingMessages: boolean;
	/** Whether to fetch the attachments of messages that are not loaded. */
	enabled: boolean;
};

/**
 * A chat's attachments, newest first: the files in the loaded messages,
 * then the server's list for the messages before them. `attachments` is
 * undefined until the first messages hydrate. `earlier` is the state of the
 * server's list, with a retry for a failed request.
 */
export const useChatAttachments = ({
	chatId,
	store,
	hasMoreMessages,
	isHydratingMessages,
	enabled,
}: UseChatAttachmentsOptions) => {
	const queryClient = useQueryClient();
	const messages = useChatMessages(store);
	const oldestLoadedMessageId = messages.at(0)?.id;
	const earlierAttachmentsQuery = useQuery({
		...chatAttachments(queryClient, chatId, oldestLoadedMessageId),
		enabled: enabled && hasMoreMessages,
	});
	// The final transcript page clears hasMoreMessages before the store holds
	// its messages, so the cached list covers them until hydration catches up.
	const earlierAttachments =
		(hasMoreMessages || isHydratingMessages) && earlierAttachmentsQuery.data
			? getEarlierMessageAttachments(earlierAttachmentsQuery.data.attachments)
			: [];
	// Gating on isHydratingMessages alone would blank the list on every live
	// message.
	const attachments =
		isHydratingMessages && messages.length === 0
			? undefined
			: [...getMessageAttachments(messages), ...earlierAttachments];

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
	const error =
		hasMoreMessages && !earlierAttachmentsQuery.data
			? (earlierAttachmentsQuery.error ?? retriedError)
			: null;
	const earlier = {
		isLoading: hasMoreMessages && !earlierAttachmentsQuery.data && !error,
		error,
		isRetrying: earlierAttachmentsQuery.isFetching,
		retry: () => {
			setRetriedFailure({ beforeId: oldestLoadedMessageId, error });
			void earlierAttachmentsQuery
				.refetch()
				.finally(() => setRetriedFailure(undefined));
		},
	};
	return { messages, earlierAttachments, attachments, earlier };
};
