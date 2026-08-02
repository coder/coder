import { useSyncExternalStore } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "../../utils/usageLimitMessage";
import { applyMessagePartToStreamState } from "./streamState";
import type { ReconnectState, RetryState, StreamState } from "./types";

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

/**
 * Transient chat state. Durable messages are NOT here: the query cache is
 * canonical for them, written only by the per-chat socket. What remains is the
 * streaming overlay, its finalization handoff, transport state, and the queue.
 */
export type ChatStoreState = {
	streamState: StreamState | null;
	// Snapshot of the overlay taken when its assistant message committed, plus
	// that message's ID. It hands the tail over to the durable list without a
	// gap: the cache notifies a macrotask after the store does, so the overlay
	// has to keep rendering until the exact durable message is readable. It
	// NEVER accumulates parts; the next turn's first part starts a fresh
	// StreamState and drops it.
	finalizingStreamState: StreamState | null;
	finalizingMessageID: number | null;
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
	applyMessagePart: (part: TypesGen.ChatMessagePart) => void;
	applyMessageParts: (parts: readonly TypesGen.ChatMessagePart[]) => void;
	// Moves the current overlay into the finalizing snapshot and records the
	// durable assistant message that superseded it. Call it AFTER writing that
	// message to the cache, inside the same batch, so any reader woken by the
	// store notification already sees that message, and complete the handoff
	// when the message renders.
	beginStreamFinalization: (durableMessageID: number) => void;
	// Retires the handoff once the recorded durable message is readable from the
	// cache. Ignores a different ID, so a stale completion cannot drop the
	// overlay of a later turn.
	completeStreamFinalization: (durableMessageID: number) => void;
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
	unsuppressQueuedMessageID: (id: number) => void;
	clearSuppressedQueuedMessageIDs: () => void;
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
	streamState: null,
	finalizingStreamState: null,
	finalizingMessageID: null,
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

	const applyMessageParts = (parts: readonly TypesGen.ChatMessagePart[]) => {
		if (parts.length === 0) {
			return;
		}

		setState((current) => {
			if (current.finalizingMessageID !== null) {
				// These parts belong to the next turn, so they start a fresh
				// StreamState instead of appending to the finalized snapshot.
				let freshStreamState: StreamState | null = null;
				for (const part of parts) {
					freshStreamState = applyMessagePartToStreamState(
						freshStreamState,
						part,
					);
				}
				// Nothing renderable arrived yet (empty or metadata-only parts), so
				// the finalizing snapshot stays until the next turn has output.
				if (freshStreamState === null) {
					return current;
				}
				return {
					...current,
					streamState: freshStreamState,
					finalizingStreamState: null,
					finalizingMessageID: null,
				};
			}
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

	const beginStreamFinalization = (durableMessageID: number): void => {
		setState((current) => {
			if (current.streamState === null) {
				// No overlay to hand off. Any earlier snapshot is stale: its message
				// committed in an earlier batch and is already in the cache.
				if (
					current.finalizingStreamState === null &&
					current.finalizingMessageID === null
				) {
					return current;
				}
				return {
					...current,
					finalizingStreamState: null,
					finalizingMessageID: null,
				};
			}
			return {
				...current,
				streamState: null,
				finalizingStreamState: current.streamState,
				finalizingMessageID: durableMessageID,
			};
		});
	};

	// The handoff exists to bridge the window where the finalized message is not
	// yet readable from the cache, so it ends as soon as it is. In the common
	// case the cache write applies immediately and this runs in the same batch,
	// leaving no handoff behind; a write buffered behind a fetch keeps the
	// overlay until the replay lands.
	const completeStreamFinalization = (durableMessageID: number): void => {
		setState((current) => {
			if (current.finalizingMessageID !== durableMessageID) {
				return current;
			}
			return {
				...current,
				finalizingStreamState: null,
				finalizingMessageID: null,
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
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		beginStreamFinalization,
		completeStreamFinalization,
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
		setStreamState: (streamState) => {
			// An explicit overlay write is authoritative, so it also ends any
			// pending finalization handoff.
			if (
				state.streamState === streamState &&
				state.finalizingStreamState === null &&
				state.finalizingMessageID === null
			) {
				return;
			}
			setState((current) => {
				if (
					current.streamState === streamState &&
					current.finalizingStreamState === null &&
					current.finalizingMessageID === null
				) {
					return current;
				}
				return {
					...current,
					streamState,
					finalizingStreamState: null,
					finalizingMessageID: null,
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
			if (
				state.streamState === null &&
				state.finalizingStreamState === null &&
				state.finalizingMessageID === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
				finalizingStreamState: null,
				finalizingMessageID: null,
			}));
		},
		resetTransportReplayState: () => {
			if (
				state.reconnectState === null &&
				state.streamState === null &&
				state.finalizingStreamState === null &&
				state.finalizingMessageID === null &&
				state.streamError === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
				streamState: null,
				finalizingStreamState: null,
				finalizingMessageID: null,
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
				state.finalizingStreamState === null &&
				state.finalizingMessageID === null &&
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
				finalizingStreamState: null,
				finalizingMessageID: null,
				streamError: null,
				retryState: null,
				reconnectState: null,
				subagentStatusOverrides: new Map(),
			}));
		},
	};
};

export const selectStreamState = (state: ChatStoreState) => state.streamState;
export const selectHasStreamState = (state: ChatStoreState) =>
	state.streamState !== null;
/**
 * True while EITHER an active overlay or a finalizing snapshot is on screen.
 * Consumers that ask "is anything streaming visible" have to include the
 * handoff window, otherwise a Thinking indicator flashes underneath the
 * finalized tail.
 */
export const selectHasStreamOverlay = (state: ChatStoreState) =>
	state.streamState !== null || state.finalizingMessageID !== null;
export const selectFinalizingStreamState = (state: ChatStoreState) =>
	state.finalizingStreamState;
export const selectFinalizingMessageID = (state: ChatStoreState) =>
	state.finalizingMessageID;

/**
 * Resolves which overlay snapshot the tail renders. The active stream wins; the
 * finalizing snapshot bridges the handoff until the durable-reading parent
 * reports that the finalized message is readable from the cache.
 */
export const resolveOverlayStreamState = (
	streamState: StreamState | null,
	finalizingStreamState: StreamState | null,
	suppressFinalizedOverlay: boolean,
): StreamState | null =>
	streamState ?? (suppressFinalizedOverlay ? null : finalizingStreamState);
export const selectStreamError = (state: ChatStoreState) => state.streamError;
export const selectQueuedMessages = (state: ChatStoreState) =>
	state.queuedMessages;
export const selectSubagentStatusOverrides = (state: ChatStoreState) =>
	state.subagentStatusOverrides;
export const selectRetryState = (state: ChatStoreState) => state.retryState;
export const selectReconnectState = (state: ChatStoreState) =>
	state.reconnectState;

export const useChatSelector = <T>(
	store: ChatStore,
	selector: (state: ChatStoreState) => T,
): T => {
	const getSnapshot = () => selector(store.getSnapshot());
	return useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
};

export { useChatStore } from "./useChatStore";
