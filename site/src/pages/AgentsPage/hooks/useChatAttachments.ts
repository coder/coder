import { useState } from "react";
import { useQuery, useQueryClient } from "react-query";
import { chatAttachments } from "#/api/queries/chats";
import {
	type ChatStore,
	selectOldestStoredMessageID,
	useChatMessages,
	useChatSelector,
} from "../components/ChatConversation/chatStore";
import {
	getEarlierMessageAttachments,
	getMessageAttachments,
} from "../components/ChatConversation/messageHelpers";

/** The state of the server's list of earlier attachments. */
export type EarlierAttachmentsRequest = {
	isLoading: boolean;
	error: unknown;
	isRetrying: boolean;
	retry: () => void;
};

type UseChatAttachmentsOptions = {
	chatId: string;
	store: ChatStore;
	hasMoreMessages: boolean;
	/** Whether to fetch the attachments of messages that are not loaded. */
	enabled: boolean;
};

/**
 * A chat's attachments, newest first: the files in the loaded messages,
 * then the server's list for the messages before them.
 * `earlierAttachmentsRequest` is the state of the server's list, with a
 * retry for a failed request.
 */
export const useChatAttachments = ({
	chatId,
	store,
	hasMoreMessages,
	enabled,
}: UseChatAttachmentsOptions) => {
	const queryClient = useQueryClient();
	const messages = useChatMessages(store);
	// The oldest shown message can be a pending one, whose ID the server
	// does not know.
	const oldestLoadedMessageId = useChatSelector(
		store,
		selectOldestStoredMessageID,
	);
	const earlierAttachmentsQuery = useQuery({
		...chatAttachments(queryClient, chatId, oldestLoadedMessageId),
		enabled: enabled && hasMoreMessages,
	});
	const earlierAttachments =
		hasMoreMessages && earlierAttachmentsQuery.data
			? getEarlierMessageAttachments(earlierAttachmentsQuery.data.attachments)
			: [];
	const attachments = [
		...getMessageAttachments(messages),
		...earlierAttachments,
	];

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
	const earlierAttachmentsRequest: EarlierAttachmentsRequest = {
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
	return {
		messages,
		earlierAttachments,
		attachments,
		earlierAttachmentsRequest,
	};
};
