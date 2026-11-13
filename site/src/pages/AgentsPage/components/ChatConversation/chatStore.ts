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
 * A user request whose direct effect the store shows from the click until
 * the stream stores its result.
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

type PendingEntry = {
	actionID: number;
	action: PendingAction;
	// A send's or an edit's message, added to the end of the transcript.
	message?: TypesGen.ChatMessage;
	// A send's new row, or the acted-on row as a promote or a queued edit
	// shows it.
	row?: TypesGen.ChatQueuedMessage;
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
	// The IDs of the queued rows that stored messages were delivered from.
	// A row ID never returns, so the set only grows.
	deliveredQueuedIDs: ReadonlySet<number>;
	// These change only what view shows. view applies them in click order.
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
	// Requests the page before the oldest stored message and adds it. It
	// drops the page if a history reset was applied during the request,
	// because the page can then contain messages that the edit deleted.
	loadOlderMessages: (
		fetchPage: (beforeID: number) => Promise<TypesGen.ChatMessagesResponse>,
	) => Promise<void>;
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
	// Shows the action's direct effect until the stream stores its result
	// or request rejects.
	runPendingAction: (
		action: PendingAction,
		request: () => Promise<unknown>,
	) => Promise<void>;
	// Call when a stream connection opens. The returned function ends the
	// pending actions whose requests had settled by then; call it in the
	// store update that applies the connection's first events.
	captureSettledActions: () => () => void;
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
		deliveredQueuedIDs: new Set(),
		pendingEntries: [],
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
	// The pending actions whose requests resolved.
	let settledActionIDs = new Set<number>();
	// The highest IDs stored. User content above them is new.
	let highestMessageID = 0;
	let highestQueuedID = 0;
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
		state = { ...next, view: buildView(next, state) };
		if (batchDepth > 0) {
			batchDirty = true;
		} else {
			emit();
		}
	};

	const removePending = (isEnded: (entry: PendingEntry) => boolean): void => {
		setState((current) => {
			const pendingEntries = current.pendingEntries.filter(
				(entry) => !isEnded(entry),
			);
			return pendingEntries.length === current.pendingEntries.length
				? current
				: { ...current, pendingEntries };
		});
	};

	// A send's queued row or delivered message counts once, whichever comes
	// first, by a row ID above highestQueuedID. A user message without a row,
	// from a direct send or an edit, counts by an ID above highestMessageID.
	const countNewUserContent = (
		messages: readonly TypesGen.ChatMessage[],
		rows: readonly TypesGen.ChatQueuedMessage[],
	): number =>
		messages.filter(
			({ id, role, queued_message_id: queuedID }) =>
				role === "user" &&
				(queuedID === undefined
					? id > highestMessageID
					: queuedID > highestQueuedID),
		).length + rows.filter((row) => row.id > highestQueuedID).length;

	const raiseHighestIDs = (
		messages: readonly TypesGen.ChatMessage[],
		rows: readonly TypesGen.ChatQueuedMessage[],
	): void => {
		for (const { id, queued_message_id: queuedID = 0 } of messages) {
			highestMessageID = Math.max(highestMessageID, id);
			highestQueuedID = Math.max(highestQueuedID, queuedID);
		}
		highestQueuedID = Math.max(highestQueuedID, ...rows.map((row) => row.id));
	};

	// Each piece of new user content ends the oldest pending send or edit. A
	// stored queue also ends the queued-row actions whose result it holds.
	const endPending = (
		messages: readonly TypesGen.ChatMessage[],
		storedQueue?: readonly TypesGen.ChatQueuedMessage[],
	): void => {
		const newUserContent = countNewUserContent(messages, storedQueue ?? []);
		raiseHighestIDs(messages, storedQueue ?? []);
		const endedTurns = state.pendingEntries
			.filter(({ action }) => queuedRowID(action) === undefined)
			.slice(0, newUserContent);
		removePending(
			(entry) =>
				endedTurns.includes(entry) ||
				(storedQueue !== undefined &&
					queueHoldsResult(entry.action, storedQueue)),
		);
	};

	const seed = (page: TypesGen.ChatMessagesResponse): void => {
		syncedSnapshotVersion = page.snapshot_version ?? 0;
		historyResetVersion = syncedSnapshotVersion;
		raiseHighestIDs(page.messages, page.queued_messages);
		setState((current) => ({
			...current,
			messagesByID: buildMessageMap(page.messages),
			orderedMessageIDs: buildOrderedMessageIDs(page.messages),
			queuedMessages: page.queued_messages,
			hasMoreMessages: page.has_more,
			deliveredQueuedIDs: deliveredFrom(page.messages),
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
				deliveredQueuedIDs: deliveredFrom(messages, current.deliveredQueuedIDs),
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
		upsertDurableMessages: (messages) => {
			batch(() => {
				writeMessages(messages);
				endPending(messages);
			});
		},
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		setQueuedMessages: (queuedMessages) => {
			batch(() => {
				setState((current) =>
					isEqual(current.queuedMessages, queuedMessages)
						? current
						: { ...current, queuedMessages },
				);
				endPending([], queuedMessages);
			});
		},
		seed,
		resetHistoryFrom: (fromID, snapshotVersion) => {
			if (snapshotVersion <= historyResetVersion) {
				return;
			}
			historyResetVersion = snapshotVersion;
			setState((current) => ({
				...current,
				messagesByID: new Map(
					[...current.messagesByID].filter(([id]) => id < fromID),
				),
				orderedMessageIDs: current.orderedMessageIDs.filter(
					(id) => id < fromID,
				),
			}));
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
			// The server runs a turn for an earlier pending message by the time
			// this request arrives.
			const runsTurn =
				isActiveChatStatus(state.chatStatus) ||
				state.pendingEntries.some((earlier) => earlier.message);
			const entry: PendingEntry = {
				actionID,
				action,
				...pendingEffect(
					action,
					state.view,
					runsTurn,
					() => nextPendingRowID--,
				),
			};
			const rowID = queuedRowID(action);
			setState((current) => ({
				...current,
				// A queued row has one pending action, the latest. A chat has one
				// row under edit, so a begin also replaces an earlier begin on
				// another row, whose result can no longer show.
				pendingEntries: [
					...current.pendingEntries.filter(
						({ action: earlier }) =>
							(rowID === undefined || queuedRowID(earlier) !== rowID) &&
							!(
								action.kind === "editQueued" &&
								action.editing &&
								earlier.kind === "editQueued" &&
								earlier.editing
							),
					),
					entry,
				],
			}));
			try {
				await request();
			} catch (error) {
				removePending((pending) => pending.actionID === actionID);
				throw error;
			}
			settledActionIDs.add(actionID);
		},
		captureSettledActions: () => {
			const settledAtOpen = new Set(
				state.pendingEntries
					.map((entry) => entry.actionID)
					.filter((id) => settledActionIDs.has(id)),
			);
			// Forgets the actions that already ended.
			settledActionIDs = new Set(settledAtOpen);
			return () => removePending((entry) => settledAtOpen.has(entry.actionID));
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

const deliveredFrom = (
	messages: readonly TypesGen.ChatMessage[],
	delivered: ReadonlySet<number> = new Set(),
): ReadonlySet<number> =>
	new Set([
		...delivered,
		...messages.flatMap((message) => message.queued_message_id ?? []),
	]);

const queuedRowID = (action: PendingAction): number | undefined =>
	"queuedMessageID" in action ? action.queuedMessageID : undefined;

// Whether the stored queue holds the result of an action on a queued row.
const queueHoldsResult = (
	action: PendingAction,
	rows: readonly TypesGen.ChatQueuedMessage[],
): boolean => {
	if (!("queuedMessageID" in action)) {
		return false;
	}
	const row = rows.find((stored) => stored.id === action.queuedMessageID);
	return (
		!row ||
		(action.kind === "promoteQueued" && isSteerRow(row)) ||
		(action.kind === "editQueued" &&
			Boolean(row.editing_since) === action.editing)
	);
};

// What a pending action shows, computed at the click from the shown state. A
// send shows its row where the chatstate SendMessage transition places it:
// as a message when a chat that runs no turn delivers the row next, else in
// its lane. Changes to other rows show when the stream stores them.
const pendingEffect = (
	action: PendingAction,
	view: ChatView,
	runsTurn: boolean,
	nextRowID: () => number,
): Pick<PendingEntry, "message" | "row"> => {
	if (action.kind === "edit") {
		const edited = view.messagesByID.get(action.editedMessageID);
		return {
			message: edited && {
				...edited,
				id: nextRowID(),
				content: action.content,
			},
		};
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
		if (
			runsTurn ||
			!nextDelivery(appendToLane(view.queuedMessages, row)).includes(row)
		) {
			return { row };
		}
		const { busy_behavior: _, ...message } = row;
		return { message: { ...message, role: "user" } };
	}
	const target = view.queuedMessages.find(
		(row) => row.id === action.queuedMessageID,
	);
	if (!target || action.kind === "deleteQueued") {
		return {};
	}
	if (action.kind === "promoteQueued") {
		return {
			row: { ...target, busy_behavior: "steer", editing_since: undefined },
		};
	}
	return {
		row: {
			...target,
			content: action.content ?? target.content,
			editing_since: action.editing
				? (target.editing_since ?? new Date().toISOString())
				: undefined,
		},
	};
};

const buildView = (
	state: Omit<ChatStoreState, "view">,
	previous?: ChatStoreState,
): ChatView => {
	let { messagesByID, orderedMessageIDs, streamState } = state;
	for (const { action, message } of state.pendingEntries) {
		if (action.kind === "edit") {
			// Hides the transcript from the edited message on, and the response
			// that the agent is writing.
			const fromID = action.editedMessageID;
			messagesByID = new Map([...messagesByID].filter(([id]) => id < fromID));
			orderedMessageIDs = orderedMessageIDs.filter((id) => id < fromID);
			streamState = null;
		}
		if (message) {
			messagesByID = new Map(messagesByID).set(message.id, message);
			orderedMessageIDs = [...orderedMessageIDs, message.id];
		}
	}
	// A new shown queue would re-render its readers on every message part.
	const queueUnchanged =
		previous !== undefined &&
		state.queuedMessages === previous.queuedMessages &&
		state.pendingEntries === previous.pendingEntries &&
		state.deliveredQueuedIDs === previous.deliveredQueuedIDs;
	return {
		messagesByID,
		orderedMessageIDs,
		streamState,
		queuedMessages: queueUnchanged
			? previous.view.queuedMessages
			: buildShownQueue(state),
	};
};

const buildShownQueue = (
	state: Omit<ChatStoreState, "view">,
): readonly TypesGen.ChatQueuedMessage[] => {
	let queuedMessages = state.queuedMessages;
	for (const { action, row } of state.pendingEntries) {
		const rowID = queuedRowID(action);
		const others = queuedMessages.filter((other) => other.id !== rowID);
		if (action.kind === "deleteQueued") {
			queuedMessages = others;
		} else if (action.kind === "editQueued" && row) {
			// A chat has at most one row under edit.
			queuedMessages = queuedMessages.map((other) =>
				other.id === rowID
					? row
					: action.editing && other.editing_since
						? { ...other, editing_since: undefined }
						: other,
			);
		} else if (
			row &&
			(action.kind === "send" || others.length < queuedMessages.length)
		) {
			// A send's row, or a promoted row that the queue still holds.
			queuedMessages = appendToLane(others, row);
		}
	}
	// The stream can store a row's message before the queue_update that
	// removes the row.
	const { deliveredQueuedIDs } = state;
	if (queuedMessages.some((row) => deliveredQueuedIDs.has(row.id))) {
		queuedMessages = queuedMessages.filter(
			(row) => !deliveredQueuedIDs.has(row.id),
		);
	}
	return queuedMessages;
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
