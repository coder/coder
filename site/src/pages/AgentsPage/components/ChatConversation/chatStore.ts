import isEqual from "lodash/isEqual";
import { useSyncExternalStore } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { type ChatDetailError, chatDetailErrorsEqual } from "./chatError";
import { applyMessagePartToStreamState } from "./streamState";
import type { ReconnectState, RetryState, StreamState } from "./types";

const buildMessageMap = (
	messages: readonly TypesGen.ChatMessage[],
): Map<number, TypesGen.ChatMessage> =>
	new Map(messages.map((message) => [message.id, message]));

const buildOrderedMessageIDs = (
	messages: readonly TypesGen.ChatMessage[],
): readonly number[] => {
	// created_at is shared across an insert batch, so only id tracks append order.
	const sorted = messages.toSorted((left, right) => left.id - right.id);
	// Deduplicate by ID as a defense against duplicate IDs in the
	// input. Cache upserts fan the same fresh value out to every
	// page containing an ID, so cross-page duplicates in the React
	// Query cache are value-identical. The Map-based messagesByID
	// already deduplicates, but orderedMessageIDs must match.
	const seen = new Set<number>();
	const orderedMessageIDs: number[] = [];
	for (const message of sorted) {
		if (seen.has(message.id)) continue;
		seen.add(message.id);
		orderedMessageIDs.push(message.id);
	}
	return orderedMessageIDs;
};

const mapsEqualByRef = <K, V>(left: Map<K, V>, right: Map<K, V>): boolean => {
	if (left.size !== right.size) {
		return false;
	}
	for (const [key, value] of left) {
		if (!right.has(key) || right.get(key) !== value) {
			return false;
		}
	}
	return true;
};

const arraysEqual = <T>(left: readonly T[], right: readonly T[]): boolean => {
	if (left.length !== right.length) {
		return false;
	}
	for (let index = 0; index < left.length; index += 1) {
		if (left[index] !== right[index]) {
			return false;
		}
	}
	return true;
};

// Queued messages cannot be edited, so a row changes only its lane.
export const chatQueuedMessagesEqual = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean =>
	left.length === right.length &&
	left.every(
		(message, index) =>
			message.id === right[index].id &&
			message.busy_behavior === right[index].busy_behavior,
	);

const retryStatesEqual = (
	left: RetryState | null,
	right: RetryState,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left) {
		return false;
	}
	return (
		left.attempt === right.attempt &&
		left.error === right.error &&
		left.kind === right.kind &&
		left.provider === right.provider &&
		left.retryingAt === right.retryingAt
	);
};

const reconnectStatesEqual = (
	left: ReconnectState | null,
	right: ReconnectState,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left) {
		return false;
	}
	return (
		left.attempt === right.attempt &&
		left.delayMs === right.delayMs &&
		left.retryingAt === right.retryingAt
	);
};

/**
 * Whether the agent's turn is still in progress. `requires_action` counts:
 * the server is blocked on a client-executed tool call and accepts an
 * interrupt to cancel it.
 */
export const isActiveChatStatus = (
	status: TypesGen.ChatStatus | null,
): boolean =>
	status === "running" ||
	status === "requires_action" ||
	status === "interrupting";

