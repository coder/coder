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
 * A user request whose expected result the store shows from the click; see
 * PendingEntry for when each part ends.
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
 * shown state. Each part ends at the first stream event of its kind above
 * syncedVersion, even if the event changes nothing in the stored state:
 *
 * - message: a user message or a queue update, because the server queues a
 *   send, and keeps a promoted row in the queue, if it finds the chat busy.
 * - editedMessage: a user message, because an edit never queues.
 * - queue: a queue update. For a delete, only one without the row, because a
 *   row's ID never returns to the queue.
 * - history: a history reset.
 *
 * An edit has editedMessage, history and, if the queue is not empty, queue.
 * A stream open resends the commit at the version it opens with. An entry
 * synced at that version ignores the resend. For an entry synced below it,
 * each resent event either ended its part when it first arrived, so ending
 * it again changes nothing, or was lost when the connection dropped, so it
 * ends the part now. The first open after a page load also resends the
 * commits above the page's snapshot version that the page holds, because the
 * server reads that version before the rest of the page. Such a commit ends
 * the part of an action clicked before the first sync, so the action's result
 * disappears until its own event arrives.
 */
type PendingEntry = {
	id: number;
	// The snapshot version synced at the click. An event at or below it is
	// from a commit before the request.
	syncedVersion: number;
	// Set for a delete.
	isRowGone?: (rows: readonly TypesGen.ChatQueuedMessage[]) => boolean;
	// The message a send or promote adds, in the transcript or the queue.
	message?: { shown?: TypesGen.ChatMessage; queue?: QueueEdit };
	editedMessage?: TypesGen.ChatMessage;
	// The queue change of a delete, promote, edit or errored-chat send.
	queue?: QueueEdit;
	// Hides the history from the edited message on.
	history?: { fromID: number };
};

