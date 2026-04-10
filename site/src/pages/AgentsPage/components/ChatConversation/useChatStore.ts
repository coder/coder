import { useEffect, useEffectEvent, useRef, useState } from "react";
import { type InfiniteData, useQueryClient } from "react-query";
import { watchChat } from "#/api/api";
import { chatMessagesKey, updateInfiniteChatsCache } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { asNumber, asString } from "../ChatElements/runtimeTypeUtils";
import {
	type ChatStore,
	type ChatStoreState,
	chatMessagesEqualByValue,
	chatQueuedMessagesEqualByID,
	createChatStore,
	isActiveChatStatus,
} from "./chatStore";
import type { RetryState } from "./types";

const isChatStreamEvent = (data: unknown): data is TypesGen.ChatStreamEvent =>
	typeof data === "object" &&
	data !== null &&
	"type" in data &&
	typeof (data as Record<string, unknown>).type === "string";

const isChatStreamEventArray = (
	data: unknown,
): data is TypesGen.ChatStreamEvent[] =>
	Array.isArray(data) && data.every(isChatStreamEvent);

const toChatStreamEvents = (data: unknown): TypesGen.ChatStreamEvent[] => {
	if (isChatStreamEvent(data)) {
		return [data];
	}
	if (isChatStreamEventArray(data)) {
		return data;
	}
	return [];
};

const normalizeChatDetailError = (
	error: TypesGen.ChatStreamError | Record<string, unknown> | undefined,
): ChatDetailError => ({
	message: asString(error?.message).trim() || "Chat processing failed.",
	kind: asString(error?.kind).trim() || "generic",
	provider: asString(error?.provider).trim() || undefined,
	retryable:
		typeof error?.retryable === "boolean" ? error.retryable : undefined,
	statusCode: asNumber(error?.status_code),
});

const normalizeRetryState = (retry: TypesGen.ChatStreamRetry): RetryState => {
	const delayMs = asNumber(retry.delay_ms);
	const retryingAt = asString(retry.retrying_at).trim() || undefined;
	return {
		attempt: Math.max(1, asNumber(retry.attempt) ?? 1),
		error: asString(retry.error).trim() || "Retrying request shortly.",
		kind: asString(retry.kind).trim() || "generic",
		provider: asString(retry.provider).trim() || undefined,
		...(delayMs !== undefined ? { delayMs } : {}),
		...(retryingAt ? { retryingAt } : {}),
	};
};

const shouldSurfaceReconnectState = (state: ChatStoreState): boolean =>
	state.streamError === null &&
	(state.streamState !== null ||
		state.retryState !== null ||
		isActiveChatStatus(state.chatStatus));

interface UseChatStoreOptions {
	chatID: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: TypesGen.Chat | undefined;
	chatMessagesData: TypesGen.ChatMessagesResponse | undefined;
	chatQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
	setChatErrorReason: (chatID: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatID: string) => void;
}

