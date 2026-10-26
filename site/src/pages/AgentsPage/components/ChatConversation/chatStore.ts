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
 * A user request that the store shows from the click until the stream
 * reports its result or the request fails.
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
 * The expected result of a pending action, computed at the click from the
 * shown state. The store shows each part until the stream's next update of
 * that part's kind.
 */
type PendingEntry = {
	id: number;
	// The message the request adds. The next user message or queue update
	// ends it, because the server queues the message when the chat is busy.
	message?: { shown?: TypesGen.ChatMessage; queue?: QueueEdit };
	queue?: QueueEdit;
	// The history from the edited message on. A history reset ends it.
	history?: { fromID: number; draft?: TypesGen.ChatMessage };
};

type PendingPart = Exclude<keyof PendingEntry, "id">;

export type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	// A failure of this client, such as a rejected request.
	clientError: ChatDetailError | null;
	retryState: RetryState | null;
	// The error the chat's last reported turn ended with.
	turnError: ChatDetailError | null;
	reconnectState: ReconnectState | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	// Whether older messages exist below the oldest held one.
	hasMoreMessages: boolean;
	// These change only what view shows. view applies them in request order.
	pendingActions: readonly PendingEntry[];
	// Built once per state, so selectors reading one state get the same
	// values, which useSyncExternalStore requires.
	view: ChatView;
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	// Fills a new store with the newest messages page. After that, only
	// stream events and older pages change the stored state.
	seed: (page: TypesGen.ChatMessagesResponse) => void;
	// Replaces the whole history, as a history_reset from the first message.
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
	// Applies a status event, and clears the turn error unless the status is
	// error.
	applyServerChatStatus: (status: TypesGen.ChatStatus) => void;
	setTurnError: (reason: ChatDetailError) => void;
	// The highest snapshot version of the newest page and the applied events.
	getSyncedSnapshotVersion: () => number;
	advanceSnapshotVersion: (version: number) => void;
	addPendingAction: (action: PendingAction) => number;
	removePendingAction: (id: number) => void;
	setStreamState: (streamState: StreamState | null) => void;
	setClientError: (reason: ChatDetailError | null) => void;
	clearClientError: () => void;
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

const createInitialState = (): ChatStoreState => {
	const state: Omit<ChatStoreState, "view"> = {
		messagesByID: new Map(),
		orderedMessageIDs: [],
		streamState: null,
		chatStatus: null,
		clientError: null,
		retryState: null,
		turnError: null,
		reconnectState: null,
		queuedMessages: [],
		subagentStatusOverrides: new Map(),
		hasMoreMessages: false,
		pendingActions: [],
	};
	return { ...state, view: buildView(state) };
};

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
	// Bookkeeping, deliberately outside the rendered state so observing a
	// server event cannot trigger a re-render.
	let historyResets = 0;
	let syncedSnapshotVersion = 0;
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
		updater: (current: ChatStoreState) => Omit<ChatStoreState, "view">,
	): void => {
		const next = updater(state);
		if (next === state) {
			return;
		}
		state = { ...next, view: buildView(next) };
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

	const seed = (page: TypesGen.ChatMessagesResponse): void => {
		syncedSnapshotVersion = page.snapshot_version ?? 0;
		setState((current) => ({
			...current,
			messagesByID: buildMessageMap(page.messages),
			orderedMessageIDs: buildOrderedMessageIDs(page.messages),
			queuedMessages: page.queued_messages,
			hasMoreMessages: page.has_more,
		}));
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
		replaceMessages: (messages) => {
			historyResets++;
			batch(() => {
				replaceMessages(messages);
				endPending(["history"]);
			});
		},
		upsertDurableMessage: (message) => {
			let result = { isDuplicate: false, changed: false };
			batch(() => {
				result = upsertDurableMessage(message);
				if (message.role === "user") {
					endPending(["message"]);
				}
			});
			return result;
		},
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
		seed,
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
			const entry = pendingEntry(id, action, state.view, state.chatStatus);
			// An action that shows nothing gets no entry, and
			// removePendingAction ignores its ID.
			if (!entry.message && !entry.queue && !entry.history) {
				return id;
			}
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
		setClientError: (reason) => {
			setState((current) => {
				if (chatDetailErrorsEqual(current.clientError, reason)) {
					return current;
				}
				return {
					...current,
					clientError: reason,
				};
			});
		},
		clearClientError: () => {
			if (state.clientError === null) {
				return;
			}
			setState((current) => ({
				...current,
				clientError: null,
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
				state.clientError === null &&
				state.retryState === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
				streamState: null,
				clientError: null,
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
				state.chatStatus === null &&
				state.clientError === null &&
				state.retryState === null &&
				state.turnError === null &&
				state.reconnectState === null &&
				state.subagentStatusOverrides.size === 0
			) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
				chatStatus: null,
				clientError: null,
				retryState: null,
				turnError: null,
				reconnectState: null,
				subagentStatusOverrides: new Map(),
			}));
		},
	};
};