export type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	streamError: ChatDetailError | null;
	retryState: RetryState | null;
	reconnectState: ReconnectState | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	// Send responses confirm these rows were delivered; retain their IDs to
	// reject queue snapshots taken before delivery.
	promotedQueuedMessageIDs: ReadonlySet<number>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	replaceMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
	upsertDurableMessage: (message: TypesGen.ChatMessage) => {
		isDuplicate: boolean;
		changed: boolean;
	};
	upsertDurableMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
	applyMessagePart: (part: TypesGen.ChatMessagePart) => void;
	applyMessageParts: (parts: readonly TypesGen.ChatMessagePart[]) => void;
	setQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	) => void;
	// Optimistic writes use setQueuedMessages to preserve promoted IDs for
	// rejecting stale server snapshots.
	applyAuthoritativeQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	) => void;
	// Advances when an accepted snapshot or active-chat change invalidates an
	// in-flight convergence request. Discarded snapshots do not advance it.
	getQueueConvergenceFence: () => number;
	// Return the filtered queue so cache hydration respects other in-flight sends.
	applyPromoteRefetchQueuedMessages: (
		chatID: string,
		promotedIDs: readonly number[],
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
		baselineFence: number,
	) => readonly TypesGen.ChatQueuedMessage[] | undefined;
	markQueuedMessagePromoted: (id: number) => void;
	// Distinguishes a tail still in flight from one the server listed and later removed.
	hasObservedQueuedMessageID: (id: number) => boolean;
	setActiveChatID: (chatID: string | null) => void;
	getActiveChatID: () => string | null;
	// Counts server-reported status events, including repeats of the
	// current value, so a caller can tell that the server spoke during a
	// request even when the status did not change.
	getServerChatStatusVersion: () => number;
	applyServerChatStatus: (status: TypesGen.ChatStatus) => void;
	resetQueuedMessageTracking: () => void;
	setChatStatus: (status: TypesGen.ChatStatus | null) => void;
	setStreamState: (streamState: StreamState | null) => void;
	setStreamError: (reason: ChatDetailError | null) => void;
	clearStreamError: () => void;
	setRetryState: (state: RetryState) => void;
	clearRetryState: () => void;
	setReconnectState: (state: ReconnectState) => void;
	clearReconnectState: () => void;
	clearStreamState: () => void;
	resetTransportReplayState: () => void;
	setSubagentStatusOverride: (
		chatID: string,
		status: TypesGen.ChatStatus,
	) => void;
	resetTransientState: () => void;
};

