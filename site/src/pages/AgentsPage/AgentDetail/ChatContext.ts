import { watchChat } from "api/api";
import { chatMessagesKey, updateInfiniteChatsCache } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { asRecord, asString } from "components/ai-elements/runtimeTypeUtils";
import {
	startTransition,
	useCallback,
	useEffect,
	useRef,
	useSyncExternalStore,
} from "react";
import { type InfiniteData, useQueryClient } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "utils/reconnectingWebSocket";
import type { ChatDetailError } from "../usageLimitMessage";
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

interface UseChatStoreOptions {
	chatID: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: TypesGen.Chat | undefined;
	chatMessagesData: TypesGen.ChatMessagesResponse | undefined;
	chatQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
	setChatErrorReason: (chatID: string, reason: ChatDetailError) => void;
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
		chatMessagesData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	} = options;

	const queryClient = useQueryClient();
	const storeRef = useRef<ChatStore>(createChatStore());
	const streamResetFrameRef = useRef<number | null>(null);
	const queuedMessagesHydratedChatIDRef = useRef<string | null>(null);
	// Tracks whether the WebSocket has delivered a queue_update for the
	// current chat. When true, the stream is the authoritative source
	// and REST re-fetches must not overwrite the store. When false,
	// REST data is allowed to re-hydrate so stale cached queued
	// messages are corrected when switching back to a chat whose
	// queue was drained while the user was away.
	const wsQueueUpdateReceivedRef = useRef(false);
	const activeChatIDRef = useRef<string | null>(null);
	const prevChatIDRef = useRef<string | undefined>(chatID);

	const store = storeRef.current;

	// Compute the last REST-fetched message ID so the stream can
	// skip messages the client already has. We use a ref so the
	// socket effect can read the latest value without including
	// chatMessages in its dependency array (which would cause
	// unnecessary reconnections).
	const lastMessageIdRef = useRef<number | undefined>(undefined);
	lastMessageIdRef.current =
		chatMessages && chatMessages.length > 0
			? chatMessages[chatMessages.length - 1].id
			: undefined;

	const updateSidebarChat = useCallback(
		(updater: (chat: TypesGen.Chat) => TypesGen.Chat) => {
			if (!chatID) {
				return;
			}
			updateInfiniteChatsCache(queryClient, (chats) => {
				let didUpdate = false;
				const nextChats = chats.map((chat) => {
					if (chat.id !== chatID) {
						return chat;
					}
					didUpdate = true;
					return updater(chat);
				});
				return didUpdate ? nextChats : chats;
			});
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
			queryClient.setQueryData<
				InfiniteData<TypesGen.ChatMessagesResponse> | undefined
			>(chatMessagesKey(chatID), (currentData) => {
				if (!currentData?.pages?.length) {
					return currentData;
				}
				const firstPage = currentData.pages[0];
				if (
					chatQueuedMessagesEqualByID(
						firstPage.queued_messages,
						nextQueuedMessages,
					)
				) {
					return currentData;
				}
				return {
					...currentData,
					pages: [
						{ ...firstPage, queued_messages: nextQueuedMessages },
						...currentData.pages.slice(1),
					],
				};
			});
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
		// Merge REST-fetched messages into the store one-by-one instead
		// of replacing the entire map. This preserves any messages the
		// WebSocket delivered via upsertDurableMessage that haven't
		// appeared in a REST page yet.
		if (chatMessages) {
			for (const message of chatMessages) {
				store.upsertDurableMessage(message);
			}
		}
	}, [chatID, chatMessages, store]);

	useEffect(() => {
		store.setChatStatus(chatRecord?.status ?? null);
	}, [chatRecord?.status, store]);

	useEffect(() => {
		queuedMessagesHydratedChatIDRef.current = null;
		wsQueueUpdateReceivedRef.current = false;
		store.setQueuedMessages([]);
		if (!chatID) {
			return;
		}
	}, [chatID, store]);

	useEffect(() => {
		if (!chatID || !chatMessagesData) {
			return;
		}
		// Allow re-hydration from REST as long as the WebSocket hasn't
		// delivered a queue_update yet (which would be fresher). This
		// ensures that when the user navigates back to a chat whose
		// queued messages were drained server-side while they were
		// away, the REST refetch corrects the stale cached state.
		if (
			queuedMessagesHydratedChatIDRef.current === chatID &&
			wsQueueUpdateReceivedRef.current
		) {
			return;
		}
		queuedMessagesHydratedChatIDRef.current = chatID;
		store.setQueuedMessages(chatQueuedMessages);
	}, [chatMessagesData, chatID, chatQueuedMessages, store]);

	useEffect(() => {
		cancelScheduledStreamReset();
		store.resetTransientState();
		activeChatIDRef.current = chatID ?? null;

		if (!chatID) {
			return;
		}

		// Capture chatID as a narrowed string for use in closures.
		const activeChatID = chatID;
		// Local disposed flag so the message handler (which lives
		// outside the utility) can bail out after cleanup.
		let disposed = false;

		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
		) => {
			if (disposed) {
				return;
			}
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
						// Keep lastMessageIdRef in sync with
						// stream-delivered messages so reconnections use
						// the correct after_id and don't re-fetch or
						// miss events.
						if (
							message.id !== undefined &&
							(lastMessageIdRef.current === undefined ||
								message.id > lastMessageIdRef.current)
						) {
							lastMessageIdRef.current = message.id;
						}
						if (changed) {
							scheduleStreamReset();
						}
						// Do not update updated_at here. The global
						// chat-list WebSocket delivers the authoritative
						// server timestamp; fabricating a client-side
						// value causes the chat to flicker between time
						// groups when the two sources race.
						continue;
					}
					case "queue_update":
						{
							const eventChatID = asString(streamEvent.chat_id);
							if (eventChatID && eventChatID !== chatID) {
								continue;
							}
						}
						wsQueueUpdateReceivedRef.current = true;
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
						}));
						continue;
					}
					case "error": {
						const eventChatID = asString(streamEvent.chat_id);
						if (eventChatID && eventChatID !== chatID) {
							continue;
						}
						const error = asRecord(streamEvent.error);
						const reason =
							asString(error?.message).trim() || "Chat processing failed.";
						store.setChatStatus("error");
						store.setStreamError(reason);
						store.clearRetryState();
						setChatErrorReason(chatID, {
							kind: "generic",
							message: reason,
						});
						updateSidebarChat((chat) => ({
							...chat,
							status: "error",
						}));
						continue;
					}
					case "retry": {
						const eventChatID = asString(streamEvent.chat_id);
						if (eventChatID && eventChatID !== chatID) {
							continue;
						}
						const retry = streamEvent.retry;
						if (retry) {
							store.clearStreamState();
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

		const disposeSocket = createReconnectingWebSocket({
			connect() {
				// Use the latest known message ID so the server only
				// sends events the client hasn't seen yet.
				const socket = watchChat(activeChatID, lastMessageIdRef.current);
				socket.addEventListener("message", handleMessage);
				return socket;
			},
			onOpen() {
				// Connection succeeded — clear any previous disconnect
				// error and stale stream state. Clearing stream state
				// is critical for reconnections: the server replays
				// all buffered message_part events, so we must start
				// from a clean slate to avoid duplicating text.
				store.clearStreamError();
				store.clearStreamState();
			},
			onDisconnect(attempt) {
				// Show the error only on the first disconnect (not
				// while we are already retrying).
				if (attempt === 0) {
					store.setStreamError("Chat stream disconnected. Reconnecting\u2026");
				}
				// Clear "running" status on disconnect so the UI
				// doesn't show a stale spinner. The reconnected
				// stream will deliver the authoritative status.
				const currentStatus = store.getSnapshot().chatStatus;
				if (currentStatus === "running") {
					store.setChatStatus(null);
				}
			},
		});

		return () => {
			disposed = true;
			disposeSocket();
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
