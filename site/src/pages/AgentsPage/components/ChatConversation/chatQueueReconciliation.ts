import type * as TypesGen from "#/api/typesGenerated";
import type { ChatStore, ChatStoreState } from "./chatStore";

/** @internal Exported for testing. */
export const restoreOptimisticRequestSnapshot = (
	store: Pick<
		ChatStore,
		| "batch"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
	>,
	snapshot: Pick<
		ChatStoreState,
		"chatStatus" | "queuedMessages" | "streamError" | "streamState"
	>,
): void => {
	store.batch(() => {
		store.setQueuedMessages(snapshot.queuedMessages);
		store.setChatStatus(snapshot.chatStatus);
		store.setStreamState(snapshot.streamState);
		store.setStreamError(snapshot.streamError);
	});
};

/**
 * Runs the optimistic queued-message promotion flow.
 *
 * The promote endpoint returns 202 Accepted with no message body, so the
 * actual user message is delivered via SSE or the messages REST endpoint.
 * Suppress the promoted ID so the transient reordered queue published by
 * the running-case backend does not flash the message back into the
 * visible queue. Roll back queue, status, and suppression on API error.
 *
 * @internal Exported for testing.
 */
export const runPromoteQueuedMessage = async (params: {
	id: number;
	store: Pick<
		ChatStore,
		| "batch"
		| "clearStreamError"
		| "clearStreamState"
		| "getSnapshot"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
		| "suppressQueuedMessageID"
		| "unsuppressQueuedMessageID"
	>;
	promoteQueuedMessage: (id: number) => Promise<void>;
	agentId: string | undefined;
	clearChatErrorReason: (chatID: string) => void;
	onError: (error: unknown) => void;
}): Promise<void> => {
	const {
		id,
		store,
		promoteQueuedMessage,
		agentId,
		clearChatErrorReason,
		onError,
	} = params;
	const previousSnapshot = store.getSnapshot();
	store.batch(() => {
		store.suppressQueuedMessageID(id);
		store.setQueuedMessages(
			previousSnapshot.queuedMessages.filter((message) => message.id !== id),
		);
		store.clearStreamState();
		store.clearStreamError();
		store.setChatStatus("running");
	});
	if (agentId) {
		clearChatErrorReason(agentId);
	}
	try {
		await promoteQueuedMessage(id);
	} catch (error) {
		store.unsuppressQueuedMessageID(id);
		restoreOptimisticRequestSnapshot(store, previousSnapshot);
		onError(error);
		throw error;
	}
};

const buildPromotedQueueReconciliation = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	insertedMessages: readonly TypesGen.ChatMessage[],
	promotedHeadID: number | undefined,
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
	hasObservedQueuedMessageID: (id: number) => boolean,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	if (promotedHeadID === undefined) {
		return undefined;
	}
	if (!insertedMessages.some((message) => message.role === "user")) {
		return undefined;
	}
	const remaining = queuedMessages.filter(
		(message) => message.id !== promotedHeadID,
	);
	const tailPending =
		queuedTail !== undefined &&
		!remaining.some((message) => message.id === queuedTail.id) &&
		!hasObservedQueuedMessageID(queuedTail.id);
	return tailPending ? [...remaining, queuedTail] : remaining;
};

// Prefer an inactive chat's cached queue so messages queued during the send
// are not dropped; fall back when no cache exists.
export const buildInactiveChatQueueReconciliation = (
	cachedQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	queuedMessagesBeforeSend: readonly TypesGen.ChatQueuedMessage[],
	insertedMessages: readonly TypesGen.ChatMessage[],
	promotedHeadID: number | undefined,
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined =>
	buildPromotedQueueReconciliation(
		cachedQueuedMessages ?? queuedMessagesBeforeSend,
		insertedMessages,
		promotedHeadID,
		queuedTail,
		() => false,
	);

// Queue updates may rotate the head before the response arrives.
export const reconcilePromotedQueueHead = (
	store: Pick<
		ChatStore,
		| "batch"
		| "getSnapshot"
		| "setQueuedMessages"
		| "markQueuedMessagePromoted"
		| "hasObservedQueuedMessageID"
	>,
	insertedMessages: readonly TypesGen.ChatMessage[],
	promotedHeadID: number | undefined,
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	const next = buildPromotedQueueReconciliation(
		store.getSnapshot().queuedMessages,
		insertedMessages,
		promotedHeadID,
		queuedTail,
		store.hasObservedQueuedMessageID,
	);
	if (!next || promotedHeadID === undefined) {
		return next;
	}
	store.batch(() => {
		// The promoted user row proves the server deleted its queue row.
		store.markQueuedMessagePromoted(promotedHeadID);
		store.setQueuedMessages(next);
	});
	return next;
};

// A promoted head is suppressed locally, but another tab can queue or
// promote concurrently, so only the server knows the resulting queue.
export const settlePromotedQueueHead = async (
	store: Pick<
		ChatStore,
		"getQueueConvergenceFence" | "applyPromoteRefetchQueuedMessages"
	>,
	chatID: string,
	promotedHeadID: number,
	fetchMessages: (chatID: string) => Promise<TypesGen.ChatMessagesResponse>,
): Promise<readonly TypesGen.ChatQueuedMessage[] | undefined> => {
	const baselineFence = store.getQueueConvergenceFence();
	let response: TypesGen.ChatMessagesResponse;
	try {
		response = await fetchMessages(chatID);
	} catch {
		// Convergence is best effort; a later authoritative update can correct it.
		return undefined;
	}
	return store.applyPromoteRefetchQueuedMessages(
		chatID,
		promotedHeadID,
		response.queued_messages ?? [],
		baselineFence,
	);
};

export async function submitEdit({
	editMessage,
	editArgs,
	onError,
}: {
	editMessage: (args: {
		messageId: number;
		optimisticMessage?: TypesGen.ChatMessage;
		req: TypesGen.EditChatMessageRequest;
	}) => Promise<unknown>;
	editArgs: {
		messageId: number;
		optimisticMessage?: TypesGen.ChatMessage;
		req: TypesGen.EditChatMessageRequest;
	};
	onError: (error: unknown) => void;
}): Promise<void> {
	try {
		await editMessage(editArgs);
	} catch (error) {
		onError(error);
		throw error;
	}
}

/** @internal Exported for testing. */
export const waitForPendingChatSettingsSyncs = async (
	pendingSyncs: readonly (Promise<unknown> | null | undefined)[],
): Promise<void> => {
	const activeSyncs = pendingSyncs.filter(
		(pendingSync): pendingSync is Promise<unknown> =>
			pendingSync !== null && pendingSync !== undefined,
	);
	if (activeSyncs.length === 0) {
		return;
	}
	await Promise.all(activeSyncs);
};
