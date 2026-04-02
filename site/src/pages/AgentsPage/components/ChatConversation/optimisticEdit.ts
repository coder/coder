import type * as TypesGen from "#/api/typesGenerated";
import type { ChatStoreState } from "./chatStore";
import type { StreamState } from "./types";

type VisibleConversation = {
	messages: readonly TypesGen.ChatMessage[];
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	chatStatus: TypesGen.ChatStatus | null;
	streamState: StreamState | null;
};

export type OptimisticEditSession = {
	token: symbol;
	editedMessageId: number;
	visibleMessageId: number;
	phase: "optimistic" | "authoritative";
};

const jsonValuesEqual = (left: unknown, right: unknown): boolean => {
	if (left === right) {
		return true;
	}
	try {
		return JSON.stringify(left) === JSON.stringify(right);
	} catch {
		return false;
	}
};

const chatMessagesEqualByValue = (
	left: TypesGen.ChatMessage,
	right: TypesGen.ChatMessage,
): boolean =>
	left.id === right.id &&
	left.chat_id === right.chat_id &&
	left.model_config_id === right.model_config_id &&
	left.created_at === right.created_at &&
	left.role === right.role &&
	jsonValuesEqual(left.content, right.content) &&
	jsonValuesEqual(left.usage, right.usage);

const getTrailingVisibleEditedMessage = ({
	visibleMessages,
	optimisticEditSession,
}: {
	visibleMessages: readonly TypesGen.ChatMessage[];
	optimisticEditSession: OptimisticEditSession;
}): TypesGen.ChatMessage | null => {
	const trailingVisibleMessage = visibleMessages[visibleMessages.length - 1];
	if (
		!trailingVisibleMessage ||
		trailingVisibleMessage.id !== optimisticEditSession.visibleMessageId ||
		trailingVisibleMessage.role !== "user"
	) {
		return null;
	}
	if (
		optimisticEditSession.phase === "optimistic" &&
		optimisticEditSession.visibleMessageId !==
			optimisticEditSession.editedMessageId
	) {
		return null;
	}
	return trailingVisibleMessage;
};

/**
 * Chat message IDs are monotonic within a chat, so removing every message with
 * an ID greater than or equal to the edited one matches the server-side tail
 * truncation for both oldest-first and newest-first arrays.
 */
export const truncateMessagesForEdit = (
	messages: readonly TypesGen.ChatMessage[],
	editedMessageId: number,
): readonly TypesGen.ChatMessage[] =>
	messages.filter((message) => message.id < editedMessageId);

export const buildOptimisticEditedMessage = (
	message: TypesGen.ChatMessage,
	optimisticContent: readonly TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	...message,
	content: optimisticContent,
});

export const projectAuthoritativeEditedConversation = (
	messagesBeforeEditedMessage: readonly TypesGen.ChatMessage[],
	responseMessage: TypesGen.ChatMessage,
): readonly TypesGen.ChatMessage[] => [
	...messagesBeforeEditedMessage,
	responseMessage,
];

export const getSavingMessageId = (
	optimisticEditSession: OptimisticEditSession | null | undefined,
): number | null =>
	optimisticEditSession?.phase === "optimistic"
		? optimisticEditSession.visibleMessageId
		: null;

export const hasConvergedOptimisticEditSession = ({
	fetchedMessages,
	optimisticEditSession,
}: {
	fetchedMessages: readonly TypesGen.ChatMessage[];
	optimisticEditSession: OptimisticEditSession | null | undefined;
}): boolean => {
	if (!optimisticEditSession) {
		return false;
	}
	return fetchedMessages.some(
		(message) => message.id === optimisticEditSession.visibleMessageId,
	);
};

/**
 * Detects the only cache/store mismatches we intentionally allow during a
 * message edit: the query cache has been truncated to the prefix before the
 * edited message, while the visible store keeps either the optimistic old-ID
 * bubble or the authoritative replacement-ID bubble in place.
 */
export const isExpectedOptimisticEditDivergence = ({
	visibleMessages,
	fetchedMessages,
	optimisticEditSession,
}: {
	visibleMessages: readonly TypesGen.ChatMessage[];
	fetchedMessages: readonly TypesGen.ChatMessage[];
	optimisticEditSession: OptimisticEditSession | null | undefined;
}): boolean => {
	if (!optimisticEditSession) {
		return false;
	}
	if (
		!getTrailingVisibleEditedMessage({
			visibleMessages,
			optimisticEditSession,
		})
	) {
		return false;
	}

	const expectedFetchedMessages = truncateMessagesForEdit(
		visibleMessages,
		optimisticEditSession.editedMessageId,
	);
	if (
		expectedFetchedMessages.length !== fetchedMessages.length ||
		expectedFetchedMessages.length !== visibleMessages.length - 1
	) {
		return false;
	}

	return expectedFetchedMessages.every((message, index) => {
		const fetchedMessage = fetchedMessages[index];
		return (
			fetchedMessage !== undefined &&
			chatMessagesEqualByValue(message, fetchedMessage)
		);
	});
};

export const getVisibleConversation = (
	snapshot: ChatStoreState,
): VisibleConversation => ({
	messages: snapshot.orderedMessageIDs
		.map((messageID) => snapshot.messagesByID.get(messageID))
		.filter((message): message is TypesGen.ChatMessage => Boolean(message)),
	queuedMessages: snapshot.queuedMessages,
	chatStatus: snapshot.chatStatus,
	streamState: snapshot.streamState,
});
