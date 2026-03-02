import { watchChat } from "api/api";
import { chatKey, chatsKey } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { asRecord, asString } from "components/ai-elements/runtimeTypeUtils";
import {
	startTransition,
	useCallback,
	useEffect,
	useRef,
	useSyncExternalStore,
} from "react";
import { useQueryClient } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { applyMessagePartToStreamState } from "./streamState";
import type { StreamState } from "./types";

const VALID_CHAT_STATUSES: ReadonlySet<string> = new Set<TypesGen.ChatStatus>([
	"pending",
	"running",
	"completed",
	"error",
	"paused",
	"waiting",
]);

const isValidChatStatus = (value: unknown): value is TypesGen.ChatStatus =>
	typeof value === "string" && VALID_CHAT_STATUSES.has(value);

const isChatStreamEvent = (
	data: unknown,
): data is TypesGen.ChatStreamEvent & Record<string, unknown> =>
	typeof data === "object" &&
	data !== null &&
	"type" in data &&
	typeof (data as Record<string, unknown>).type === "string";

const isChatStreamEventArray = (
	data: unknown,
): data is (TypesGen.ChatStreamEvent & Record<string, unknown>)[] =>
	Array.isArray(data) && data.every(isChatStreamEvent);

const toChatStreamEvents = (
	data: unknown,
): (TypesGen.ChatStreamEvent & Record<string, unknown>)[] => {
	if (isChatStreamEvent(data)) {
		return [data];
	}
	if (isChatStreamEventArray(data)) {
		return data;
	}
	return [];
};

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

type ChatStoreState = {
	messagesByID: Map<number, TypesGen.ChatMessage>;
	orderedMessageIDs: readonly number[];
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	streamError: string | null;
	retryState: { attempt: number; error: string } | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
};