type ChatView = Pick<
	ChatStoreState,
	"messagesByID" | "orderedMessageIDs" | "queuedMessages" | "streamState"
>;

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
// the database assigns. They collide with no stored ID, and a later edit
// hides them with the rest of the history from the edited message on.
const pendingRowIDFloor = Number.MAX_SAFE_INTEGER - 2 ** 32;

/**
 * Whether a shown message or queued row comes from a pending action. The
 * server does not know such a row's ID, so nothing may act on it by ID.
 */
export const isPendingRowID = (id: number) => id > pendingRowIDFloor;

// pendingEntry places a request's message as chatstate SendMessage and
// PromoteQueuedMessage do, but by the state shown at the click. The server
// uses the state the request finds, which can differ.
const pendingEntry = (
	id: number,
	action: PendingAction,
	shown: ChatView,
	status: TypesGen.ChatStatus | null,
): PendingEntry => {
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
			// A running chat moves the row to the head. For a row already at the
			// head, the server sends no queue update.
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
				// The server sends no queue update when the queue is already empty.
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

// Returns the stored state with each pending entry applied in request order.
const buildView = (state: Omit<ChatStoreState, "view">): ChatView => {
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
	return view;
};

export const selectMessagesByID = (state: ChatStoreState) =>
	state.view.messagesByID;
export const selectOrderedMessageIDs = (state: ChatStoreState) =>
	state.view.orderedMessageIDs;
export const selectStreamState = (state: ChatStoreState) =>
	state.view.streamState;
export const selectHasStreamState = (state: ChatStoreState) =>
	state.view.streamState !== null;
export const selectChatStatus = (state: ChatStoreState) => state.chatStatus;
export const selectShownError = (state: ChatStoreState) =>
	state.clientError ?? state.turnError;
export const selectQueuedMessages = (state: ChatStoreState) =>
	state.view.queuedMessages;
export const selectSubagentStatusOverrides = (state: ChatStoreState) =>
	state.subagentStatusOverrides;
export const selectRetryState = (state: ChatStoreState) => state.retryState;
export const selectReconnectState = (state: ChatStoreState) =>
	state.reconnectState;
export const selectHasMoreMessages = (state: ChatStoreState) =>
	state.hasMoreMessages;

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
	const latestMessage = selectLatestMessage(state.view);
	const latestMessageNeedsAssistantResponse =
		latestMessage?.role !== "assistant";
	// Show the Thinking indicator when the store has no stream
	// data yet, the chat is running, and the conversation is
	// waiting for an assistant response (any non-assistant latest
	// message).
	if (state.view.streamState !== null || !latestMessageNeedsAssistantResponse) {
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
					"not found in messagesByID. This may indicate a store bug.",
			);
		}
		return message ? [message] : [];
	});
};

export { useChatStore } from "./useChatStore";
