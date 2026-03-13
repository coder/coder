import type * as TypesGen from "api/typesGenerated";
import { applyMessagePartToStreamState } from "./streamState";
import type { StreamState } from "./types";

export const VALID_CHAT_STATUSES: ReadonlySet<string> =
	new Set<TypesGen.ChatStatus>([
		"pending",
		"running",
		"completed",
		"error",
		"paused",
		"waiting",
	]);

export const isValidChatStatus = (
	value: unknown,
): value is TypesGen.ChatStatus =>
	typeof value === "string" && VALID_CHAT_STATUSES.has(value);

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
	return sorted.map((message) => message.id);
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

const chatMessagesEqualByValue = (
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

const chatQueuedMessagesEqualByID = (
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

export type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	streamError: string | null;
	retryState: { attempt: number; error: string } | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
};

export type ChatStore = {
	getSnapshot: () => ChatStoreState;
	subscribe: (listener: () => void) => () => void;
	replaceMessages: (
		messages: readonly TypesGen.ChatMessage[] | undefined,
	) => void;
	upsertDurableMessage: (message: TypesGen.ChatMessage) => {
		isDuplicate: boolean;
		changed: boolean;
	};
	applyMessagePart: (part: Record<string, unknown>) => void;
	applyMessageParts: (parts: readonly Record<string, unknown>[]) => void;
	setQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	) => void;
	setChatStatus: (status: TypesGen.ChatStatus | null) => void;
	setStreamError: (reason: string | null) => void;
	clearStreamError: () => void;
	setRetryState: (state: { attempt: number; error: string } | null) => void;
	clearRetryState: () => void;
	clearStreamState: () => void;
	setSubagentStatusOverride: (
		chatID: string,
		status: TypesGen.ChatStatus,
	) => void;
	resetTransientState: () => void;
};

export const createInitialState = (): ChatStoreState => ({
	messagesByID: new Map(),
	orderedMessageIDs: [],
	streamState: null,
	chatStatus: null,
	streamError: null,
	retryState: null,
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

	const setState = (
		updater: (current: ChatStoreState) => ChatStoreState,
	): void => {
		const next = updater(state);
		if (next === state) {
			return;
		}
		state = next;
		emit();
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

	const applyMessageParts = (parts: readonly Record<string, unknown>[]) => {
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
		replaceMessages,
		upsertDurableMessage,
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
		setStreamError: (reason) => {
			if (state.streamError === reason) {
				return;
			}
			setState((current) => ({
				...current,
				streamError: reason,
			}));
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
			if (state.retryState === retryState) {
				return;
			}
			setState((current) => ({
				...current,
				retryState,
			}));
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
		clearStreamState: () => {
			if (state.streamState === null) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
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
				state.subagentStatusOverrides.size === 0
			) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
				streamError: null,
				retryState: null,
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