type ChatStore = {
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

const createInitialState = (): ChatStoreState => ({
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

		if (
			mapsEqualByRef(state.messagesByID, nextMessagesByID) &&
			arraysEqual(state.orderedMessageIDs, nextOrderedMessageIDs)
		) {
			return;
		}

		setState((current) => ({
			...current,
			messagesByID: nextMessagesByID,
			orderedMessageIDs: nextOrderedMessageIDs,
		}));
	};

	const upsertDurableMessage = (message: TypesGen.ChatMessage) => {
		const existing = state.messagesByID.get(message.id);
		const isDuplicate = state.messagesByID.has(message.id);
		if (existing && chatMessagesEqualByValue(existing, message)) {
			return { isDuplicate, changed: false };
		}

		const nextMessagesByID = new Map(state.messagesByID);
		nextMessagesByID.set(message.id, message);

		// When a real server message (positive ID) arrives, remove any
		// optimistic placeholder (negative ID) for the same role so the
		// user doesn't momentarily see the message twice.
		if (message.id > 0) {
			for (const [id, existing] of nextMessagesByID) {
				if (id < 0 && existing.role === message.role) {
					nextMessagesByID.delete(id);
				}
			}
		}

		const needsReorder =
			!isDuplicate || nextMessagesByID.size !== state.messagesByID.size;
		const nextOrderedMessageIDs = needsReorder
			? buildOrderedMessageIDs(Array.from(nextMessagesByID.values()))
			: state.orderedMessageIDs;

		setState((current) => ({
			...current,
			messagesByID: nextMessagesByID,
			orderedMessageIDs: nextOrderedMessageIDs,
		}));
		return { isDuplicate, changed: true };
	};

	const applyMessageParts = (parts: readonly Record<string, unknown>[]) => {
		if (parts.length === 0) {
			return;
		}

		let nextStreamState: StreamState | null = state.streamState;
		for (const part of parts) {
			nextStreamState = applyMessagePartToStreamState(nextStreamState, part);
		}
		if (nextStreamState === state.streamState) {
			return;
		}
		setState((current) => ({
			...current,
			streamState: nextStreamState,
		}));
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
			if (
				chatQueuedMessagesEqualByID(state.queuedMessages, nextQueuedMessages)
			) {
				return;
			}
			setState((current) => ({
				...current,
				queuedMessages: nextQueuedMessages,
			}));
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
			const nextOverrides = new Map(state.subagentStatusOverrides);
			nextOverrides.set(chatID, status);
			setState((current) => ({
				...current,
				subagentStatusOverrides: nextOverrides,
			}));
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

interface UseChatStoreOptions {
	chatID: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: TypesGen.Chat | undefined;
	chatData: TypesGen.ChatWithMessages | undefined;
	chatQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
	setChatErrorReason: (chatID: string, reason: string) => void;
	clearChatErrorReason: (chatID: string) => void;
}

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

export const useChatStore = (
	options: UseChatStoreOptions,
): { store: ChatStore; clearStreamError: () => void } => {
	const {
		chatID,
		chatMessages,
		chatRecord,
		chatData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	} = options;

	const queryClient = useQueryClient();
	const storeRef = useRef<ChatStore>(createChatStore());
	const streamResetFrameRef = useRef<number | null>(null);
	const queuedMessagesHydratedChatIDRef = useRef<string | null>(null);
	const activeChatIDRef = useRef<string | null>(null);
	const prevChatIDRef = useRef<string | undefined>(chatID);

	const store = storeRef.current;

	const updateSidebarChat = useCallback(
		(updater: (chat: TypesGen.Chat) => TypesGen.Chat) => {
			if (!chatID) {
				return;
			}
			queryClient.setQueryData<readonly TypesGen.Chat[] | undefined>(
				chatsKey,
				(currentChats) => {
					if (!currentChats) {
						return currentChats;
					}
					let didUpdate = false;
					const nextChats = currentChats.map((chat) => {
						if (chat.id !== chatID) {
							return chat;
						}
						didUpdate = true;
						return updater(chat);
					});
					return didUpdate ? nextChats : currentChats;
				},
			);
		},
		[chatID, queryClient],
	);

	const cancelScheduledStreamReset = useCallback(() => {
		if (streamResetFrameRef.current === null) {
			return;
		}
		window.cancelAnimationFrame(streamResetFrameRef.current);
		streamResetFrameRef.current = null;
	}, []);

	const scheduleStreamReset = useCallback(() => {
		cancelScheduledStreamReset();
		streamResetFrameRef.current = window.requestAnimationFrame(() => {
			store.clearStreamState();
			streamResetFrameRef.current = null;
		});
	}, [cancelScheduledStreamReset, store]);

	const updateChatQueuedMessages = useCallback(
		(queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined) => {
			if (!chatID) {
				return;
			}
			const nextQueuedMessages = queuedMessages ?? [];
			queryClient.setQueryData<TypesGen.ChatWithMessages | undefined>(
				chatKey(chatID),
				(currentChat) => {
					if (!currentChat) {
						return currentChat;
					}
					if (
						chatQueuedMessagesEqualByID(
							currentChat.queued_messages,
							nextQueuedMessages,
						)
					) {
						return currentChat;
					}
					return {
						...currentChat,
						queued_messages: nextQueuedMessages,
					};
				},
			);
		},
		[chatID, queryClient],
	);

	useEffect(() => {
		// When the active chat changes, clear stale messages immediately
		// so the previous chat's messages aren't briefly visible while
		// the new chat's query resolves.
		if (prevChatIDRef.current !== chatID) {
			prevChatIDRef.current = chatID;
			store.replaceMessages([]);
		}
		store.replaceMessages(chatMessages);
	}, [chatID, chatMessages, store]);

	useEffect(() => {
		store.setChatStatus(chatRecord?.status ?? null);
	}, [chatRecord?.status, store]);

	useEffect(() => {
		queuedMessagesHydratedChatIDRef.current = null;
		store.setQueuedMessages([]);
		if (!chatID) {
			return;
		}
	}, [chatID, store]);

	useEffect(() => {
		if (!chatID || !chatData) {
			return;
		}
		if (queuedMessagesHydratedChatIDRef.current === chatID) {
			return;
		}
		queuedMessagesHydratedChatIDRef.current = chatID;
		store.setQueuedMessages(chatQueuedMessages);
	}, [chatData, chatID, chatQueuedMessages, store]);

	useEffect(() => {
		cancelScheduledStreamReset();
		store.resetTransientState();
		activeChatIDRef.current = chatID ?? null;

		if (!chatID) {
			return;
		}

		const socket = watchChat(chatID);
		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
		) => {
			if (payload.parseError || !payload.parsedMessage) {
				store.setStreamError("Failed to parse chat stream update.");
				return;
			}
			if (payload.parsedMessage.type !== "data") {
				return;
			}

			const streamEvents = toChatStreamEvents(payload.parsedMessage.data);
			if (streamEvents.length === 0) {
				return;
			}

			const shouldApplyMessagePart = (): boolean => {
				const currentStatus = store.getSnapshot().chatStatus;
				return currentStatus !== "pending" && currentStatus !== "waiting";
			};

			const pendingMessageParts: Record<string, unknown>[] = [];
			const flushMessageParts = () => {
				if (pendingMessageParts.length === 0) {
					return;
				}
				cancelScheduledStreamReset();
				const parts = pendingMessageParts.splice(0, pendingMessageParts.length);
				const currentChatID = chatID;
				startTransition(() => {
					if (activeChatIDRef.current !== currentChatID) {
						return;
					}
					// Re-check status at execution time. A status
					// event processed between scheduling and running
					// this callback may have cleared stream state.
					if (!shouldApplyMessagePart()) {
						return;
					}
					store.applyMessageParts(parts);
				});
			};

			for (const streamEvent of streamEvents) {
				if (streamEvent.type === "message_part") {
					const eventChatID = asString(streamEvent.chat_id);
					if (eventChatID && eventChatID !== chatID) {
						continue;
					}
					if (!shouldApplyMessagePart()) {
						continue;
					}
					const part = asRecord(streamEvent.message_part?.part);
					if (part) {
						cancelScheduledStreamReset();
						pendingMessageParts.push(part);
					}
					continue;
				}
				flushMessageParts();

				switch (streamEvent.type) {
					case "message": {
						const message = streamEvent.message;
						if (!message) {
							continue;
						}
						const eventChatID = asString(streamEvent.chat_id);
						if (eventChatID && eventChatID !== chatID) {
							continue;
						}
						const { changed } = store.upsertDurableMessage(message);
						if (changed) {
							scheduleStreamReset();
						}
						updateSidebarChat((chat) => ({
							...chat,
							updated_at: message.created_at ?? new Date().toISOString(),
						}));
						continue;
					}
					case "queue_update":
						{
							const eventChatID = asString(streamEvent.chat_id);
							if (eventChatID && eventChatID !== chatID) {
								continue;
							}
						}
						store.setQueuedMessages(streamEvent.queued_messages);
						updateChatQueuedMessages(streamEvent.queued_messages);
						continue;
					case "status": {
						const status = asRecord(streamEvent.status);
						const nextStatus = asString(status?.status);
						if (!isValidChatStatus(nextStatus)) {
							continue;
						}

						const eventChatID = asString(streamEvent.chat_id);
						if (eventChatID && eventChatID !== chatID) {
							store.setSubagentStatusOverride(eventChatID, nextStatus);
							continue;
						}

						store.setChatStatus(nextStatus);
						if (nextStatus === "pending" || nextStatus === "waiting") {
							store.clearStreamState();
							store.clearRetryState();
						}
						if (nextStatus === "running") {
							store.clearRetryState();
						}
						if (nextStatus !== "error") {
							clearChatErrorReason(chatID);
						}
						updateSidebarChat((chat) => ({
							...chat,
							status: nextStatus,
							updated_at: new Date().toISOString(),
						}));

						continue;
					}
					case "error": {
						const error = asRecord(streamEvent.error);
						const reason =
							asString(error?.message).trim() || "Chat processing failed.";
						store.setChatStatus("error");
						store.setStreamError(reason);
						store.clearRetryState();
						setChatErrorReason(chatID, reason);
						updateSidebarChat((chat) => ({
							...chat,
							status: "error",
							updated_at: new Date().toISOString(),
						}));
						continue;
					}
					case "retry": {
						const retry = streamEvent.retry;
						if (retry) {
							store.setRetryState({
								attempt: retry.attempt,
								error: retry.error,
							});
						}
						continue;
					}
					default:
						continue;
				}
			}
			flushMessageParts();
		};

		const handleError = () => {
			if (!store.getSnapshot().streamError) {
				store.setStreamError("Chat stream disconnected.");
			}
		};

		socket.addEventListener("message", handleMessage);
		socket.addEventListener("error", handleError);

		return () => {
			socket.removeEventListener("message", handleMessage);
			socket.removeEventListener("error", handleError);
			socket.close();
			cancelScheduledStreamReset();
			activeChatIDRef.current = null;
		};
	}, [
		cancelScheduledStreamReset,
		chatID,
		clearChatErrorReason,
		scheduleStreamReset,
		setChatErrorReason,
		store,
		updateChatQueuedMessages,
		updateSidebarChat,
	]);

	return {
		store,
		clearStreamError: useCallback(() => {
			store.clearStreamError();
		}, [store]),
	};
};

export const useChatSelector = <T>(
	store: ChatStore,
	selector: (state: ChatStoreState) => T,
): T => {
	const getSnapshot = useCallback(
		() => selector(store.getSnapshot()),
		[selector, store],
	);
	return useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
};
