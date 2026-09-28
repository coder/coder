import type * as TypesGen from "#/api/typesGenerated";
import type { ChatStore, ChatStoreState } from "./chatStore";

/**
 * Restores the store to a snapshot taken before an optimistic request.
 * With baselineFence, the queue is restored only while the queue
 * convergence fence still has that value.
 */
export const restoreOptimisticRequestSnapshot = (
	store: Pick<
		ChatStore,
		| "batch"
		| "getQueueConvergenceFence"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
	>,
	snapshot: Pick<
		ChatStoreState,
		"chatStatus" | "queuedMessages" | "streamError" | "streamState"
	>,
	baselineFence?: number,
): void => {
	store.batch(() => {
		// A queue update received during the request is newer than the snapshot.
		if (
			baselineFence === undefined ||
			store.getQueueConvergenceFence() === baselineFence
		) {
			store.setQueuedMessages(snapshot.queuedMessages);
		}
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
 * visible queue. On API error, status, stream and suppression are rolled
 * back. The queue is restored from the snapshot when no queue update
 * arrived during the request. Otherwise it is refetched and written to the
 * store and the messages cache: that update was filtered through the
 * suppression, so both lack the promoted row the server still holds. If
 * the refetch fails, the snapshot is written instead.
 */
export const runPromoteQueuedMessage = async (params: {
	id: number;
	store: Pick<
		ChatStore,
		| "applyAuthoritativeQueuedMessages"
		| "batch"
		| "clearStreamError"
		| "clearStreamState"
		| "getActiveChatID"
		| "getQueueConvergenceFence"
		| "getSnapshot"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
		| "suppressQueuedMessageID"
		| "unsuppressQueuedMessageID"
	>;
	promoteQueuedMessage: (id: number) => Promise<void>;
	fetchQueueConvergence: (
		chatID: string,
	) => Promise<TypesGen.ChatMessagesResponse>;
	setCacheQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	) => void;
	agentId: string;
	clearChatErrorReason: (chatID: string) => void;
	onError: (error: unknown) => void;
}): Promise<void> => {
	const {
		id,
		store,
		promoteQueuedMessage,
		fetchQueueConvergence,
		setCacheQueuedMessages,
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
	const baselineFence = store.getQueueConvergenceFence();
	clearChatErrorReason(agentId);
	try {
		await promoteQueuedMessage(id);
	} catch (error) {
		store.unsuppressQueuedMessageID(id);
		restoreOptimisticRequestSnapshot(store, previousSnapshot, baselineFence);
		onError(error);
		if (
			store.getActiveChatID() === agentId &&
			store.getQueueConvergenceFence() !== baselineFence
		) {
			const refetchFence = store.getQueueConvergenceFence();
			let queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
			try {
				queuedMessages =
					(await fetchQueueConvergence(agentId)).queued_messages ?? [];
			} catch {
				queuedMessages = undefined;
			}
			// A queue update or chat switch during the refetch is newer.
			if (
				store.getActiveChatID() === agentId &&
				store.getQueueConvergenceFence() === refetchFence
			) {
				if (queuedMessages) {
					store.applyAuthoritativeQueuedMessages(queuedMessages);
				} else {
					store.setQueuedMessages(previousSnapshot.queuedMessages);
				}
				setCacheQueuedMessages(store.getSnapshot().queuedMessages);
			}
		}
		throw error;
	}
};

/**
 * Removes a queued message optimistically and deletes it. On failure the
 * queue is restored unless a queue update arrived during the request.
 */
export const runDeleteQueuedMessage = async (params: {
	id: number;
	store: Pick<
		ChatStore,
		"getQueueConvergenceFence" | "getSnapshot" | "setQueuedMessages"
	>;
	deleteQueuedMessage: (id: number) => Promise<unknown>;
}): Promise<void> => {
	const { id, store, deleteQueuedMessage } = params;
	const previousQueuedMessages = store.getSnapshot().queuedMessages;
	const baselineFence = store.getQueueConvergenceFence();
	store.setQueuedMessages(
		previousQueuedMessages.filter((message) => message.id !== id),
	);
	try {
		await deleteQueuedMessage(id);
	} catch (error) {
		if (store.getQueueConvergenceFence() === baselineFence) {
			store.setQueuedMessages(previousQueuedMessages);
		}
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
