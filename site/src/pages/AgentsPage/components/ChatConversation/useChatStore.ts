import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useQueryClient } from "react-query";
import { watchChat } from "#/api/api";
import { updateInfiniteChatsCache } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import {
	type ChatStore,
	type ChatStoreState,
	createChatStore,
	isActiveChatStatus,
} from "./chatStore";
import type { RetryState } from "./types";

const normalizeChatDetailError = (
	error: TypesGen.ChatStreamError | undefined,
): ChatDetailError => ({
	message: error?.message.trim() || "Chat processing failed.",
	kind: error?.kind?.trim() || "generic",
	provider: error?.provider?.trim() || undefined,
	retryable: error?.retryable,
	statusCode: error?.status_code,
});

const normalizeRetryState = (retry: TypesGen.ChatStreamRetry): RetryState => ({
	attempt: Math.max(1, retry.attempt),
	error: retry.error.trim() || "Retrying request shortly.",
	kind: retry.kind?.trim() || "generic",
	provider: retry.provider?.trim() || undefined,
	delayMs: retry.delay_ms,
	retryingAt: retry.retrying_at.trim() || undefined,
});

const shouldSurfaceReconnectState = (state: ChatStoreState): boolean =>
	state.streamError === null &&
	(state.streamState !== null ||
		state.retryState !== null ||
		isActiveChatStatus(state.chatStatus));

interface UseChatStoreOptions {
	chatID: string | undefined;
	chatRecord: TypesGen.Chat | undefined;
	setChatErrorReason: (chatID: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatID: string) => void;
}

