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
 * undefined until the first messages hydrate.
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
	return {
		messages,
		oldestLoadedMessageId,
		earlierAttachmentsQuery,
		earlierAttachments,
		attachments,
	};
};
