import { useSyncExternalStore } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "../../utils/usageLimitMessage";
import { applyMessagePartToStreamState } from "./streamState";
import type { ReconnectState, RetryState, StreamState } from "./types";

const byMessageCreatedAt = (
	left: TypesGen.ChatMessage,
	right: TypesGen.ChatMessage,
): number => {
	return (
		new Date(left.created_at).getTime() - new Date(right.created_at).getTime()
	);
};

const buildMessageMap = (
	messages: readonly TypesGen.ChatMessage[],
): Map<number, TypesGen.ChatMessage> =>
	new Map(messages.map((message) => [message.id, message]));

const buildOrderedMessageIDs = (
	messages: readonly TypesGen.ChatMessage[],
): readonly number[] => {
	const sorted = [...messages];
	sorted.sort(byMessageCreatedAt);
	// Deduplicate by ID. The input can contain duplicate IDs when
	// cross-page duplication occurs in the React Query cache (e.g.
	// upsertCacheMessages writes to page 0 while the same message
	// still exists in a later page). The Map-based messagesByID
	// already deduplicates, but orderedMessageIDs must match.
	const seen = new Set<number>();
	return sorted
		.map((message) => message.id)
		.filter((id) => {
			if (seen.has(id)) return false;
			seen.add(id);
			return true;
		});
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

export const chatMessagesEqualByValue = (
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

export const chatQueuedMessagesEqualByID = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean => {
	if (left.length !== right.length) {
		return false;
	}
	for (let index = 0; index < left.length; index += 1) {
		if (left[index]?.id !== right[index]?.id) {
			return false;
		}
	}
	return true;
};

const retryStatesEqual = (
	left: RetryState | null,
	right: RetryState | null,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.attempt === right.attempt &&
		left.error === right.error &&
		left.kind === right.kind &&
		left.provider === right.provider &&
		left.delayMs === right.delayMs &&
		left.retryingAt === right.retryingAt
	);
};

const reconnectStatesEqual = (
	left: ReconnectState | null,
	right: ReconnectState | null,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.attempt === right.attempt &&
		left.delayMs === right.delayMs &&
		left.retryingAt === right.retryingAt
	);
};

export const isActiveChatStatus = (
	status: TypesGen.ChatStatus | null,
): boolean => status === "running" || status === "pending";

export type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	streamError: ChatDetailError | null;
	retryState: RetryState | null;
	reconnectState: ReconnectState | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	replaceMessages: (
		messages: readonly TypesGen.ChatMessage[] | undefined,
	) => void;
	upsertDurableMessage: (message: TypesGen.ChatMessage) => {
		isDuplicate: boolean;
		changed: boolean;
	};
	upsertDurableMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
	applyMessagePart: (part: TypesGen.ChatMessagePart) => void;
	applyMessageParts: (parts: readonly TypesGen.ChatMessagePart[]) => void;
	setQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	) => void;
	setChatStatus: (status: TypesGen.ChatStatus | null) => void;
	setStreamState: (streamState: StreamState | null) => void;
	setStreamError: (reason: ChatDetailError | null) => void;
	clearStreamError: () => void;
	setRetryState: (state: RetryState | null) => void;
	clearRetryState: () => void;
	setReconnectState: (state: ReconnectState | null) => void;
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
	subagentStatusOverrides: new Map(),
});

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
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

	const replaceMessages = (
		messages: readonly TypesGen.ChatMessage[] | undefined,
	): void => {
		const safeMessages = messages ?? [];
		const nextMessagesByID = buildMessageMap(safeMessages);
		const nextOrderedMessageIDs = buildOrderedMessageIDs(safeMessages);

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
		if (existing && chatMessagesEqualByValue(existing, message)) {
			return { isDuplicate, changed: false };
		}

		let actuallyChanged = false;
		setState((current) => {
			// Re-check inside the updater: another call may have
			// already applied this exact message.
			const curExisting = current.messagesByID.get(message.id);
			if (curExisting && chatMessagesEqualByValue(curExisting, message)) {
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
				if (existing && chatMessagesEqualByValue(existing, message)) {
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
			const nextQueuedMessages = queuedMessages ?? [];
			setState((current) => {
				if (
					chatQueuedMessagesEqualByID(
						current.queuedMessages,
						nextQueuedMessages,
					)
				) {
					return current;
				}
				return { ...current, queuedMessages: nextQueuedMessages };
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
		!latestMessage || latestMessage.role !== "assistant";
	// Show the "Thinking..." indicator when the store has no stream
	// data yet and the conversation is waiting for an assistant
	// response. For "running" status we use the existing broad
	// check (any non-assistant latest message). For "pending" we
	// restrict to the case where the latest message is explicitly
	// a user message — this covers the fresh-send flow (user just
	// submitted and the server hasn't started streaming yet) while
	// avoiding a spurious indicator during multi-turn tool-call
	// cycles, where the latest durable message is a tool result
	// and the assistant response is still being assembled.
	if (state.streamState !== null || !latestMessageNeedsAssistantResponse) {
		return false;
	}
	if (state.chatStatus === "running") {
		return true;
	}
	if (state.chatStatus === "pending" && latestMessage?.role === "user") {
		return true;
	}
	return false;
};

export const useChatSelector = <T>(
	store: ChatStore,
	selector: (state: ChatStoreState) => T,
): T => {
	const getSnapshot = () => selector(store.getSnapshot());
	return useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
};

export { useChatStore } from "./useChatStore";
