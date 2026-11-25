import { getErrorMessage, getErrorStatus } from "#/api/errors";
import type { EditChatQueuedMessageVariables } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	type ChatStoreState,
	isActiveChatStatus,
} from "./chatStore";

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

// Match the server's steer-first order for the first row's Enter shortcut.
const appendToLane = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	message: TypesGen.ChatQueuedMessage,
): readonly TypesGen.ChatQueuedMessage[] => {
	const steer = queuedMessages.filter((m) => m.busy_behavior === "steer");
	const queue = queuedMessages.filter((m) => m.busy_behavior !== "steer");
	return message.busy_behavior === "steer"
		? [...steer, message, ...queue]
		: [...steer, ...queue, message];
};

const moveToSteerLane = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	id: number,
): readonly TypesGen.ChatQueuedMessage[] => {
	const target = queuedMessages.find((message) => message.id === id);
	if (target === undefined || target.busy_behavior === "steer") {
		return queuedMessages;
	}
	return appendToLane(
		queuedMessages.filter((message) => message.id !== id),
		{ ...target, busy_behavior: "steer" },
	);
};

/**
 * Tolerate a missing row because delivery or deletion can race "Send now".
 * An active chat keeps its status and live stream: the steer lane waits for
 * the next delivery point.
 */
export const runPromoteQueuedMessage = async (params: {
	id: number;
	store: Pick<
		ChatStore,
		| "batch"
		| "clearStreamError"
		| "clearStreamState"
		| "getQueueConvergenceFence"
		| "getServerChatStatusVersion"
		| "getSnapshot"
		| "setChatStatus"
		| "setQueuedMessages"
		| "setStreamError"
		| "setStreamState"
	>;
	promoteQueuedMessage: (id: number) => Promise<void>;
	agentId: string;
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
	const queueFenceBeforePromote = store.getQueueConvergenceFence();
	const statusVersionBeforePromote = store.getServerChatStatusVersion();
	const startsTurn = !isActiveChatStatus(previousSnapshot.chatStatus);
	store.batch(() => {
		store.setQueuedMessages(
			moveToSteerLane(previousSnapshot.queuedMessages, id),
		);
		if (startsTurn) {
			store.clearStreamState();
			store.clearStreamError();
			store.setChatStatus("running");
		}
	});
	clearChatErrorReason(agentId);
	try {
		await promoteQueuedMessage(id);
	} catch (error) {
		const isGone = getErrorStatus(error) === 404;
		// Preserve server queue and status updates accepted during the request.
		const queuedMessages =
			store.getQueueConvergenceFence() === queueFenceBeforePromote
				? previousSnapshot.queuedMessages
				: store.getSnapshot().queuedMessages;
		store.batch(() => {
			store.setQueuedMessages(
				isGone
					? queuedMessages.filter((message) => message.id !== id)
					: queuedMessages,
			);
			if (
				startsTurn &&
				store.getServerChatStatusVersion() === statusVersionBeforePromote
			) {
				store.setChatStatus(previousSnapshot.chatStatus);
				store.setStreamState(previousSnapshot.streamState);
				store.setStreamError(previousSnapshot.streamError);
			}
		});
		if (isGone) {
			return;
		}
		onError(error);
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

/**
 * Sends a queued-message edit request and applies its outcome to the store.
 * A 404 means the row was sent or removed; it is dropped locally because no
 * queue_update is guaranteed to follow. A successful request that sets
 * `editing` is written into the store so the composer does not wait for the
 * queue_update. Every failure is reported through onError and rethrown.
 */
export const runEditQueuedMessage = async (params: {
	id: number;
	req: TypesGen.EditChatQueuedMessageRequest;
	store: Pick<ChatStore, "getSnapshot" | "setQueuedMessages">;
	editQueuedMessage: (
		variables: EditChatQueuedMessageVariables,
	) => Promise<unknown>;
	failureMessage: string;
	onError: (message: string) => void;
}): Promise<void> => {
	const { id, req, store, editQueuedMessage, failureMessage, onError } = params;
	try {
		await editQueuedMessage({ queuedMessageId: id, req });
	} catch (error) {
		if (getErrorStatus(error) === 404) {
			store.setQueuedMessages(
				store.getSnapshot().queuedMessages.filter((row) => row.id !== id),
			);
			onError("Queued message was already sent or removed.");
		} else {
			onError(getErrorMessage(error, failureMessage));
		}
		throw error;
	}
	if (req.editing !== undefined) {
		// The server allows one row under edit per chat, so a begin ends any
		// other row's edit.
		const editingSince = req.editing ? new Date().toISOString() : undefined;
		store.setQueuedMessages(
			store.getSnapshot().queuedMessages.map((row) => {
				if (row.id === id) {
					return { ...row, editing_since: editingSince };
				}
				return req.editing && row.editing_since
					? { ...row, editing_since: undefined }
					: row;
			}),
		);
	}
};

const buildPromotedQueueReconciliation = (
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	promotedIDs: readonly number[],
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
	hasObservedQueuedMessageID: (id: number) => boolean,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	if (promotedIDs.length === 0) {
		return undefined;
	}
	const remaining = queuedMessages.filter(
		(message) => !promotedIDs.includes(message.id),
	);
	const tailPending =
		queuedTail !== undefined &&
		!promotedIDs.includes(queuedTail.id) &&
		!remaining.some((message) => message.id === queuedTail.id) &&
		!hasObservedQueuedMessageID(queuedTail.id);
	return tailPending ? appendToLane(remaining, queuedTail) : remaining;
};

// Prefer an inactive chat's cached queue so messages queued during the send
// are not dropped; fall back when no cache exists.
export const buildInactiveChatQueueReconciliation = (
	cachedQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	queuedMessagesBeforeSend: readonly TypesGen.ChatQueuedMessage[],
	promotedIDs: readonly number[],
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined =>
	buildPromotedQueueReconciliation(
		cachedQueuedMessages ?? queuedMessagesBeforeSend,
		promotedIDs,
		queuedTail,
		() => false,
	);

// Preserve queue updates received while the send was in flight.
export const reconcilePromotedQueuedMessages = (
	store: Pick<
		ChatStore,
		| "batch"
		| "getSnapshot"
		| "setQueuedMessages"
		| "markQueuedMessagePromoted"
		| "hasObservedQueuedMessageID"
	>,
	promotedIDs: readonly number[],
	queuedTail: TypesGen.ChatQueuedMessage | undefined,
): readonly TypesGen.ChatQueuedMessage[] | undefined => {
	const next = buildPromotedQueueReconciliation(
		store.getSnapshot().queuedMessages,
		promotedIDs,
		queuedTail,
		store.hasObservedQueuedMessageID,
	);
	if (!next) {
		return undefined;
	}
	store.batch(() => {
		for (const id of promotedIDs) {
			store.markQueuedMessagePromoted(id);
		}
		store.setQueuedMessages(next);
	});
	return next;
};

// Refetch after promotion to include concurrent queue changes from other clients.
export const settlePromotedQueuedMessages = async (
	store: Pick<
		ChatStore,
		"getQueueConvergenceFence" | "applyPromoteRefetchQueuedMessages"
	>,
	chatID: string,
	promotedIDs: readonly number[],
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
		promotedIDs,
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