const createInitialState = (): ChatStoreState => ({
	messagesByID: new Map(),
	orderedMessageIDs: [],
	streamState: null,
	chatStatus: null,
	streamError: null,
	retryState: null,
	reconnectState: null,
	queuedMessages: [],
	promotedQueuedMessageIDs: new Set(),
	subagentStatusOverrides: new Map(),
});

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
	// Bookkeeping, deliberately outside the rendered state so observing a
	// server event cannot trigger a re-render.
	let observedQueuedMessageIDs = new Set<number>();
	let queueConvergenceFence = 0;
	let serverChatStatusVersion = 0;
	let activeChatID: string | null = null;
	const listeners = new Set<() => void>();

	const emit = (): void => {
		for (const listener of listeners) {
			listener();
		}
	};

	// Batching: suppress emit() during a batch and fire once
	// at the end. This collapses N store mutations from a
	// single WebSocket message into one subscriber notification.
	let batchDepth = 0;
	let batchDirty = false;

	const batch = (fn: () => void): void => {
		batchDepth += 1;
		try {
			fn();
		} finally {
			batchDepth -= 1;
			if (batchDepth === 0 && batchDirty) {
				batchDirty = false;
				emit();
			}
		}
	};

	const setState = (
		updater: (current: ChatStoreState) => ChatStoreState,
	): void => {
		const next = updater(state);
		if (next === state) {
			return;
		}
		state = next;
		if (batchDepth > 0) {
			batchDirty = true;
		} else {
			emit();
		}
	};

	const replaceMessages = (messages: readonly TypesGen.ChatMessage[]): void => {
		const nextMessagesByID = buildMessageMap(messages);
		const nextOrderedMessageIDs = buildOrderedMessageIDs(messages);

		// Fast-path: skip setState entirely when nothing changed.
		if (
			mapsEqualByRef(state.messagesByID, nextMessagesByID) &&
			arraysEqual(state.orderedMessageIDs, nextOrderedMessageIDs)
		) {
			return;
		}

		setState((current) => {
			// Re-check equality against `current` inside the updater
			// to avoid overwriting a concurrent state change.
			if (
				mapsEqualByRef(current.messagesByID, nextMessagesByID) &&
				arraysEqual(current.orderedMessageIDs, nextOrderedMessageIDs)
			) {
				return current;
			}
			return {
				...current,
				messagesByID: nextMessagesByID,
				orderedMessageIDs: nextOrderedMessageIDs,
			};
		});
	};

	const upsertDurableMessage = (message: TypesGen.ChatMessage) => {
		// Use `state` for the early-return guard so we can return
		// the result synchronously. The actual mutation below uses
		// `current` inside the updater to avoid overwriting a
		// concurrent state change (TOCTOU).
		const existing = state.messagesByID.get(message.id);
		const isDuplicate = state.messagesByID.has(message.id);
		if (existing && isEqual(existing, message)) {
			return { isDuplicate, changed: false };
		}

		let actuallyChanged = false;
		setState((current) => {
			// Re-check inside the updater: another call may have
			// already applied this exact message.
			const curExisting = current.messagesByID.get(message.id);
			if (curExisting && isEqual(curExisting, message)) {
				return current;
			}

			actuallyChanged = true;

			const nextMessagesByID = new Map(current.messagesByID);
			nextMessagesByID.set(message.id, message);

			const curIsDuplicate = current.messagesByID.has(message.id);
			const needsReorder =
				!curIsDuplicate || nextMessagesByID.size !== current.messagesByID.size;
			const nextOrderedMessageIDs = needsReorder
				? buildOrderedMessageIDs(Array.from(nextMessagesByID.values()))
				: current.orderedMessageIDs;

			return {
				...current,
				messagesByID: nextMessagesByID,
				orderedMessageIDs: nextOrderedMessageIDs,
			};
		});
		return { isDuplicate, changed: actuallyChanged };
	};

	// Bulk variant that applies all messages in a single pass —
	// one Map copy and one sort instead of N copies and N sorts.
	const upsertDurableMessages = (
		messages: readonly TypesGen.ChatMessage[],
	): void => {
		if (messages.length === 0) {
			return;
		}
		setState((current) => {
			let nextMessagesByID: Map<number, TypesGen.ChatMessage> | null = null;
			for (const message of messages) {
				const map = nextMessagesByID ?? current.messagesByID;
				const existing = map.get(message.id);
				if (existing && isEqual(existing, message)) {
					continue;
				}
				// Lazily copy the map on first actual change.
				if (!nextMessagesByID) {
					nextMessagesByID = new Map(current.messagesByID);
				}
				nextMessagesByID.set(message.id, message);
			}
			if (!nextMessagesByID) {
				return current;
			}
			const needsReorder = nextMessagesByID.size !== current.messagesByID.size;
			const nextOrderedMessageIDs = needsReorder
				? buildOrderedMessageIDs(Array.from(nextMessagesByID.values()))
				: current.orderedMessageIDs;
			return {
				...current,
				messagesByID: nextMessagesByID,
				orderedMessageIDs: nextOrderedMessageIDs,
			};
		});
	};

	const applyMessageParts = (parts: readonly TypesGen.ChatMessagePart[]) => {
		if (parts.length === 0) {
			return;
		}

		setState((current) => {
			let nextStreamState: StreamState | null = current.streamState;
			for (const part of parts) {
				nextStreamState = applyMessagePartToStreamState(nextStreamState, part);
			}
			if (nextStreamState === current.streamState) {
				return current;
			}
			return {
				...current,
				streamState: nextStreamState,
			};
		});
	};

	return {
		getSnapshot: () => state,
		subscribe: (listener) => {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		},
		batch,
		replaceMessages,
		upsertDurableMessage,
		upsertDurableMessages,
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		setQueuedMessages: (queuedMessages) => {
			setState((current) => {
				if (chatQueuedMessagesEqual(current.queuedMessages, queuedMessages)) {
					return current;
				}
				return { ...current, queuedMessages };
			});
		},
		applyAuthoritativeQueuedMessages: (queuedMessages) => {
			const incoming = queuedMessages ?? [];
			for (const message of incoming) {
				observedQueuedMessageIDs.add(message.id);
			}
			// A snapshot containing a confirmed promoted ID predates its queue
			// deletion. Applying it would also drop newer queued messages, and
			// counting it would discard the fresher refetch racing it.
			if (
				incoming.some((message) =>
					state.promotedQueuedMessageIDs.has(message.id),
				)
			) {
				return;
			}
			queueConvergenceFence++;
			setState((current) => {
				const sameQueue = chatQueuedMessagesEqual(
					current.queuedMessages,
					incoming,
				);
				if (sameQueue && current.promotedQueuedMessageIDs.size === 0) {
					return current;
				}
				return {
					...current,
					queuedMessages: sameQueue ? current.queuedMessages : incoming,
					promotedQueuedMessageIDs: new Set(),
				};
			});
		},
		getQueueConvergenceFence: () => queueConvergenceFence,
		applyPromoteRefetchQueuedMessages: (
			chatID,
			promotedIDs,
			queuedMessages,
			baselineFence,
		) => {
			// The fence covers ordering, including navigation. This identity check
			// additionally keeps a response that names another chat out of the
			// shared store even if its caller captured the fence incorrectly.
			if (activeChatID !== chatID) {
				return undefined;
			}
			if (queueConvergenceFence !== baselineFence) {
				return undefined;
			}
			queueConvergenceFence++;
			for (const message of queuedMessages) {
				observedQueuedMessageIDs.add(message.id);
			}
			const promoted = new Set(state.promotedQueuedMessageIDs);
			for (const id of promotedIDs) {
				promoted.delete(id);
			}
			const applied =
				promoted.size === 0
					? queuedMessages
					: queuedMessages.filter((message) => !promoted.has(message.id));
			setState((current) => ({
				...current,
				queuedMessages: chatQueuedMessagesEqual(current.queuedMessages, applied)
					? current.queuedMessages
					: applied,
				promotedQueuedMessageIDs: promoted,
			}));
			return applied;
		},
		hasObservedQueuedMessageID: (id) => observedQueuedMessageIDs.has(id),
		markQueuedMessagePromoted: (id) => {
			setState((current) => {
				if (current.promotedQueuedMessageIDs.has(id)) {
					return current;
				}
				const promoted = new Set(current.promotedQueuedMessageIDs);
				promoted.add(id);
				return { ...current, promotedQueuedMessageIDs: promoted };
			});
		},
		resetQueuedMessageTracking: () => {
			observedQueuedMessageIDs = new Set();
			setState((current) => {
				if (current.promotedQueuedMessageIDs.size === 0) {
					return current;
				}
				return { ...current, promotedQueuedMessageIDs: new Set() };
			});
		},
		setChatStatus: (status) => {
			if (state.chatStatus === status) {
				return;
			}
			setState((current) => ({
				...current,
				chatStatus: status,
			}));
		},
		setActiveChatID: (chatID) => {
			if (activeChatID === chatID) {
				return;
			}
			activeChatID = chatID;
			// Leaving a chat strands any convergence request issued for it, so a
			// later return to the same chat cannot revive one.
			queueConvergenceFence++;
		},
		getActiveChatID: () => activeChatID,
		getServerChatStatusVersion: () => serverChatStatusVersion,
		applyServerChatStatus: (status) => {
			serverChatStatusVersion++;
			if (state.chatStatus === status) {
				return;
			}
			setState((current) => ({
				...current,
				chatStatus: status,
			}));
		},
		setStreamState: (streamState) => {
			if (state.streamState === streamState) {
				return;
			}
			setState((current) => {
				if (current.streamState === streamState) {
					return current;
				}
				return {
					...current,
					streamState,
				};
			});
		},
		setStreamError: (reason) => {
			setState((current) => {
				if (chatDetailErrorsEqual(current.streamError, reason)) {
					return current;
				}
				return {
					...current,
					streamError: reason,
				};
			});
		},
		clearStreamError: () => {
			if (state.streamError === null) {
				return;
			}
			setState((current) => ({
				...current,
				streamError: null,
			}));
		},
		setRetryState: (retryState) => {
			setState((current) => {
				if (retryStatesEqual(current.retryState, retryState)) {
					return current;
				}
				return {
					...current,
					retryState,
				};
			});
		},
		clearRetryState: () => {
			if (state.retryState === null) {
				return;
			}
			setState((current) => ({
				...current,
				retryState: null,
			}));
		},
		setReconnectState: (reconnectState) => {
			setState((current) => {
				if (reconnectStatesEqual(current.reconnectState, reconnectState)) {
					return current;
				}
				return {
					...current,
					reconnectState,
				};
			});
		},
		clearReconnectState: () => {
			if (state.reconnectState === null) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
			}));
		},
		clearStreamState: () => {
			if (state.streamState === null) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
			}));
		},
		resetTransportReplayState: () => {
			if (
				state.reconnectState === null &&
				state.streamState === null &&
				state.streamError === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
				streamState: null,
				streamError: null,
			}));
		},
		setSubagentStatusOverride: (chatID, status) => {
			if (state.subagentStatusOverrides.get(chatID) === status) {
				return;
			}
			setState((current) => {
				if (current.subagentStatusOverrides.get(chatID) === status) {
					return current;
				}
				const nextOverrides = new Map(current.subagentStatusOverrides);
				nextOverrides.set(chatID, status);
				return { ...current, subagentStatusOverrides: nextOverrides };
			});
		},
		resetTransientState: () => {
			if (
				state.streamState === null &&
				state.streamError === null &&
				state.retryState === null &&
				state.reconnectState === null &&
				state.subagentStatusOverrides.size === 0
			) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
				streamError: null,
				retryState: null,
				reconnectState: null,
				subagentStatusOverrides: new Map(),
			}));
		},
	};
};