export const useChatStore = (
	options: UseChatStoreOptions,
): {
	store: ChatStore;
	clearStreamError: () => void;
	upsertCacheMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
} => {
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
	const [store] = useState(createChatStore);
	const queuedMessagesHydratedChatIDRef = useRef<string | null>(null);
	// Tracks whether the WebSocket has delivered a queue_update for the
	// current chat. When true, the stream is the authoritative source
	// and REST re-fetches must not overwrite the store. When false,
	// REST data is allowed to re-hydrate so stale cached queued
	// messages are corrected when switching back to a chat whose
	// queue was drained while the user was away.
	const wsQueueUpdateReceivedRef = useRef(false);
	// Tracks whether the WebSocket has delivered a status event for
	// the current chat. Once true, the WS is the authoritative
	// source for chatStatus and the REST-fetched chatRecord.status
	// must not overwrite it. Without this guard, a React Query
	// refetch (e.g. on window focus) can regress chatStatus to a
	// stale value like "pending", causing shouldApplyMessagePart()
	// to drop all incoming parts.
	const wsStatusReceivedRef = useRef(false);
	const activeChatIDRef = useRef<string | null>(null);
	const prevChatIDRef = useRef<string | undefined>(chatID);
	// Snapshot of the chatMessages elements from the last sync effect
	// run. Used to detect whether chatMessages actually changed (e.g.
	// after a refetch producing new objects) vs. just getting a new
	// array reference because an unrelated field like queued_messages
	// was updated in the query cache. Element-level reference
	// comparison works because the flattening step preserves message
	// object references when only non-message fields change in the
	// page, while a genuine refetch returns new objects from the
	// server.
	const lastSyncedMessagesRef = useRef<readonly TypesGen.ChatMessage[]>([]);

	// Compute the last REST-fetched message ID so the stream can
	// skip messages the client already has. We use a ref so the
	// socket effect can read the latest value without including
	// chatMessages in its dependency array (which would cause
	// unnecessary reconnections).
	const lastMessageIdRef = useRef<number | undefined>(undefined);
	useEffect(() => {
		lastMessageIdRef.current =
			chatMessages && chatMessages.length > 0
				? chatMessages[chatMessages.length - 1].id
				: undefined;
	});

	// Wrap error-reason callbacks so the WebSocket effect can call
	// them without including them in its dependency array.
	const setChatErrorReasonEvent = useEffectEvent(setChatErrorReason);
	const clearChatErrorReasonEvent = useEffectEvent(clearChatErrorReason);

	// True once the initial REST page has resolved for the current
	// chat. The WebSocket effect gates on this so that
	// lastMessageIdRef is populated before the socket opens;
	// otherwise the server replays the entire message history as
	// its snapshot, defeating pagination.
	const initialDataLoaded = chatMessages !== undefined;

	// Write WebSocket-delivered durable messages into the React
	// Query infinite cache so that navigating away and back
	// serves up-to-date data instead of the stale REST snapshot.
	// Without this, the cache only contains messages from the
	// last REST fetch, and structural sharing can suppress the
	// refetch-driven store update when no new durable messages
	// have been committed to the DB yet.
	const upsertCacheMessages = useEffectEvent(
		(messages: readonly TypesGen.ChatMessage[]) => {
			if (!chatID || messages.length === 0) {
				return;
			}
			queryClient.setQueryData<
				InfiniteData<TypesGen.ChatMessagesResponse> | undefined
			>(chatMessagesKey(chatID), (currentData) => {
				if (!currentData?.pages?.length) {
					return currentData;
				}
				const firstPage = currentData.pages[0];
				const existingByID = new Map(firstPage.messages.map((m) => [m.id, m]));

				let changed = false;
				for (const msg of messages) {
					const existing = existingByID.get(msg.id);
					if (!existing || !chatMessagesEqualByValue(existing, msg)) {
						changed = true;
						existingByID.set(msg.id, msg);
					}
				}

				if (!changed) {
					return currentData;
				}

				// Sort descending to match the API page order
				// (newest first).
				const updatedMessages = Array.from(existingByID.values());
				updatedMessages.sort((a, b) => b.id - a.id);

				return {
					...currentData,
					pages: [
						{ ...firstPage, messages: updatedMessages },
						...currentData.pages.slice(1),
					],
				};
			});
		},
	);

	useEffect(() => {
		store.batch(() => {
			// When the active chat changes, clear stale messages
			// immediately so the previous chat's messages aren't
			// briefly visible while the new chat's query resolves.
			if (prevChatIDRef.current !== chatID) {
				prevChatIDRef.current = chatID;
				lastSyncedMessagesRef.current = [];
				store.replaceMessages([]);
			}
			// Merge REST-fetched messages into the store, preserving
			// any messages the WebSocket delivered that haven't
			// appeared in a REST page yet.
			//
			// If the fetched set is missing message IDs the store
			// already has (e.g. after an edit truncation), a full
			// replace is needed. We must only do this when the
			// fetched messages actually changed (new elements from
			// a refetch), not when an unrelated field like
			// queued_messages caused the query data reference to
			// update.
			if (chatMessages) {
				const prev = lastSyncedMessagesRef.current;
				const contentChanged =
					chatMessages.length !== prev.length ||
					chatMessages.some((m, i) => m !== prev[i]);
				lastSyncedMessagesRef.current = chatMessages;

				const storeSnap = store.getSnapshot();
				const fetchedIDs = new Set(chatMessages.map((m) => m.id));
				// Only classify a store-held ID as stale if it was
				// present in the PREVIOUS sync's fetched data. IDs
				// added to the store after the last sync (by the WS
				// handler or handleSend) are new, not stale, and
				// must not trigger the destructive replaceMessages
				// path.
				const prevIDs = new Set(prev.map((m) => m.id));
				const hasStaleEntries =
					contentChanged &&
					storeSnap.orderedMessageIDs.some(
						(id) => !fetchedIDs.has(id) && prevIDs.has(id),
					);
				if (hasStaleEntries) {
					store.replaceMessages(chatMessages);
				} else {
					store.upsertDurableMessages(chatMessages);
				}
			}
		});
	}, [chatID, chatMessages, store]);

	useEffect(() => {
		// Only hydrate from REST when the WebSocket hasn't delivered
		// a status event yet. Once the WS is the authoritative
		// source, a stale REST refetch must not overwrite the
		// fresher WS-delivered value.
		if (!wsStatusReceivedRef.current) {
			store.setChatStatus(chatRecord?.status ?? null);
		}
	}, [chatRecord?.status, store]);

	useEffect(() => {
		queuedMessagesHydratedChatIDRef.current = null;
		wsQueueUpdateReceivedRef.current = false;
		wsStatusReceivedRef.current = false;
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

		const updateChatQueuedMessages = (
			queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
		) => {
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
		};

		store.resetTransientState();
		activeChatIDRef.current = chatID ?? null;

		if (!chatID || !initialDataLoaded) {
			return;
		}

		// Capture chatID as a narrowed string for use in closures.
		const activeChatID = chatID;
		// Local disposed flag so the message handler (which lives
		// outside the utility) can bail out after cleanup.
		let disposed = false;

		// Parts buffer lives at the effect scope so it persists
		// across WebSocket messages. A rAF-based flush coalesces
		// parts from multiple WS messages into a single render,
		// capping stream renders to once per animation frame.
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

		// Immediate flush for non-message_part events that need
		// the parts applied before they execute (e.g. a durable
		// message commit right after the last part).
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

		// Discard buffered parts without applying them. Used when
		// stream state is about to be cleared (pending, waiting,
		// retry) — flushing would re-populate the state that the
		// event is about to clear.
		const discardBufferedParts = () => {
			partsBuf.length = 0;
			if (partsFlushTimer !== null) {
				clearTimeout(partsFlushTimer);
				partsFlushTimer = null;
			}
		};

		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
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
			if (payload.parsedMessage.type !== "data") {
				return;
			}

			const streamEvents = toChatStreamEvents(payload.parsedMessage.data);
			if (streamEvents.length === 0) {
				return;
			}
			// Collect durable messages for bulk upsert so the
			// entire batch produces one Map copy + one sort
			// instead of N copies and N sorts.
			const pendingMessages: TypesGen.ChatMessage[] = [];
			let needsStreamReset = false;

			// Wrap all store mutations in a batch so subscribers
			// are notified exactly once at the end, not per event.
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

					// Only flush buffered parts before events that
					// need them applied first. `message` events
					// commit durable state that must include all
					// stream parts. `error` events should surface
					// partial output. Other events (status, retry,
					// queue_update) must NOT flush — status changes
					// need to be visible before parts so the
					// "Thinking..." indicator can render, and retry
					// clears stream state which a flush would
					// re-populate.
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
							wsQueueUpdateReceivedRef.current = true;
							store.setQueuedMessages(streamEvent.queued_messages);
							updateChatQueuedMessages(streamEvent.queued_messages);
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

							wsStatusReceivedRef.current = true;
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

				// Schedule a coalesced flush for any remaining
				// parts. If parts were already flushed by a
				// non-message_part event above, this is a no-op.
				schedulePartsFlush();

				// Bulk-upsert all collected durable messages in one
				// pass: one Map copy + one sort instead of N each.
				if (pendingMessages.length > 0) {
					store.upsertDurableMessages(pendingMessages);
					upsertCacheMessages(pendingMessages);
				}

				// Clear stream state atomically with the durable
				// message commit so subscribers never see a
				// snapshot where both the committed message and
				// the streaming output coexist. Previously this
				// was deferred to a requestAnimationFrame, which
				// left a window where ConversationTimeline and
				// LiveStreamTail rendered the same content.
				if (needsStreamReset) {
					store.clearStreamState();
					// If more message_part events arrived in this
					// batch after the durable message, they belong
					// to the next turn. Apply them immediately so
					// the stream transitions from the old turn to
					// the new one without a flash.
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
				// Connection succeeded. Before the socket replays any
				// buffered message_part events, drop transport-scoped
				// state from the previous socket attempt so stale
				// partial output or failures do not leak into the new
				// stream.
				store.resetTransportReplayState();
				// Drain any message parts buffered from the
				// previous socket. Without this, a pending
				// flush timer could fire after reconnect and
				// apply stale parts from the old connection
				// into the fresh stream state.
				discardBufferedParts();
			},
			onDisconnect(
				reconnectState: import("#/utils/reconnectingWebSocket").ReconnectSchedule,
			) {
				// Only surface reconnecting when the disconnect
				// interrupted active response work. Idle watcher
				// reconnects stay silent.
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
	}, [chatID, initialDataLoaded, queryClient, store]);
	return {
		store,
		clearStreamError: () => {
			store.clearStreamError();
		},
		upsertCacheMessages,
	};
};
