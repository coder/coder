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
 * A turn completes in `waiting`, or in `paused` when the queue head is under
 * edit. A turn that ends in `error` did not complete.
 */
export const isTurnCompletedChatStatus = (
	status: TypesGen.ChatStatus,
): boolean => status === "waiting" || status === "paused";

/**
 * A user request whose expected result the store shows from the click until
 * the stream reports it.
 */
export type PendingAction =
	| {
			kind: "edit";
			editedMessageID: number;
			content: readonly TypesGen.ChatMessagePart[];
	  }
	| {
			kind: "send";
			chatID: string;
			content: readonly TypesGen.ChatMessagePart[];
			modelConfigID?: string;
			busyBehavior: TypesGen.ChatBusyBehavior;
	  }
	| { kind: "deleteQueued"; queuedMessageID: number }
	| { kind: "promoteQueued"; queuedMessageID: number }
	// A queued-message edit: editing begins or ends the row's edit, and
	// content is set for a save.
	| {
			kind: "editQueued";
			queuedMessageID: number;
			editing: boolean;
			content?: readonly TypesGen.ChatMessagePart[];
	  };

type StreamChange = "user_message" | "queue_update" | "history_reset";

/**
 * What a pending action shows, computed at the click from the shown state.
 * The server can place the change differently, because it uses the state
 * that the request finds.
 */
type PendingPart = {
	actionID: number;
	// The snapshot version synced at the click. An event at or below it
	// reports a change made before the request.
	syncedVersion: number;
	// The part ends at the first of these changes above syncedVersion, even
	// if another request made that change.
	endsAt: readonly StreamChange[];
	// Hides the transcript from this message on, and the response that the
	// agent is writing.
	hideFromID?: number;
	// Added to the end of the transcript.
	messages?: readonly TypesGen.ChatMessage[];
	// Replaces the queue.
	queuedMessages?: readonly TypesGen.ChatQueuedMessage[];
};

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
	// These change only what view shows. view applies them in click order.
	pendingParts: readonly PendingPart[];
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
	// Requests the page before the oldest stored message and adds it. It
	// drops the page if a history reset was applied during the request,
	// because the page can then contain messages that the edit deleted.
	loadOlderMessages: (
		fetchPage: (beforeID: number) => Promise<TypesGen.ChatMessagesResponse>,
	) => Promise<void>;
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
	// Shows the action's result until the stream reports it or request
	// rejects.
	runPendingAction: (
		action: PendingAction,
		request: () => Promise<unknown>,
	) => Promise<void>;
	setClientError: (reason: ChatDetailError) => void;
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
		pendingParts: [],
	};
	return { ...state, view: buildView(state) };
};

