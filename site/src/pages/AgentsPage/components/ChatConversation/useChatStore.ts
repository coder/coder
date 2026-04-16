import { useEffect, useEffectEvent, useRef, useState } from "react";
import { type InfiniteData, useQueryClient } from "react-query";
import { watchChat } from "#/api/api";
import { chatMessagesKey, updateInfiniteChatsCache } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import {
	type ChatStore,
	type ChatStoreState,
	chatQueuedMessagesEqualByID,
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
	const activeChatIDRef = useRef<string | null>(null);
	const prevChatIDRef = useRef<string | undefined>(chatID);
	// Tracks whether the WebSocket is connected and authoritative.
	// Once true, REST data changes are ignored by the store — the
	// WS owns all mutable chat state. Reset to false on chat
	// switch and WS teardown.
	const wsConnectedRef = useRef(false);

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

	// Snapshot the store's messages into the React Query cache.
	// Called once on WS teardown so navigating away and back
	// shows up-to-date data without a continuous writeback loop.
	const snapshotStoreToCache = useEffectEvent((snapshotChatID: string) => {
		const snap = store.getSnapshot();
		const msgs = Array.from(snap.messagesByID.values());
		if (msgs.length === 0) {
			return;
		}
		const sorted = [...msgs].sort((a, b) => b.id - a.id);
		queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatMessagesKey(snapshotChatID), (old) => ({
			pages: [
				{
					messages: sorted,
					has_more: old?.pages?.at(-1)?.has_more ?? true,
					queued_messages: [],
				},
			],
			pageParams: [undefined],
		}));
	});

	useEffect(() => {
		store.batch(() => {
			// When the active chat changes, clear stale messages
			// immediately so the previous chat's messages aren't
			// briefly visible while the new chat's query resolves.
			if (prevChatIDRef.current !== chatID) {
				prevChatIDRef.current = chatID;
				wsConnectedRef.current = false;
				store.replaceMessages([]);
			}
			// Once the WebSocket is connected it owns the store.
			// REST data changes are ignored to prevent stale
			// refetches from racing with WS-delivered state.
			if (wsConnectedRef.current) {
				return;
			}
			// Hydrate from REST before the WS connects.
			if (chatMessages) {
				// Detect edit truncation: if the store holds IDs
				// not in the fetched set, the server truncated
				// messages (e.g. after an edit). Use
				// replaceMessages so the stale entries are
				// removed. Otherwise use upsertDurableMessages so
				// messages delivered by the WS handler between
				// socket creation and onOpen are preserved.
				const storeSnap = store.getSnapshot();
				const fetchedIDs = new Set(chatMessages.map((m) => m.id));
				const hasStaleEntries = storeSnap.orderedMessageIDs.some(
					(id) => !fetchedIDs.has(id),
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
		// Only hydrate status from REST before the WS connects.
		// Once connected, the WS is authoritative for status.
		if (!wsConnectedRef.current) {
			store.setChatStatus(chatRecord?.status ?? null);
		}
	}, [chatRecord?.status, store]);

	useEffect(() => {
		store.setQueuedMessages([]);
		if (!chatID) {
			return;
		}
	}, [chatID, store]);

	useEffect(() => {
		if (!chatID || !chatMessagesData) {
			return;
		}
		// Only hydrate queued messages from REST before the WS
		// connects. Once connected, queue_update events from the
		// WS are authoritative.
		if (wsConnectedRef.current) {
			return;
		}
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

		// True for the first handleMessage call after onOpen.
		// The initial snapshot batch may contain message_part
		// events for already-committed messages (the server
		// buffer accumulates across the entire processChat run).
		// We use this flag to filter stale parts from the
		// snapshot.
		let isSnapshotBatch = false;
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

			// On the first batch after connect (the snapshot),
			// filter out stale message_part events. The server
			// buffer accumulates parts for the entire
			// processChat run, including parts for
			// already-committed messages. Find the position of
			// the last durable assistant message in the batch.
			// Parts before it are stale. If there are no durable
			// messages, all parts may be stale — skip them and
			// let the live stream deliver fresh parts.
			let snapshotStalePartCutoff = -1;
			if (isSnapshotBatch) {
				isSnapshotBatch = false;
				const storeSnap = store.getSnapshot();
				// Find the last durable message in the batch
				// that the store already has (from REST).
				for (let i = streamEvents.length - 1; i >= 0; i--) {
					const ev = streamEvents[i];
					if (
						ev.type === "message" &&
						ev.message?.id !== undefined &&
						storeSnap.messagesByID.has(ev.message.id)
					) {
						snapshotStalePartCutoff = i;
						break;
					}
				}
				// If there are no durable messages in the batch
				// but the store has committed assistant messages,
				// some parts may replay their content. Build a
				// prefix string from committed assistant text and
				// skip parts that reproduce it.
				if (
					snapshotStalePartCutoff === -1 &&
					storeSnap.orderedMessageIDs.length > 0
				) {
					// Collect text from all committed assistant
					// messages in order. The server buffer
					// replays parts for these messages before the
					// live in-flight content.
					let committedText = "";
					for (const msgId of storeSnap.orderedMessageIDs) {
						const msg = storeSnap.messagesByID.get(msgId);
						if (msg?.role === "assistant") {
							for (const part of msg.content ?? []) {
								if (part.type === "text" && part.text) {
									committedText += part.text;
								}
							}
						}
					}
					if (committedText.length > 0) {
						// Walk the snapshot parts and consume the
						// committed prefix. Once we've consumed all
						// of it, the remaining parts are live.
						let consumed = 0;
						for (let i = 0; i < streamEvents.length; i++) {
							if (consumed >= committedText.length) {
								break;
							}
							const ev = streamEvents[i];
							if (ev.type !== "message_part") {
								continue;
							}
							const partText =
								ev.message_part?.part?.type === "text"
									? (ev.message_part.part.text ?? "")
									: "";
							if (partText.length > 0) {
								consumed += partText.length;
								snapshotStalePartCutoff = i + 1;
							}
						}
					}
				}
			}
			// Collect durable messages for bulk upsert so the
			// entire batch produces one Map copy + one sort
			// instead of N copies and N sorts.
			const pendingMessages: TypesGen.ChatMessage[] = [];
			let needsStreamReset = false;
			// Wrap all store mutations in a batch so subscribers
			// are notified exactly once at the end, not per event.
			store.batch(() => {
				for (let eventIdx = 0; eventIdx < streamEvents.length; eventIdx++) {
					const streamEvent = streamEvents[eventIdx];
					if (streamEvent.type === "message_part") {
						if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
							continue;
						}
						if (!shouldApplyMessagePart()) {
							continue;
						}
						// Skip stale parts from the snapshot batch.
						// These correspond to already-committed
						// messages and would duplicate content that
						// ConversationTimeline already renders.
						if (eventIdx < snapshotStalePartCutoff) {
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
				// The WebSocket is now connected and authoritative
				// for all mutable chat state. REST data changes
				// are ignored until the socket tears down.
				wsConnectedRef.current = true;
				// Mark the next handleMessage call as the
				// snapshot batch so stale parts are filtered.
				isSnapshotBatch = true;
				// Drop transport-scoped state from the previous
				// socket attempt so stale partial output or
				// failures do not leak into the new stream.
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
			wsConnectedRef.current = false;
			disposeSocket();
			if (partsFlushTimer !== null) {
				clearTimeout(partsFlushTimer);
			}
			// Snapshot the store into the React Query cache so
			// navigating back shows up-to-date messages without
			// a round-trip.
			snapshotStoreToCache(activeChatID);
			activeChatIDRef.current = null;
		};
	}, [chatID, initialDataLoaded, queryClient, store]);
	return {
		store,
		clearStreamError: () => {
			store.clearStreamError();
		},
	};
};