export const selectMessagesByID = (state: ChatStoreState) => state.messagesByID;
export const selectOrderedMessageIDs = (state: ChatStoreState) =>
	state.orderedMessageIDs;
export const selectStreamState = (state: ChatStoreState) => state.streamState;
export const selectHasStreamState = (state: ChatStoreState) =>
	state.streamState !== null;
export const selectChatStatus = (state: ChatStoreState) => state.chatStatus;
export const selectStreamError = (state: ChatStoreState) => state.streamError;
export const selectQueuedMessages = (state: ChatStoreState) =>
	state.queuedMessages;
export const selectSubagentStatusOverrides = (state: ChatStoreState) =>
	state.subagentStatusOverrides;
export const selectRetryState = (state: ChatStoreState) => state.retryState;
export const selectReconnectState = (state: ChatStoreState) =>
	state.reconnectState;

const selectLatestDurableMessage = (
	state: ChatStoreState,
): TypesGen.ChatMessage | undefined => {
	const latestMessageID =
		state.orderedMessageIDs[state.orderedMessageIDs.length - 1];
	return latestMessageID === undefined
		? undefined
		: state.messagesByID.get(latestMessageID);
};

export const selectIsAwaitingFirstStreamChunk = (
	state: ChatStoreState,
): boolean => {
	const latestMessage = selectLatestDurableMessage(state);
	const latestMessageNeedsAssistantResponse =
		latestMessage?.role !== "assistant";
	// Show the Thinking indicator when the store has no stream
	// data yet, the chat is running, and the conversation is
	// waiting for an assistant response (any non-assistant latest
	// message).
	if (state.streamState !== null || !latestMessageNeedsAssistantResponse) {
		return false;
	}
	return state.chatStatus === "running";
};

export const useChatSelector = <T>(
	store: ChatStore,
	selector: (state: ChatStoreState) => T,
): T => {
	const getSnapshot = () => selector(store.getSnapshot());
	return useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
};

/** The store's durable messages, oldest first. */
export const useChatMessages = (store: ChatStore): TypesGen.ChatMessage[] => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	return orderedMessageIDs.flatMap((messageID) => {
		const message = messagesByID.get(messageID);
		if (!message && process.env.NODE_ENV !== "production") {
			console.warn(
				`[useChatMessages] orderedMessageIDs contains ID ${messageID} ` +
					"not found in messagesByID. This may indicate a store/cache " +
					"desync bug.",
			);
		}
		return message ? [message] : [];
	});
};

export { useChatStore } from "./useChatStore";