export const createChatStore = (): ChatStore => {
	let state = createInitialState();
	// Kept outside the rendered state, so changing them never triggers a
	// re-render.
	let syncedSnapshotVersion = 0;
	// Every reset at or below this version is in the store.
	let historyResetVersion = 0;
	let nextPendingActionID = 1;
	let nextPendingRowID = Number.MAX_SAFE_INTEGER;
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

	const endPending = (change: StreamChange, version: number): void => {
		setState((current) => {
			const pendingParts = current.pendingParts.filter(
				(part) =>
					version <= part.syncedVersion || !part.endsAt.includes(change),
			);
			return pendingParts.length === current.pendingParts.length
				? current
				: { ...current, pendingParts };
		});
	};

	const seed = (page: TypesGen.ChatMessagesResponse): void => {
		syncedSnapshotVersion = page.snapshot_version ?? 0;
		historyResetVersion = syncedSnapshotVersion;
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
					endPending("user_message", userMessageVersion);
				}
			});
		},
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		setQueuedMessages: (queuedMessages, snapshotVersion) => {
			batch(() => {
				setState((current) =>
					isEqual(current.queuedMessages, queuedMessages)
						? current
						: { ...current, queuedMessages },
				);
				endPending("queue_update", snapshotVersion);
			});
		},
		seed,
		resetHistoryFrom: (fromID, snapshotVersion) => {
			// A reset at or below historyResetVersion is already applied.
			if (snapshotVersion <= historyResetVersion) {
				return;
			}
			historyResetVersion = snapshotVersion;
			batch(() => {
				setState((current) => ({
					...current,
					messagesByID: new Map(
						[...current.messagesByID].filter(([id]) => id < fromID),
					),
					orderedMessageIDs: current.orderedMessageIDs.filter(
						(id) => id < fromID,
					),
				}));
				endPending("history_reset", snapshotVersion);
			});
		},
		loadOlderMessages: async (fetchPage) => {
			const oldestID = state.orderedMessageIDs[0];
			if (oldestID === undefined) {
				return;
			}
			const resetVersion = historyResetVersion;
			const page = await fetchPage(oldestID);
			if (resetVersion !== historyResetVersion) {
				return;
			}
			batch(() => {
				writeMessages(
					page.messages.filter(
						(message) => !state.messagesByID.has(message.id),
					),
				);
				setState((current) =>
					current.hasMoreMessages === page.has_more
						? current
						: { ...current, hasMoreMessages: page.has_more },
				);
			});
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
		runPendingAction: async (action, request) => {
			const actionID = nextPendingActionID++;
			const syncedVersion = syncedSnapshotVersion;
			const parts = pendingChanges(
				action,
				state.view,
				state.chatStatus,
				() => nextPendingRowID--,
			).map((change) => ({ ...change, actionID, syncedVersion }));
			setState((current) => ({
				...current,
				pendingParts: [...current.pendingParts, ...parts],
			}));
			try {
				await request();
			} catch (error) {
				setState((current) => ({
					...current,
					pendingParts: current.pendingParts.filter(
						(part) => part.actionID !== actionID,
					),
				}));
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

// A pending message or queued row takes its ID from the top of the safe
// integer range, above every ID that the database assigns.
const pendingRowIDFloor = Number.MAX_SAFE_INTEGER - 2 ** 32;

/**
 * Whether a shown message or queued row comes from a pending action. The
 * server does not know such a row's ID, so nothing may act on it by ID.
 */
export const isPendingRowID = (id: number) => id > pendingRowIDFloor;

const isSteerRow = (row: TypesGen.ChatQueuedMessage) =>
	row.busy_behavior === "steer";

// The queue holds the steer lane first, then the queue lane.
const appendToLane = (
	rows: readonly TypesGen.ChatQueuedMessage[],
	row: TypesGen.ChatQueuedMessage,
): readonly TypesGen.ChatQueuedMessage[] =>
	isSteerRow(row)
		? [
				...rows.filter(isSteerRow),
				row,
				...rows.filter((other) => !isSteerRow(other)),
			]
		: [...rows, row];

// The rows that the server sends next, as chatstate nextDelivery selects
// them: the steer lane, or the first row when the steer lane is empty. A row
// under edit holds them.
const nextDelivery = (
	rows: readonly TypesGen.ChatQueuedMessage[],
): readonly TypesGen.ChatQueuedMessage[] => {
	const steerLane = rows.filter(isSteerRow);
	const next = steerLane.length > 0 ? steerLane : rows.slice(0, 1);
	return next.some((row) => row.editing_since) ? [] : next;
};

type PendingChange = Omit<PendingPart, "actionID" | "syncedVersion">;

// A change to the queue. If the chat resumes, the server also sends the
// next rows, which then show as messages.
const queueChange = (
	rows: readonly TypesGen.ChatQueuedMessage[],
	resumes: boolean,
	endsAt: readonly StreamChange[],
	nextRowID: () => number,
): PendingChange => {
	const sent = resumes ? nextDelivery(rows) : [];
	return {
		endsAt,
		messages: sent.map((row) => ({
			id: nextRowID(),
			chat_id: row.chat_id,
			created_at: row.created_at,
			role: "user",
			content: row.content,
			model_config_id: row.model_config_id,
		})),
		queuedMessages: rows.filter((row) => !sent.includes(row)),
	};
};

// pendingChanges places an action's result as the chatstate transitions
// SendMessage, EditMessage, SteerQueuedMessage, DeleteQueuedMessage and
// EditQueuedMessage do. A send or a promote resumes a chat that runs no
// turn. A delete or the end of a row's edit resumes only a paused chat.
const pendingChanges = (
	action: PendingAction,
	view: ChatView,
	status: TypesGen.ChatStatus | null,
	nextRowID: () => number,
): readonly PendingChange[] => {
	const rows = view.queuedMessages;
	const isIdle = !isActiveChatStatus(status);
	const isPaused = status === "paused";
	if (action.kind === "edit") {
		const edited = view.messagesByID.get(action.editedMessageID);
		const transcript: PendingChange = {
			endsAt: ["history_reset"],
			hideFromID: action.editedMessageID,
			messages: edited && [
				{ ...edited, id: nextRowID(), content: action.content },
			],
		};
		// An edit clears the queue. The server sends no queue_update when the
		// queue is already empty.
		return rows.length > 0
			? [transcript, { endsAt: ["queue_update"], queuedMessages: [] }]
			: [transcript];
	}
	if (action.kind === "send") {
		const row: TypesGen.ChatQueuedMessage = {
			id: nextRowID(),
			chat_id: action.chatID,
			model_config_id: action.modelConfigID,
			content: action.content,
			busy_behavior: action.busyBehavior,
			created_at: new Date().toISOString(),
		};
		return [
			queueChange(
				appendToLane(rows, row),
				isIdle,
				["user_message", "queue_update"],
				nextRowID,
			),
		];
	}
	const target = rows.find((row) => row.id === action.queuedMessageID);
	if (!target) {
		return [];
	}
	const others = rows.filter((row) => row !== target);
	switch (action.kind) {
		case "promoteQueued":
			return [
				queueChange(
					appendToLane(others, {
						...target,
						busy_behavior: "steer",
						editing_since: undefined,
					}),
					isIdle,
					["user_message", "queue_update"],
					nextRowID,
				),
			];
		case "deleteQueued":
			return [queueChange(others, isPaused, ["queue_update"], nextRowID)];
		case "editQueued": {
			const edited = {
				...target,
				content: action.content ?? target.content,
				editing_since: action.editing
					? (target.editing_since ?? new Date().toISOString())
					: undefined,
			};
			return [
				queueChange(
					rows.map((row) => {
						if (row === target) {
							return edited;
						}
						// A chat has at most one row under edit.
						return action.editing && row.editing_since
							? { ...row, editing_since: undefined }
							: row;
					}),
					isPaused && !action.editing,
					["queue_update"],
					nextRowID,
				),
			];
		}
	}
};

const buildView = (state: Omit<ChatStoreState, "view">): ChatView => {
	const view: ChatView = {
		messagesByID: state.messagesByID,
		orderedMessageIDs: state.orderedMessageIDs,
		queuedMessages: state.queuedMessages,
		streamState: state.streamState,
	};
	for (const part of state.pendingParts) {
		const { hideFromID, messages = [], queuedMessages } = part;
		if (hideFromID !== undefined) {
			view.orderedMessageIDs = view.orderedMessageIDs.filter(
				(id) => id < hideFromID,
			);
			view.streamState = null;
		}
		if (hideFromID !== undefined || messages.length > 0) {
			view.messagesByID = new Map(
				[...view.messagesByID].filter(
					([id]) => hideFromID === undefined || id < hideFromID,
				),
			);
			for (const message of messages) {
				view.messagesByID.set(message.id, message);
			}
			view.orderedMessageIDs = [
				...view.orderedMessageIDs,
				...messages.map((message) => message.id),
			];
		}
		if (queuedMessages) {
			view.queuedMessages = queuedMessages;
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
/**
 * The oldest stored message's ID. Unlike the oldest row useChatMessages
 * shows, it is never a pending row's ID, so it is a valid before_id.
 */
export const selectOldestStoredMessageID = (state: ChatStoreState) =>
	state.orderedMessageIDs[0];

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
	const latest = latestMessage(state.view);
	// The server has not started a response to a pending message.
	if (
		state.view.streamState !== null ||
		latest?.role === "assistant" ||
		(latest && isPendingRowID(latest.id))
	) {
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