export const useChatStore = (
	options: UseChatStoreOptions,
): {
	store: ChatStore;
	clearStreamError: () => void;
	/** True once the WS has delivered its initial snapshot. */
	wsReady: boolean;
} => {
	const { chatID, chatRecord, setChatErrorReason, clearChatErrorReason } =
		options;

	const queryClient = useQueryClient();
	const [store] = useState(createChatStore);
	const activeChatIDRef = useRef<string | null>(null);
	const prevChatIDRef = useRef<string | undefined>(chatID);
	const lastMessageIdRef = useRef<number | undefined>(undefined);
	const [wsReady, setWsReady] = useState(false);

	// Wrap error-reason callbacks so the WebSocket effect can call
	// them without including them in its dependency array.
	const setChatErrorReasonEvent = useEffectEvent(setChatErrorReason);
	const clearChatErrorReasonEvent = useEffectEvent(clearChatErrorReason);

	// Hydrate status from the REST chat record only before the WS
	// delivers its own status event. The WS status event in the
	// snapshot will overwrite this immediately.
	useEffect(() => {
		if (chatRecord?.status) {
			store.setChatStatus(chatRecord.status);
		}
	}, [chatRecord?.status, store]);

	// Clear stale messages on chat switch.
	useEffect(() => {
		if (prevChatIDRef.current !== chatID) {
			prevChatIDRef.current = chatID;
			store.replaceMessages([]);
			store.setQueuedMessages([]);
			setWsReady(false);
		}
	}, [chatID, store]);

	useEffect(() => {
		const updateSidebarChat = (
			updater: (chat: TypesGen.Chat) => TypesGen.Chat,
		) => {
			if (!chatID) {
				return;
			}
			updateInfiniteChatsCache(queryClient, (chats) => {
				let didUpdate = false;
				const nextChats = chats.map((chat) => {
					if (chat.id !== chatID) {
						return chat;
					}
					const updated = updater(chat);
					if (updated !== chat) {
						didUpdate = true;
					}
					return updated;
				});
				return didUpdate ? nextChats : chats;
			});
		};

		store.resetTransientState();
		activeChatIDRef.current = chatID ?? null;

		if (!chatID) {
			return;
		}

		// Capture chatID as a narrowed string for use in closures.
		const activeChatID = chatID;
		let disposed = false;

		// Parts buffer lives at the effect scope so it persists
		// across WebSocket messages.
		const partsBuf: TypesGen.ChatMessagePart[] = [];
		let partsFlushTimer: ReturnType<typeof setTimeout> | null = null;

		const shouldApplyMessagePart = (): boolean => {
			const currentStatus = store.getSnapshot().chatStatus;
			return currentStatus !== "pending" && currentStatus !== "waiting";
		};

		const schedulePartsFlush = () => {
			if (partsFlushTimer !== null || partsBuf.length === 0) {
				return;
			}
			partsFlushTimer = setTimeout(() => {
				partsFlushTimer = null;
				if (disposed || activeChatIDRef.current !== chatID) {
					return;
				}
				const parts = partsBuf.splice(0);
				if (parts.length === 0 || !shouldApplyMessagePart()) {
					return;
				}
				store.applyMessageParts(parts);
			}, 0);
		};

		const flushMessageParts = () => {
			if (partsBuf.length === 0) {
				return;
			}
			if (partsFlushTimer !== null) {
				clearTimeout(partsFlushTimer);
				partsFlushTimer = null;
			}
			const parts = partsBuf.splice(0);
			if (activeChatIDRef.current !== chatID || !shouldApplyMessagePart()) {
				return;
			}
			store.applyMessageParts(parts);
		};

		const discardBufferedParts = () => {
			partsBuf.length = 0;
			if (partsFlushTimer !== null) {
				clearTimeout(partsFlushTimer);
				partsFlushTimer = null;
			}
		};

		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ChatStreamEvent[]>,
		) => {
			if (disposed) {
				return;
			}
			if (payload.parseError || !payload.parsedMessage) {
				store.setStreamError({
					kind: "generic",
					message: "Failed to parse chat stream update.",
				});
				return;
			}

			const streamEvents = payload.parsedMessage;
			if (streamEvents.length === 0) {
				return;
			}

			const pendingMessages: TypesGen.ChatMessage[] = [];
			let needsStreamReset = false;

			store.batch(() => {
				for (const streamEvent of streamEvents) {
					if (streamEvent.type === "message_part") {
						if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
							continue;
						}
						if (!shouldApplyMessagePart()) {
							continue;
						}
						const part = streamEvent.message_part?.part;
						if (part) {
							store.clearRetryState();
							partsBuf.push(part);
						}
						continue;
					}

					if (streamEvent.type === "message" || streamEvent.type === "error") {
						flushMessageParts();
					}

					switch (streamEvent.type) {
						case "message": {
							const message = streamEvent.message;
							if (!message) {
								continue;
							}
							if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
								continue;
							}
							store.clearRetryState();
							pendingMessages.push(message);
							if (
								message.id !== undefined &&
								(lastMessageIdRef.current === undefined ||
									message.id > lastMessageIdRef.current)
							) {
								lastMessageIdRef.current = message.id;
							}
							if (message.role === "assistant") {
								needsStreamReset = true;
							}
							continue;
						}
						case "queue_update":
							if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
								continue;
							}
							store.setQueuedMessages(streamEvent.queued_messages);
							continue;
						case "status": {
							const nextStatus = streamEvent.status?.status;
							if (!nextStatus) {
								continue;
							}

							if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
								store.setSubagentStatusOverride(
									streamEvent.chat_id,
									nextStatus,
								);
								continue;
							}

							store.clearRetryState();
							store.setChatStatus(nextStatus);
							if (nextStatus === "pending" || nextStatus === "waiting") {
								discardBufferedParts();
								store.clearStreamState();
								store.clearRetryState();
							}
							if (nextStatus === "running") {
								store.clearRetryState();
							}
							if (nextStatus !== "error") {
								clearChatErrorReasonEvent(chatID);
							}
							updateSidebarChat((chat) =>
								chat.status === nextStatus
									? chat
									: { ...chat, status: nextStatus },
							);
							continue;
						}
						case "error": {
							if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
								continue;
							}
							const reason = normalizeChatDetailError(streamEvent.error);
							store.setChatStatus("error");
							store.setStreamError(reason);
							store.clearRetryState();
							setChatErrorReasonEvent(chatID, reason);
							updateSidebarChat((chat) =>
								chat.status === "error" ? chat : { ...chat, status: "error" },
							);
							continue;
						}
						case "retry": {
							if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
								continue;
							}
							const retry = streamEvent.retry;
							if (retry) {
								discardBufferedParts();
								store.clearStreamState();
								store.setRetryState(normalizeRetryState(retry));
							}
							continue;
						}
						default:
							continue;
					}
				}

				schedulePartsFlush();

				if (pendingMessages.length > 0) {
					store.upsertDurableMessages(pendingMessages);
				}

				if (needsStreamReset) {
					store.clearStreamState();
					if (partsBuf.length > 0) {
						if (partsFlushTimer !== null) {
							clearTimeout(partsFlushTimer);
							partsFlushTimer = null;
						}
						const nextParts = partsBuf.splice(0);
						if (shouldApplyMessagePart()) {
							store.applyMessageParts(nextParts);
						}
					}
				}
			});

			// Signal ready after the first batch is processed.
			// This replaces the old isLoading gate from the REST
			// infinite query.
			setWsReady(true);
		};

		const disposeSocket = createReconnectingWebSocket({
			connect() {
				// Connect with after_id from the last known message
				// so reconnects only get the delta. On first connect
				// lastMessageIdRef is undefined → server sends all.
				const socket = watchChat(activeChatID, lastMessageIdRef.current);
				socket.addEventListener("message", handleMessage);
				return socket;
			},
			onOpen() {
				store.resetTransportReplayState();
				discardBufferedParts();
			},
			onDisconnect(
				reconnectState: import("#/utils/reconnectingWebSocket").ReconnectSchedule,
			) {
				const snapshot = store.getSnapshot();
				if (shouldSurfaceReconnectState(snapshot)) {
					store.setReconnectState(reconnectState);
				}
			},
		});

		return () => {
			disposed = true;
			disposeSocket();
			if (partsFlushTimer !== null) {
				clearTimeout(partsFlushTimer);
			}
			activeChatIDRef.current = null;
		};
	}, [chatID, queryClient, store]);

	return {
		store,
		clearStreamError: () => {
			store.clearStreamError();
		},
		wsReady,
	};
};
