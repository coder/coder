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

// Compares two queue snapshots by value. Deliberately not by ID alone: a
// requeued edit keeps the ID and changes the content. Covers every field of
// `ChatQueuedMessage`, so it agrees with the structural sharing the query
// cache applies to a write.
export const chatQueuedMessagesEqualByValue = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean =>
	left.length === right.length &&
	left.every((message, index) => {
		const other = right[index];
		return (
			message.id === other.id &&
			message.chat_id === other.chat_id &&
			message.model_config_id === other.model_config_id &&
			message.created_at === other.created_at &&
			jsonValuesEqual(message.content, other.content)
		);
	});

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
 * Transient chat state. Durable messages and the durable queue are NOT here:
 * the query cache is canonical for both, and only server-derived values are
 * written to it. What remains is the streaming overlay, its finalization
 * handoff, transport state, and the read-time reconciliation markers that hide
 * queue rows the server has not caught up with yet.
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
	// Hides queued IDs from the visible queue while the cache still holds them.
	// Markers only SUBTRACT at read time, so an optimistic removal never writes
	// the cache and never needs a rollback. `chat_queued_messages.id` is a
	// global bigserial, so a marker for an id the server never lists again is
	// permanently inert.
	suppressedQueuedMessageIDs: ReadonlySet<number>;
	// IDs confirmed deleted from the queue because the send response contained
	// their promoted user rows. A snapshot still listing one predates that
	// deletion, so the gate rejects it.
	promotedQueuedMessageIDs: ReadonlySet<number>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
};

/**
 * Which arm of the queue gate delivered a snapshot. The socket arm gates the
 * cache write itself; the cache arm observes a write that already happened.
 */
