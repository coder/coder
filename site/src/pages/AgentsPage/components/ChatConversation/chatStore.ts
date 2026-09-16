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

export const chatQueuedMessagesEqualByID = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean =>
	left.length === right.length &&
	left.every((message, index) => message.id === right[index].id);

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
): boolean => status === "running" || status === "interrupting";

export type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	streamError: ChatDetailError | null;
	retryState: RetryState | null;
	reconnectState: ReconnectState | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	// Hides queued IDs from the visible queue while the backend is
	// in a transient state that would briefly include them. Used by
	// the running-case promote, where the backend reorders the
	// queued message to the front before auto-promoting it.
	suppressedQueuedMessageIDs: ReadonlySet<number>;
	// IDs confirmed deleted from the queue because the send response
	// contained their promoted user rows.
	promotedQueuedMessageIDs: ReadonlySet<number>;
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
	// Server-truthful queue snapshot, filtered through the
	// suppression set. Use for SSE queue_update and REST hydration;
	// optimistic writes go through setQueuedMessages so they don't
	// lift suppression.
	applyAuthoritativeQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	) => void;
	// Advances when an accepted snapshot or active-chat change invalidates an
	// in-flight convergence request. Discarded snapshots do not advance it.
	getQueueConvergenceFence: () => number;
	// Applies a promotion refetch only while the chat and fence still match.
	// Clears that ID's markers and returns the filtered queue for cache mirroring.
	applyPromoteRefetchQueuedMessages: (
		chatID: string,
		promotedID: number,
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
		baselineFence: number,
	) => readonly TypesGen.ChatQueuedMessage[] | undefined;
	suppressQueuedMessageID: (id: number) => void;
	markQueuedMessagePromoted: (id: number) => void;
	// Distinguishes a tail still in flight from one the server listed and later removed.
	hasObservedQueuedMessageID: (id: number) => boolean;
	setActiveChatID: (chatID: string | null) => void;
	getActiveChatID: () => string | null;
	// Counts server-reported status events, including repeats of the
	// current value, so a caller can tell that the server spoke during a
	// request even when the status did not change.
	getServerChatStatusVersion: () => number;
	applyServerChatStatus: (status: TypesGen.ChatStatus | null) => void;
	unsuppressQueuedMessageID: (id: number) => void;
	clearSuppressedQueuedMessageIDs: () => void;
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
	suppressedQueuedMessageIDs: new Set(),
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
				let nextSuppressed = current.suppressedQueuedMessageIDs;
				if (current.suppressedQueuedMessageIDs.size > 0) {
					const incomingIDs = new Set(incoming.map((message) => message.id));
					let copy: Set<number> | null = null;
					for (const id of current.suppressedQueuedMessageIDs) {
						if (!incomingIDs.has(id)) {
							if (!copy) {
								copy = new Set(current.suppressedQueuedMessageIDs);
							}
							copy.delete(id);
						}
					}
					if (copy) {
						nextSuppressed = copy;
					}
				}
				const filtered =
					nextSuppressed.size === 0
						? incoming
						: incoming.filter((message) => !nextSuppressed.has(message.id));
				const sameQueue = chatQueuedMessagesEqualByID(
					current.queuedMessages,
					filtered,
				);
				const sameSuppressed =
					nextSuppressed === current.suppressedQueuedMessageIDs;
				const nextPromoted =
					current.promotedQueuedMessageIDs.size === 0
						? current.promotedQueuedMessageIDs
						: new Set<number>();
				const samePromoted = nextPromoted === current.promotedQueuedMessageIDs;
				if (sameQueue && sameSuppressed && samePromoted) {
					return current;
				}
				return {
					...current,
					queuedMessages: sameQueue ? current.queuedMessages : filtered,
					suppressedQueuedMessageIDs: nextSuppressed,
					promotedQueuedMessageIDs: nextPromoted,
				};
			});
		},
		getQueueConvergenceFence: () => queueConvergenceFence,
		applyPromoteRefetchQueuedMessages: (
			chatID,
			promotedID,
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
			const incoming = queuedMessages ?? [];
			queueConvergenceFence++;
			for (const message of incoming) {
				observedQueuedMessageIDs.add(message.id);
			}
			const suppressed = new Set(state.suppressedQueuedMessageIDs);
			suppressed.delete(promotedID);
			const promoted = new Set(state.promotedQueuedMessageIDs);
			promoted.delete(promotedID);
			const applied =
				suppressed.size === 0
					? incoming
					: incoming.filter((message) => !suppressed.has(message.id));
			setState((current) => ({
				...current,
				queuedMessages: chatQueuedMessagesEqualByID(
					current.queuedMessages,
					applied,
				)
					? current.queuedMessages
					: applied,
				suppressedQueuedMessageIDs: suppressed,
				promotedQueuedMessageIDs: promoted,
			}));
			return applied;
		},
		hasObservedQueuedMessageID: (id) => observedQueuedMessageIDs.has(id),
		suppressQueuedMessageID: (id) => {
			setState((current) => {
				if (current.suppressedQueuedMessageIDs.has(id)) {
					return current;
				}
				const next = new Set(current.suppressedQueuedMessageIDs);
				next.add(id);
				return { ...current, suppressedQueuedMessageIDs: next };
			});
		},
		markQueuedMessagePromoted: (id) => {
			setState((current) => {
				if (
					current.suppressedQueuedMessageIDs.has(id) &&
					current.promotedQueuedMessageIDs.has(id)
				) {
					return current;
				}
				const suppressed = new Set(current.suppressedQueuedMessageIDs);
				suppressed.add(id);
				const promoted = new Set(current.promotedQueuedMessageIDs);
				promoted.add(id);
				return {
					...current,
					suppressedQueuedMessageIDs: suppressed,
					promotedQueuedMessageIDs: promoted,
				};
			});
		},
		unsuppressQueuedMessageID: (id) => {
			setState((current) => {
				if (
					!current.suppressedQueuedMessageIDs.has(id) &&
					!current.promotedQueuedMessageIDs.has(id)
				) {
					return current;
				}
				const suppressed = new Set(current.suppressedQueuedMessageIDs);
				suppressed.delete(id);
				const promoted = new Set(current.promotedQueuedMessageIDs);
				promoted.delete(id);
				return {
					...current,
					suppressedQueuedMessageIDs: suppressed,
					promotedQueuedMessageIDs: promoted,
				};
			});
		},
		clearSuppressedQueuedMessageIDs: () => {
			observedQueuedMessageIDs = new Set();
			setState((current) => {
				if (
					current.suppressedQueuedMessageIDs.size === 0 &&
					current.promotedQueuedMessageIDs.size === 0
				) {
					return current;
				}
				return {
					...current,
					suppressedQueuedMessageIDs: new Set(),
					promotedQueuedMessageIDs: new Set(),
				};
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

export { useChatStore } from "./useChatStore";
