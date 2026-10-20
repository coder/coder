import { getErrorStatus } from "#/api/errors";
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

// The server lists steer rows before queue rows, so a row joins the end of
// its own lane.
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

// As on the server, a steer row keeps its place.
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
 * Runs the optimistic "Send now" flow. A 404 means the row was already
 * delivered or deleted, so the row is removed and no error is reported.
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
	store.batch(() => {
		store.setQueuedMessages(
			moveToSteerLane(previousSnapshot.queuedMessages, id),
		);
		store.clearStreamState();
		store.clearStreamError();
		store.setChatStatus("running");
	});
	clearChatErrorReason(agentId);
	try {
		await promoteQueuedMessage(id);
	} catch (error) {
		const isGone = getErrorStatus(error) === 404;
		// A server queue or status received during the request is newer
		// than the snapshot and must win.
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
			if (store.getServerChatStatusVersion() === statusVersionBeforePromote) {
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

// Queue updates may change the queue before the response arrives, so this
// filters the current queue instead of the queue captured before the send.
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
		// The promoted user rows prove the server deleted their queue rows.
		for (const id of promotedIDs) {
			store.markQueuedMessagePromoted(id);
		}
		store.setQueuedMessages(next);
	});
	return next;
};

// Promoted rows are hidden locally, but another tab can queue or
// promote concurrently, so only the server knows the resulting queue.
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