type PendingPart = "message" | "editedMessage" | "queue" | "history";

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
	pendingEntries: readonly PendingEntry[];
	// Built once per state, so selectors reading one state get the same
	// values, which useSyncExternalStore requires.
	view: ChatView;
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	// Fills a new store with the newest messages page. After that, only
	// stream events and older pages change its messages and queue.
	seed: (page: TypesGen.ChatMessagesResponse) => void;
	// Applies a history_reset: drops the messages with IDs at or above fromID.
	resetHistoryFrom: (fromID: number, snapshotVersion: number) => void;
	getHistoryResetCount: () => number;
	// Drops the page if a history reset was applied since resetCount.
	prependOlderMessages: (
		messages: readonly TypesGen.ChatMessage[],
		hasMore: boolean,
		resetCount: number,
	) => void;
	// userMessageVersion is the highest snapshot version of the user messages.
	upsertDurableMessages: (
		messages: readonly TypesGen.ChatMessage[],
		userMessageVersion: number,
	) => void;
	applyMessagePart: (part: TypesGen.ChatMessagePart) => void;
	applyMessageParts: (parts: readonly TypesGen.ChatMessagePart[]) => void;
	setQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
		snapshotVersion: number,
	) => void;
	// Applies a status event, and clears the turn error unless the status is
	// error.
	applyServerChatStatus: (status: TypesGen.ChatStatus) => void;
	setTurnError: (reason: ChatDetailError) => void;
	// The highest snapshot version of the newest page and the applied events.
	getSyncedSnapshotVersion: () => number;
	advanceSnapshotVersion: (version: number) => void;
	// Shows the action until a stream update ends it or request rejects.
	// status is the chat status the page shows, which places the message.
	runPendingAction: (
		action: PendingAction,
		status: TypesGen.ChatStatus | null,
		request: () => Promise<unknown>,
	) => Promise<void>;
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
		pendingEntries: [],
	};
	return { ...state, view: buildView(state) };
};

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
	// Kept outside the rendered state, so changing them never triggers a
	// re-render.
	let historyResets = 0;
	let syncedSnapshotVersion = 0;
	// The version of the newest reset the store holds. The newest page holds
	// every reset up to its snapshot version.
	let resetVersion = 0;
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

	const endPending = (
		parts: readonly PendingPart[],
		version: number,
		rows?: readonly TypesGen.ChatQueuedMessage[],
	): void => {
		setState((current) => {
			const pendingEntries = endPendingParts(
				current.pendingEntries,
				parts,
				version,
				rows,
			);
			return arraysEqual(pendingEntries, current.pendingEntries)
				? current
				: { ...current, pendingEntries };
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
		resetVersion = syncedSnapshotVersion;
		setState((current) => ({
			...current,
			messagesByID: buildMessageMap(page.messages),
			orderedMessageIDs: buildOrderedMessageIDs(page.messages),
			queuedMessages: page.queued_messages,
			hasMoreMessages: page.has_more,
		}));
	};

	const writeMessages = (messages: readonly TypesGen.ChatMessage[]): void => {
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

	const addPendingAction = (
		action: PendingAction,
		status: TypesGen.ChatStatus | null,
	): number => {
		const id = nextPendingActionID++;
		const entry = pendingEntry(id, action, state.view, status);
		// An action that shows nothing gets no entry, and
		// removePendingAction ignores its ID.
		if (
			!entry.message &&
			!entry.editedMessage &&
			!entry.queue &&
			!entry.history
		) {
			return id;
		}
		setState((current) => ({
			...current,
			pendingEntries: [
				...current.pendingEntries,
				{ ...entry, syncedVersion: syncedSnapshotVersion },
			],
		}));
		return id;
	};

	const removePendingAction = (id: number): void => {
		setState((current) =>
			current.pendingEntries.some((entry) => entry.id === id)
				? {
						...current,
						pendingEntries: current.pendingEntries.filter(
							(entry) => entry.id !== id,
						),
					}
				: current,
		);
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
		upsertDurableMessages: (messages, userMessageVersion) => {
			batch(() => {
				writeMessages(messages);
				if (messages.some((message) => message.role === "user")) {
					endPending(["message", "editedMessage"], userMessageVersion);
				}
			});
		},
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		setQueuedMessages: (queuedMessages, snapshotVersion) => {
			batch(() => {
				setState((current) =>
					chatQueuedMessagesEqualByID(current.queuedMessages, queuedMessages)
						? current
						: { ...current, queuedMessages },
				);
				endPending(["message", "queue"], snapshotVersion, queuedMessages);
			});
		},
		seed,
		resetHistoryFrom: (fromID, snapshotVersion) => {
			// A commit has at most one reset, so a reset at or below
			// resetVersion is a resend.
			if (snapshotVersion <= resetVersion) {
				return;
			}
			resetVersion = snapshotVersion;
			historyResets++;
			batch(() => {
				replaceMessages(
					[...state.messagesByID.values()].filter(
						(message) => message.id < fromID,
					),
				);
				endPending(["history"], snapshotVersion);
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
			writeMessages(older);
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
		runPendingAction: async (action, status, request) => {
			const id = addPendingAction(action, status);
			try {
				await request();
			} catch (error) {
				removePendingAction(id);
				throw error;
			}
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
// the database assigns, so a later edit hides them with the rest of the
// history from the edited message on. The IDs stay above the floor while a
// store creates fewer than maxPendingActions actions.
const maxPendingActions = 2 ** 32;
const pendingRowIDFloor = Number.MAX_SAFE_INTEGER - maxPendingActions;

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
	view: ChatView,
	status: TypesGen.ChatStatus | null,
): Omit<PendingEntry, "syncedVersion"> => {
	const pendingRowID = Number.MAX_SAFE_INTEGER - id;
	const running = status === "running" || status === "interrupting";
	switch (action.kind) {
		case "send": {
			const sent = { ...action.message, id: pendingRowID };
			const row = queuedRow(sent);
			// A busy chat queues the message.
			if (isActiveChatStatus(status)) {
				return { id, message: { queue: (rows) => [...rows, row] } };
			}
			// An errored chat with a queue sends its head and queues this one.
			const head = status === "error" ? view.queuedMessages[0] : undefined;
			if (head) {
				return {
					id,
					message: { shown: sentQueuedMessage(pendingRowID, head) },
					queue: (rows) => [...withoutRow(head.id)(rows), row],
				};
			}
			return { id, message: { shown: sent } };
		}
		case "promoteQueued": {
			const target = view.queuedMessages.find(
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
						view.queuedMessages[0] === target
							? undefined
							: (rows) => [target, ...withoutRow(target.id)(rows)],
				};
			}
			return {
				id,
				message: { shown: sentQueuedMessage(pendingRowID, target) },
				queue: withoutRow(target.id),
			};
		}
		case "edit":
			return {
				id,
				history: { fromID: action.editedMessageID },
				editedMessage: action.message && {
					...action.message,
					id: pendingRowID,
				},
				// The server sends no queue update when the queue is already empty.
				queue: view.queuedMessages.length > 0 ? () => [] : undefined,
			};
		case "deleteQueued":
			return {
				id,
				isRowGone: (rows) =>
					!rows.some((row) => row.id === action.queuedMessageID),
				queue: withoutRow(action.queuedMessageID),
			};
	}
};

const endPendingParts = (
	entries: readonly PendingEntry[],
	ended: readonly PendingPart[],
	version: number,
	rows?: readonly TypesGen.ChatQueuedMessage[],
): readonly PendingEntry[] =>
	entries.flatMap((entry) => {
		if (
			version <= entry.syncedVersion ||
			(rows && entry.isRowGone && !entry.isRowGone(rows))
		) {
			return [entry];
		}
		const message = ended.includes("message") ? undefined : entry.message;
		const editedMessage = ended.includes("editedMessage")
			? undefined
			: entry.editedMessage;
		const queue = ended.includes("queue") ? undefined : entry.queue;
		const history = ended.includes("history") ? undefined : entry.history;
		if (!message && !editedMessage && !queue && !history) {
			return [];
		}
		if (
			message === entry.message &&
			editedMessage === entry.editedMessage &&
			queue === entry.queue &&
			history === entry.history
		) {
			return [entry];
		}
		return [{ ...entry, message, editedMessage, queue, history }];
	});

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
	for (const {
		message,
		editedMessage,
		queue,
		history,
	} of state.pendingEntries) {
		if (history) {
			view.messagesByID = new Map(
				[...view.messagesByID].filter(([id]) => id < history.fromID),
			);
			view.orderedMessageIDs = view.orderedMessageIDs.filter(
				(id) => id < history.fromID,
			);
			view.streamState = null;
		}
		if (editedMessage) {
			show(editedMessage);
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

const latestMessage = (view: ChatView): TypesGen.ChatMessage | undefined => {
	const latestMessageID =
		view.orderedMessageIDs[view.orderedMessageIDs.length - 1];
	return latestMessageID === undefined
		? undefined
		: view.messagesByID.get(latestMessageID);
};

export const selectIsAwaitingFirstStreamChunk = (
	state: ChatStoreState,
): boolean => {
	const latestMessageNeedsAssistantResponse =
		latestMessage(state.view)?.role !== "assistant";
	// A pending edit clears the shown stream state, so an edit to a running
	// chat shows Thinking.
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

/**
 * The shown messages, oldest first. They include pending rows, so check an ID
 * with isPendingRowID before sending it to the server.
 */
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
