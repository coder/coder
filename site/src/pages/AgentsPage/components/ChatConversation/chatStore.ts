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

/**
 * A request's expected result, drawn over the server state from the click
 * until the stream reports it or the request fails.
 */
export type PendingAction =
	| { kind: "edit"; editedMessageID: number; message?: TypesGen.ChatMessage }
	| { kind: "send"; message: TypesGen.ChatMessage }
	| { kind: "deleteQueued"; queuedMessageID: number }
	| { kind: "promoteQueued"; queuedMessageID: number };

type QueueEdit = (
	rows: readonly TypesGen.ChatQueuedMessage[],
) => readonly TypesGen.ChatQueuedMessage[];

/**
 * A pending action's expected result, fixed at the click from the shown
 * state. Each part is drawn until the stream's next update of its kind
 * replaces it.
 */
type PendingEntry = {
	id: number;
	// The message the request adds. The server reports it as a user message
	// or, when it queues it, as a queue update.
	message?: { shown?: TypesGen.ChatMessage; queue?: QueueEdit };
	// Replaced by a queue update.
	queue?: QueueEdit;
	// The history from an edited message on, replaced by a history reset.
	history?: { fromID: number; draft?: TypesGen.ChatMessage };
};

type PendingPart = Exclude<keyof PendingEntry, "id">;

/** Progress of loading a store's newest messages page. */
export type ChatStoreLoad =
	| { status: "loading" }
	| { status: "loaded" }
	| { status: "failed"; error: unknown };

