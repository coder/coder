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
	// Deduplicate by ID: the Map-based messagesByID already does,
	// and orderedMessageIDs must match it.
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

const chatQueuedMessagesEqualByID = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean =>
	left.length === right.length &&
	left.every((message, index) => message.id === right[index].id);

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

/** A request's expected result, shown until the stream syncs past it. */
export type PendingAction =
	| { kind: "edit"; editedMessageID: number; message?: TypesGen.ChatMessage }
	| {
			kind: "send";
			messages: readonly TypesGen.ChatMessage[];
			queuedMessage?: TypesGen.ChatQueuedMessage;
	  }
	| { kind: "deleteQueued"; queuedMessageID: number }
	| { kind: "promoteQueued"; queuedMessageID: number }
	| { kind: "compact" };

type PendingEntry = PendingAction & { id: number; untilVersion?: number };

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
	// Whether older messages exist below the oldest held one.
	hasMoreMessages: boolean;
	seeded: boolean;
	// Display only; the selectors apply them in request order.
	pendingActions: readonly PendingEntry[];
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	// Fills a store that has not been seeded from the newest messages page.
	seed: (page: {
		messages: readonly TypesGen.ChatMessage[];
		queuedMessages: readonly TypesGen.ChatQueuedMessage[];
		hasMore: boolean;
		historyVersion: number;
	}) => void;
	replaceMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
	// Applies a history_reset: keeps messages below fromID, or none.
	replaceMessagesFrom: (
		fromID: number | undefined,
		messages: readonly TypesGen.ChatMessage[],
	) => void;
	getHistoryResetCount: () => number;
	// Drops the page if a history reset was applied since resetCount.
	prependOlderMessages: (
		messages: readonly TypesGen.ChatMessage[],
		hasMore: boolean,
		resetCount: number,
	) => void;
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
	applyCheckpoint: (snapshotVersion: number, historyVersion?: number) => void;
	getSyncedHistoryVersion: () => number;
	// Adds nothing when the stream already synced past untilVersion.
	addPendingAction: (action: PendingAction, untilVersion?: number) => number;
	updatePendingAction: (
		id: number,
		patch: { untilVersion: number; message?: TypesGen.ChatMessage },
	) => void;
	removePendingAction: (id: number) => void;
	hasServerStatus: () => boolean;
	applyServerChatStatus: (status: TypesGen.ChatStatus) => void;
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
	subagentStatusOverrides: new Map(),
	hasMoreMessages: false,
	seeded: false,
	pendingActions: [],
});

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
	// Bookkeeping, deliberately outside the rendered state.
	let historyResets = 0;
	let syncedSnapshotVersion = 0;
	let syncedHistoryVersion = 0;
	let serverStatusReceived = false;
	let nextPendingActionID = 1;
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
				if (
					chatQueuedMessagesEqualByID(current.queuedMessages, queuedMessages)
				) {
					return current;
				}
				return { ...current, queuedMessages };
			});
		},
		seed: (page) => {
			if (state.seeded) {
				return;
			}
			syncedHistoryVersion = page.historyVersion;
			setState((current) => ({
				...current,
				messagesByID: buildMessageMap(page.messages),
				orderedMessageIDs: buildOrderedMessageIDs(page.messages),
				queuedMessages: page.queuedMessages,
				hasMoreMessages: page.hasMore,
				seeded: true,
			}));
		},
		replaceMessagesFrom: (fromID, messages) => {
			historyResets++;
			const kept =
				fromID === undefined
					? []
					: [...state.messagesByID.values()].filter(
							(message) => message.id < fromID,
						);
			replaceMessages([...kept, ...messages]);
		},
		getHistoryResetCount: () => historyResets,
		prependOlderMessages: (messages, hasMore, resetCount) => {
			if (resetCount !== historyResets) {
				return;
			}
			const older = messages.filter(
				(message) => !state.messagesByID.has(message.id),
			);
			upsertDurableMessages(older);
			setState((current) =>
				current.hasMoreMessages === hasMore
					? current
					: { ...current, hasMoreMessages: hasMore },
			);
		},
		applyCheckpoint: (snapshotVersion, historyVersion) => {
			syncedSnapshotVersion = Math.max(syncedSnapshotVersion, snapshotVersion);
			if (historyVersion !== undefined) {
				syncedHistoryVersion = historyVersion;
			}
			setState((current) => {
				const remaining = current.pendingActions.filter(
					(entry) =>
						entry.untilVersion === undefined ||
						entry.untilVersion > syncedSnapshotVersion,
				);
				return remaining.length === current.pendingActions.length
					? current
					: { ...current, pendingActions: remaining };
			});
		},
		getSyncedHistoryVersion: () => syncedHistoryVersion,
		addPendingAction: (action, untilVersion) => {
			const id = nextPendingActionID++;
			if (untilVersion !== undefined && untilVersion <= syncedSnapshotVersion) {
				return id;
			}
			setState((current) => ({
				...current,
				pendingActions: [
					...current.pendingActions,
					{ ...action, id, untilVersion },
				],
			}));
			return id;
		},
		updatePendingAction: (id, patch) => {
			setState((current) => {
				const done = patch.untilVersion <= syncedSnapshotVersion;
				const next = current.pendingActions.flatMap((entry) => {
					if (entry.id !== id) {
						return [entry];
					}
					if (done) {
						return [];
					}
					return [
						entry.kind === "edit" && patch.message
							? {
									...entry,
									message: patch.message,
									untilVersion: patch.untilVersion,
								}
							: { ...entry, untilVersion: patch.untilVersion },
					];
				});
				return { ...current, pendingActions: next };
			});
		},
		removePendingAction: (id) => {
			setState((current) =>
				current.pendingActions.some((entry) => entry.id === id)
					? {
							...current,
							pendingActions: current.pendingActions.filter(
								(entry) => entry.id !== id,
							),
						}
					: current,
			);
		},
		hasServerStatus: () => serverStatusReceived,
		setChatStatus: (status) => {
			if (state.chatStatus === status) {
				return;
			}
			setState((current) => ({
				...current,
				chatStatus: status,
			}));
		},
		applyServerChatStatus: (status) => {
			serverStatusReceived = true;
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

type ChatView = Pick<
	ChatStoreState,
	| "messagesByID"
	| "orderedMessageIDs"
	| "chatStatus"
	| "queuedMessages"
	| "streamState"
	| "streamError"
	| "retryState"
>;

// Cached per state object so selectors return the same values on every call,
// which useSyncExternalStore requires.
const views = new WeakMap<ChatStoreState, ChatView>();

// The stored state with each pending action's expected result applied.
const selectView = (state: ChatStoreState): ChatView => {
	if (state.pendingActions.length === 0) {
		return state;
	}
	const cached = views.get(state);
	if (cached) {
		return cached;
	}
	let messagesByID = state.messagesByID;
	let orderedMessageIDs = state.orderedMessageIDs;
	let queuedMessages = state.queuedMessages;
	let chatStatus = state.chatStatus;
	let turnStarts = false;
	const addMessages = (messages: readonly TypesGen.ChatMessage[]) => {
		const added = messages.filter((message) => !messagesByID.has(message.id));
		if (added.length === 0) {
			return;
		}
		messagesByID = new Map(messagesByID);
		for (const message of added) {
			messagesByID.set(message.id, message);
		}
		orderedMessageIDs = buildOrderedMessageIDs([...messagesByID.values()]);
	};
	for (const entry of state.pendingActions) {
		switch (entry.kind) {
			case "edit": {
				messagesByID = new Map(
					[...messagesByID].filter(([id]) => id < entry.editedMessageID),
				);
				orderedMessageIDs = orderedMessageIDs.filter(
					(id) => id < entry.editedMessageID,
				);
				if (entry.message) {
					addMessages([entry.message]);
				}
				queuedMessages = [];
				turnStarts = true;
				break;
			}
			case "send": {
				addMessages(entry.messages);
				const promotedIDs = new Set(
					entry.messages.map((message) => message.queued_message_id),
				);
				queuedMessages = queuedMessages.filter(
					(message) => !promotedIDs.has(message.id),
				);
				const queued = entry.queuedMessage;
				if (queued && !queuedMessages.some((m) => m.id === queued.id)) {
					queuedMessages = [...queuedMessages, queued];
				}
				if (entry.messages.some((message) => message.role === "user")) {
					turnStarts = true;
				}
				break;
			}
			case "deleteQueued":
				queuedMessages = queuedMessages.filter(
					(message) => message.id !== entry.queuedMessageID,
				);
				break;
			case "promoteQueued": {
				const target = queuedMessages.find(
					(message) => message.id === entry.queuedMessageID,
				);
				if (!target) {
					break;
				}
				const rest = queuedMessages.filter((message) => message !== target);
				// A busy chat moves the row to the head and stops the turn; an idle
				// one sends it at once (chatstate PromoteQueuedMessage).
				if (
					state.chatStatus === "running" ||
					state.chatStatus === "interrupting"
				) {
					queuedMessages = [target, ...rest];
					chatStatus = "interrupting";
					break;
				}
				queuedMessages = rest;
				const promoted = [...messagesByID.values()].some(
					(message) => message.queued_message_id === target.id,
				);
				if (!promoted) {
					addMessages([
						{
							// Sorts after every stored message until the real one arrives.
							id: Number.MAX_SAFE_INTEGER - entry.id,
							chat_id: target.chat_id,
							created_at: target.created_at,
							role: "user",
							content: target.content,
							model_config_id: target.model_config_id,
							queued_message_id: target.id,
						},
					]);
				}
				turnStarts = true;
				break;
			}
			case "compact":
				turnStarts = true;
				break;
		}
	}
	const view: ChatView = {
		messagesByID,
		orderedMessageIDs,
		queuedMessages,
		chatStatus: turnStarts ? "running" : chatStatus,
		streamState: turnStarts ? null : state.streamState,
		streamError: turnStarts ? null : state.streamError,
		retryState: turnStarts ? null : state.retryState,
	};
	views.set(state, view);
	return view;
};

export const selectMessagesByID = (state: ChatStoreState) =>
	selectView(state).messagesByID;
export const selectOrderedMessageIDs = (state: ChatStoreState) =>
	selectView(state).orderedMessageIDs;
export const selectStreamState = (state: ChatStoreState) =>
	selectView(state).streamState;
export const selectHasStreamState = (state: ChatStoreState) =>
	selectView(state).streamState !== null;
export const selectChatStatus = (state: ChatStoreState) =>
	selectView(state).chatStatus;
export const selectStreamError = (state: ChatStoreState) =>
	selectView(state).streamError;
export const selectQueuedMessages = (state: ChatStoreState) =>
	selectView(state).queuedMessages;
export const selectSubagentStatusOverrides = (state: ChatStoreState) =>
	state.subagentStatusOverrides;
export const selectRetryState = (state: ChatStoreState) =>
	selectView(state).retryState;
export const selectReconnectState = (state: ChatStoreState) =>
	state.reconnectState;
export const selectHasMoreMessages = (state: ChatStoreState) =>
	state.hasMoreMessages;
export const selectIsSeeded = (state: ChatStoreState) => state.seeded;

export const selectIsAwaitingFirstStreamChunk = (
	state: ChatStoreState,
): boolean => {
	const view = selectView(state);
	const latestMessageID =
		view.orderedMessageIDs[view.orderedMessageIDs.length - 1];
	const latestMessage =
		latestMessageID === undefined
			? undefined
			: view.messagesByID.get(latestMessageID);
	// Show the Thinking indicator while the chat runs, no stream output is
	// shown, and the latest shown message is not an assistant's.
	if (view.streamState !== null || latestMessage?.role === "assistant") {
		return false;
	}
	return view.chatStatus === "running";
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