export type QueueSnapshotSource = "socket" | "cache";

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
	// Accept/reject gate every authoritative queue snapshot passes through.
	// `source` names the arm that delivered it, because the two arms can do
	// different things about a snapshot:
	//   "socket": a `queue_update`. Its caller writes an accepted snapshot to
	//     the cache VERBATIM and drops a rejected one, so the gate really is a
	//     write gate here.
	//   "cache": a page-0 value already installed in the query cache. The
	//     observer runs AFTER the install, so rejecting cannot undo the write;
	//     it only withholds the fence advance and the marker reconciliation.
	// Suppression markers do the hiding at read time either way.
	acceptAuthoritativeQueueSnapshot: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
		source: QueueSnapshotSource,
	) => boolean;
	// Arms the echo the cache arm of the gate is expected to report back once,
	// for a write this client is making to the canonical queue. The WRITE PATH
	// is the only caller: an expectation is owed only by a write that actually
	// changes the cached value, and only the writer can know that. Arming for a
	// no-op write would leave an expectation nothing consumes, and it would
	// swallow the next genuine snapshot of that value.
	noteLocalQueueProjection: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	) => void;
	// Advances when an accepted snapshot or active-chat change invalidates an
	// in-flight convergence request. Rejected snapshots do not advance it.
	getQueueConvergenceFence: () => number;
	// Accepts a promotion refetch only while the chat and fence still match, and
	// clears that ID's markers. The caller writes the RAW response to the cache;
	// filtering stays at read time.
	acceptQueueConvergence: (
		chatID: string,
		promotedID: number,
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
		baselineFence: number,
	) => boolean;
	// Ends the promoted-head veto when its convergence fetch failed. The veto
	// rests on a GUESS about which row the server promoted, so leaving it
	// standing would reject every later snapshot that still lists that row and
	// freeze the queue. Clearing the marker alone is not enough: the snapshots
	// vetoed meanwhile were dropped and nothing re-delivers them, so this also
	// returns the newest one for the caller to write back as the last thing the
	// server said about the queue.
	abandonQueueConvergence: (
		chatID: string,
		promotedID: number,
		baselineFence: number,
	) => readonly TypesGen.ChatQueuedMessage[] | undefined;
	suppressQueuedMessageIDs: (ids: readonly number[]) => void;
	markQueuedMessagePromoted: (id: number) => void;
	// Distinguishes a tail still in flight from one the server listed and later removed.
	hasObservedQueuedMessageID: (id: number) => boolean;
	setActiveChatID: (chatID: string | null) => void;
	getActiveChatID: () => string | null;
	// Lifts ORDINARY suppression only. A promoted ID keeps both of its markers:
	// the queue operation that suppressed an ID does not own the promotion veto
	// a send placed on the same ID, and dropping that veto would let a stale
	// snapshot resurrect a row the server already promoted.
	unsuppressQueuedMessageIDs: (ids: readonly number[]) => void;
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
	// The value this client last wrote to the canonical cache and therefore
	// expects the cache arm of the gate to report back once. It is NOT a
	// general value-based dedupe: a later authoritative snapshot that happens
	// to carry the same value is a real server statement and must be gated.
	let expectedCacheEcho: readonly TypesGen.ChatQueuedMessage[] | null = null;
	// The newest snapshot the promoted-head veto rejected. A rejected snapshot
	// is dropped, and the server does not resend it, so this is the only
	// corrective truth left if the convergence fetch that justifies the veto
	// fails.
	let vetoedQueueSnapshot: readonly TypesGen.ChatQueuedMessage[] | null = null;
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
		acceptAuthoritativeQueueSnapshot: (queuedMessages, source) => {
			const incoming = queuedMessages ?? [];
			// The veto runs FIRST, ahead of any short-circuit. A snapshot
			// containing a confirmed promoted ID predates its queue deletion, so
			// installing it would also drop the newer tail the send returned, and
			// counting it would discard the convergence fetch racing it. Promoted
			// markers exist only from the send reconciliation until the server
			// catches up, so every other snapshot is accepted. Keep the newest
			// rejected one: it is what the convergence failure path restores.
			if (
				incoming.some((message) =>
					state.promotedQueuedMessageIDs.has(message.id),
				)
			) {
				vetoedQueueSnapshot = incoming;
				return false;
			}
			// The cache arm is watching the write this client just made. Consuming
			// the expectation once keeps that echo from advancing the fence a
			// second time and stranding a convergence request that carries
			// strictly newer information, while a genuine snapshot of the same
			// value still passes through the full gate below.
			if (
				source === "cache" &&
				expectedCacheEcho !== null &&
				chatQueuedMessagesEqualByValue(expectedCacheEcho, incoming)
			) {
				expectedCacheEcho = null;
				return true;
			}
			// Below the echo check, so a tail the CLIENT projected into the cache
			// is never recorded as a row the server listed. The send
			// reconciliation asks exactly that question about its own tail.
			for (const message of incoming) {
				observedQueuedMessageIDs.add(message.id);
			}
			vetoedQueueSnapshot = null;
			// Any expectation still standing describes an older write, and the
			// observer only ever reports the newest cached value. The write that
			// follows an accepted socket snapshot arms a fresh one if it changes
			// the cache.
			expectedCacheEcho = null;
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
				// The veto above already proved this snapshot lists no promoted ID,
				// so the server has caught up with every one of them.
				const nextPromoted =
					current.promotedQueuedMessageIDs.size === 0
						? current.promotedQueuedMessageIDs
						: new Set<number>();
				if (
					nextSuppressed === current.suppressedQueuedMessageIDs &&
					nextPromoted === current.promotedQueuedMessageIDs
				) {
					return current;
				}
				return {
					...current,
					suppressedQueuedMessageIDs: nextSuppressed,
					promotedQueuedMessageIDs: nextPromoted,
				};
			});
			return true;
		},
		noteLocalQueueProjection: (queuedMessages) => {
			expectedCacheEcho = queuedMessages;
		},
		getQueueConvergenceFence: () => queueConvergenceFence,
		acceptQueueConvergence: (
			chatID,
			promotedID,
			queuedMessages,
			baselineFence,
		) => {
			// The fence covers ordering, including navigation. This identity check
			// additionally keeps a response that names another chat out of the
			// shared store even if its caller captured the fence incorrectly.
			if (activeChatID !== chatID) {
				return false;
			}
			if (queueConvergenceFence !== baselineFence) {
				return false;
			}
			queueConvergenceFence++;
			for (const message of queuedMessages) {
				observedQueuedMessageIDs.add(message.id);
			}
			vetoedQueueSnapshot = null;
			// The caller writes this response to the cache, and that write arms
			// the echo if it changes anything. Arming here instead would arm one
			// for a response that merely confirms the cached queue, which is the
			// common case and produces no observation to consume it.
			expectedCacheEcho = null;
			setState((current) => {
				if (
					!current.suppressedQueuedMessageIDs.has(promotedID) &&
					!current.promotedQueuedMessageIDs.has(promotedID)
				) {
					return current;
				}
				const suppressed = new Set(current.suppressedQueuedMessageIDs);
				suppressed.delete(promotedID);
				const promoted = new Set(current.promotedQueuedMessageIDs);
				promoted.delete(promotedID);
				return {
					...current,
					suppressedQueuedMessageIDs: suppressed,
					promotedQueuedMessageIDs: promoted,
				};
			});
			return true;
		},
		abandonQueueConvergence: (chatID, promotedID, baselineFence) => {
			// Same guards as the accepting path. A moved fence or a changed chat
			// means the markers this call would clear are already gone.
			if (activeChatID !== chatID) {
				return undefined;
			}
			if (queueConvergenceFence !== baselineFence) {
				return undefined;
			}
			const corrective = vetoedQueueSnapshot;
			vetoedQueueSnapshot = null;
			queueConvergenceFence++;
			if (corrective) {
				for (const message of corrective) {
					observedQueuedMessageIDs.add(message.id);
				}
			}
			setState((current) => {
				const correctiveIDs = corrective
					? new Set(corrective.map((message) => message.id))
					: null;
				const staleSuppressed = [...current.suppressedQueuedMessageIDs].filter(
					(id) =>
						id === promotedID ||
						(correctiveIDs !== null && !correctiveIDs.has(id)),
				);
				if (
					staleSuppressed.length === 0 &&
					!current.promotedQueuedMessageIDs.has(promotedID)
				) {
					return current;
				}
				const suppressed = new Set(current.suppressedQueuedMessageIDs);
				for (const id of staleSuppressed) {
					suppressed.delete(id);
				}
				// Only this send's guess is retired. Another send's promotion still
				// has its own convergence request in flight.
				const promoted = new Set(current.promotedQueuedMessageIDs);
				promoted.delete(promotedID);
				return {
					...current,
					suppressedQueuedMessageIDs: suppressed,
					promotedQueuedMessageIDs: promoted,
				};
			});
			return corrective ?? undefined;
		},
		hasObservedQueuedMessageID: (id) => observedQueuedMessageIDs.has(id),
		suppressQueuedMessageIDs: (ids) => {
			setState((current) => {
				const added = ids.filter(
					(id) => !current.suppressedQueuedMessageIDs.has(id),
				);
				if (added.length === 0) {
					return current;
				}
				const next = new Set(current.suppressedQueuedMessageIDs);
				for (const id of added) {
					next.add(id);
				}
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
		unsuppressQueuedMessageIDs: (ids) => {
			setState((current) => {
				// A promoted ID is skipped: its markers belong to the send that
				// confirmed the promotion, and only that send's convergence may
				// retire them. A failed delete or edit lifting them here would let
				// a stale snapshot resurrect the row the server already promoted.
				const removed = ids.filter(
					(id) =>
						current.suppressedQueuedMessageIDs.has(id) &&
						!current.promotedQueuedMessageIDs.has(id),
				);
				if (removed.length === 0) {
					return current;
				}
				const suppressed = new Set(current.suppressedQueuedMessageIDs);
				for (const id of removed) {
					suppressed.delete(id);
				}
				return {
					...current,
					suppressedQueuedMessageIDs: suppressed,
				};
			});
		},
		clearSuppressedQueuedMessageIDs: () => {
			observedQueuedMessageIDs = new Set();
			expectedCacheEcho = null;
			vetoedQueueSnapshot = null;
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
export const selectSuppressedQueuedMessageIDs = (state: ChatStoreState) =>
	state.suppressedQueuedMessageIDs;
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