export type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	// A failure of this client, such as a rejected request.
	streamError: ChatDetailError | null;
	retryState: RetryState | null;
	// The error the chat's last reported turn ended with.
	turnError: ChatDetailError | null;
	reconnectState: ReconnectState | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	// Whether older messages exist below the oldest held one.
	hasMoreMessages: boolean;
	load: ChatStoreLoad;
	// Display only; the selectors apply them in request order.
	pendingActions: readonly PendingEntry[];
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	// Fills a store that has not loaded yet with the newest messages page.
	seed: (page: TypesGen.ChatMessagesResponse) => void;
	// Seeds the store from fetchPage unless it is loaded or loading.
	load: (fetchPage: () => Promise<TypesGen.ChatMessagesResponse>) => void;
	replaceMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
	// Applies a history_reset: drops the messages with IDs at or above fromID.
	resetHistoryFrom: (fromID: number) => void;
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
	// Applies a status event; the turn error lasts while it is error.
	applyServerChatStatus: (status: TypesGen.ChatStatus) => void;
	setTurnError: (reason: ChatDetailError) => void;
	// The highest snapshot version of the newest page and the applied events.
	getSyncedSnapshotVersion: () => number;
	advanceSnapshotVersion: (version: number) => void;
	addPendingAction: (action: PendingAction) => number;
	removePendingAction: (id: number) => void;
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
	turnError: null,
	reconnectState: null,
	queuedMessages: [],
	subagentStatusOverrides: new Map(),
	hasMoreMessages: false,
	load: { status: "loading" },
	pendingActions: [],
});

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
	// Bookkeeping, deliberately outside the rendered state so observing a
	// server event cannot trigger a re-render.
	let historyResets = 0;
	let syncedSnapshotVersion = 0;
	let nextPendingActionID = 1;
	let loadInFlight = false;
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

	const endPending = (parts: readonly PendingPart[]): void => {
		setState((current) => {
			const pendingActions = endPendingParts(current.pendingActions, parts);
			return arraysEqual(pendingActions, current.pendingActions)
				? current
				: { ...current, pendingActions };
		});
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

	const store: ChatStore = {
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
		upsertDurableMessages: (messages) => {
			batch(() => {
				upsertDurableMessages(messages);
				if (messages.some((message) => message.role === "user")) {
					endPending(["message"]);
				}
			});
		},
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		setQueuedMessages: (queuedMessages) => {
			batch(() => {
				setState((current) => {
					if (
						chatQueuedMessagesEqualByID(current.queuedMessages, queuedMessages)
					) {
						return current;
					}
					return { ...current, queuedMessages };
				});
				endPending(["message", "queue"]);
			});
		},
		seed: (page) => {
			if (state.load.status === "loaded") {
				return;
			}
			syncedSnapshotVersion = page.snapshot_version ?? 0;
			setState((current) => ({
				...current,
				messagesByID: buildMessageMap(page.messages),
				orderedMessageIDs: buildOrderedMessageIDs(page.messages),
				queuedMessages: page.queued_messages,
				hasMoreMessages: page.has_more,
				load: { status: "loaded" },
			}));
		},
		load: (fetchPage) => {
			if (loadInFlight || state.load.status === "loaded") {
				return;
			}
			loadInFlight = true;
			setState((current) =>
				current.load.status === "loading"
					? current
					: { ...current, load: { status: "loading" } },
			);
			fetchPage().then(
				(page) => {
					loadInFlight = false;
					// Only a server older than this client omits the version.
					if (page.snapshot_version === undefined) {
						const error = new Error(
							"The server does not support this version of the page. Try again, or reload the page.",
						);
						setState((current) => ({
							...current,
							load: { status: "failed", error },
						}));
						return;
					}
					store.seed(page);
				},
				(error: unknown) => {
					loadInFlight = false;
					setState((current) => ({
						...current,
						load: { status: "failed", error },
					}));
				},
			);
		},
		resetHistoryFrom: (fromID) => {
			historyResets++;
			batch(() => {
				replaceMessages(
					[...state.messagesByID.values()].filter(
						(message) => message.id < fromID,
					),
				);
				endPending(["history"]);
			});
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
		applyServerChatStatus: (status) => {
			setState((current) => {
				const turnError = status === "error" ? current.turnError : null;
				if (current.chatStatus === status && current.turnError === turnError) {
					return current;
				}
				return { ...current, chatStatus: status, turnError };
			});
		},
		setTurnError: (reason) => {
			setState((current) =>
				chatDetailErrorsEqual(current.turnError, reason)
					? current
					: { ...current, turnError: reason },
			);
		},
		getSyncedSnapshotVersion: () => syncedSnapshotVersion,
		advanceSnapshotVersion: (version) => {
			syncedSnapshotVersion = Math.max(syncedSnapshotVersion, version);
		},
		addPendingAction: (action) => {
			const id = nextPendingActionID++;
			const entry = pendingEntry(
				id,
				action,
				selectView(state),
				state.chatStatus,
			);
			setState((current) => ({
				...current,
				pendingActions: [...current.pendingActions, entry],
			}));
			return id;
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
				state.streamError === null &&
				state.retryState === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
				streamState: null,
				streamError: null,
				retryState: null,
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
	return store;
};

type ChatView = Pick<
	ChatStoreState,
	"messagesByID" | "orderedMessageIDs" | "queuedMessages" | "streamState"
>;

// Cached per state object so selectors return the same values on every call,
// which useSyncExternalStore requires.
const views = new WeakMap<ChatStoreState, ChatView>();

const queuedRow = (
	message: TypesGen.ChatMessage,
): TypesGen.ChatQueuedMessage => ({
	id: message.id,
	chat_id: message.chat_id,
	model_config_id: message.model_config_id,
	content: message.content ?? [],
	created_at: message.created_at,
});

const sentQueuedMessage = (
	id: number,
	row: TypesGen.ChatQueuedMessage,
): TypesGen.ChatMessage => ({
	id,
	chat_id: row.chat_id,
	created_at: row.created_at,
	role: "user",
	content: row.content,
	model_config_id: row.model_config_id,
	queued_message_id: row.id,
});

const withoutRow =
	(id: number): QueueEdit =>
	(rows) =>
		rows.filter((row) => row.id !== id);

// Pending rows take IDs from the top of the safe integer range, above any ID
// the database assigns.
const pendingRowIDFloor = Number.MAX_SAFE_INTEGER - 2 ** 32;

/**
 * Whether a shown message or queued row is drawn by a pending action. Such a
 * row has no server identity, so nothing may act on it by ID.
 */
export const isPendingRowID = (id: number) => id > pendingRowIDFloor;

// The server places a request's message by the state the request finds
// (chatstate SendMessage and PromoteQueuedMessage); the entry places it by
// the state shown at the click.
const pendingEntry = (
	id: number,
	action: PendingAction,
	shown: ChatView,
	status: TypesGen.ChatStatus | null,
): PendingEntry => {
	// Above every stored message ID: it collides with none, and a later edit
	// hides it with the rest of the history.
	const eagerID = Number.MAX_SAFE_INTEGER - id;
	const running = status === "running" || status === "interrupting";
	switch (action.kind) {
		case "send": {
			const sent = { ...action.message, id: eagerID };
			const row = queuedRow(sent);
			// A busy chat queues the message.
			if (running || status === "requires_action") {
				return { id, message: { queue: (rows) => [...rows, row] } };
			}
			// An errored chat with a queue sends its head and queues this one.
			const head = status === "error" ? shown.queuedMessages[0] : undefined;
			if (head) {
				return {
					id,
					message: {
						shown: sentQueuedMessage(eagerID, head),
						queue: (rows) => [...withoutRow(head.id)(rows), row],
					},
				};
			}
			return { id, message: { shown: sent } };
		}
		case "promoteQueued": {
			const target = shown.queuedMessages.find(
				(row) => row.id === action.queuedMessageID,
			);
			if (!target) {
				return { id };
			}
			// A running chat moves the row to the head. A row already at the head
			// does not move and sends no queue update.
			if (running) {
				return {
					id,
					queue:
						shown.queuedMessages[0] === target
							? undefined
							: (rows) => [target, ...withoutRow(target.id)(rows)],
				};
			}
			return {
				id,
				message: {
					shown: sentQueuedMessage(eagerID, target),
					queue: withoutRow(target.id),
				},
			};
		}
		case "edit":
			return {
				id,
				history: {
					fromID: action.editedMessageID,
					draft: action.message && { ...action.message, id: eagerID },
				},
				// Clearing an empty queue sends no queue update.
				queue: shown.queuedMessages.length > 0 ? () => [] : undefined,
			};
		case "deleteQueued":
			return { id, queue: withoutRow(action.queuedMessageID) };
	}
};

// Drops the ended parts from each entry, and each entry left with none.
const endPendingParts = (
	entries: readonly PendingEntry[],
	ended: readonly PendingPart[],
): readonly PendingEntry[] =>
	entries.flatMap((entry) => {
		const message = ended.includes("message") ? undefined : entry.message;
		const queue = ended.includes("queue") ? undefined : entry.queue;
		const history = ended.includes("history") ? undefined : entry.history;
		if (!message && !queue && !history) {
			return [];
		}
		if (
			message === entry.message &&
			queue === entry.queue &&
			history === entry.history
		) {
			return [entry];
		}
		return [{ id: entry.id, message, queue, history }];
	});

// The stored state with each pending entry drawn over it in request order.
const selectView = (state: ChatStoreState): ChatView => {
	if (state.pendingActions.length === 0) {
		return state;
	}
	const cached = views.get(state);
	if (cached) {
		return cached;
	}
	const view: ChatView = {
		messagesByID: state.messagesByID,
		orderedMessageIDs: state.orderedMessageIDs,
		queuedMessages: state.queuedMessages,
		streamState: state.streamState,
	};
	const show = (message: TypesGen.ChatMessage) => {
		view.messagesByID = new Map(view.messagesByID).set(message.id, message);
		view.orderedMessageIDs = [...view.orderedMessageIDs, message.id];
	};
	for (const { message, queue, history } of state.pendingActions) {
		if (history) {
			view.messagesByID = new Map(
				[...view.messagesByID].filter(([id]) => id < history.fromID),
			);
			view.orderedMessageIDs = view.orderedMessageIDs.filter(
				(id) => id < history.fromID,
			);
			view.streamState = null;
			if (history.draft) {
				show(history.draft);
			}
		}
		if (queue) {
			view.queuedMessages = queue(view.queuedMessages);
		}
		if (message?.shown) {
			show(message.shown);
		}
		if (message?.queue) {
			view.queuedMessages = message.queue(view.queuedMessages);
		}
	}
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
export const selectChatStatus = (state: ChatStoreState) => state.chatStatus;
// A failure of this client, or else the error the chat's turn ended with.
export const selectStreamError = (state: ChatStoreState) =>
	state.streamError ?? state.turnError;
export const selectQueuedMessages = (state: ChatStoreState) =>
	selectView(state).queuedMessages;
export const selectSubagentStatusOverrides = (state: ChatStoreState) =>
	state.subagentStatusOverrides;
export const selectRetryState = (state: ChatStoreState) => state.retryState;
export const selectReconnectState = (state: ChatStoreState) =>
	state.reconnectState;
export const selectHasMoreMessages = (state: ChatStoreState) =>
	state.hasMoreMessages;
export const selectLoad = (state: ChatStoreState) => state.load;
export const selectIsLoaded = (state: ChatStoreState) =>
	state.load.status === "loaded";

const selectLatestMessage = (
	state: ChatView,
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
	const view = selectView(state);
	const latestMessage = selectLatestMessage(view);
	const latestMessageNeedsAssistantResponse =
		latestMessage?.role !== "assistant";
	// Show the Thinking indicator when the store has no stream
	// data yet, the chat is running, and the conversation is
	// waiting for an assistant response (any non-assistant latest
	// message).
	if (view.streamState !== null || !latestMessageNeedsAssistantResponse) {
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
