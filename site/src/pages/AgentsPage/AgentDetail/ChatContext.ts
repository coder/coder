import { watchChat } from "api/api";
import { chatKey, updateInfiniteChatsCache } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { asRecord, asString } from "components/ai-elements/runtimeTypeUtils";
import {
	type ChatStore,
	type ChatStoreState,
	type ChatStreamEvent,
	createChatStore,
	isValidChatStatus,
} from "modules/chat-shared";
import {
	startTransition,
	useCallback,
	useEffect,
	useRef,
	useSyncExternalStore,
} from "react";
import { useQueryClient } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "utils/reconnectingWebSocket";

const isChatStreamEvent = (
	data: unknown,
): data is ChatStreamEvent & Record<string, unknown> =>
	typeof data === "object" &&
	data !== null &&
	"type" in data &&
	typeof (data as Record<string, unknown>).type === "string";

const isChatStreamEventArray = (
	data: unknown,
): data is (ChatStreamEvent & Record<string, unknown>)[] =>
	Array.isArray(data) && data.every(isChatStreamEvent);

const toChatStreamEvents = (
	data: unknown,
): (ChatStreamEvent & Record<string, unknown>)[] => {
	if (isChatStreamEvent(data)) {
		return [data];
	}
	if (isChatStreamEventArray(data)) {
		return data;
	}
	return [];
};

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

interface UseChatStoreOptions {
	chatID: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: TypesGen.Chat | undefined;
	chatData: TypesGen.ChatWithMessages | undefined;
	chatQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
	setChatErrorReason: (chatID: string, reason: string) => void;
	clearChatErrorReason: (chatID: string) => void;
}

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
		wsQueueUpdateReceivedRef.current = false;
		store.setQueuedMessages([]);
		if (!chatID) {
			return;
		}
	}, [chatID, store]);

	useEffect(() => {
		if (!chatID || !chatData) {
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
	}, [chatData, chatID, chatQueuedMessages, store]);

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
						setChatErrorReason(chatID, reason);
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

export {
	selectChatStatus,
	selectHasStreamState,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
} from "modules/chat-shared";
