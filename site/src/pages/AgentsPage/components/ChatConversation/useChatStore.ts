import {
	useCallback,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { type InfiniteData, useQueryClient } from "react-query";
import { watchChat } from "#/api/api";
import {
	chatMessagesKey,
	chatPromptsKey,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { reportClientError } from "#/utils/clientErrorReporting";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { normalizeChatErrorPayload } from "./chatError";
import {
	type ChatStore,
	type ChatStoreState,
	chatMessagesEqualByValue,
	chatQueuedMessagesEqualByID,
	createChatStore,
	isActiveChatStatus,
} from "./chatStore";
import type { RetryState } from "./types";

const normalizeRetryState = (retry: TypesGen.ChatStreamRetry): RetryState => ({
	attempt: Math.max(1, retry.attempt),
	error: retry.error.trim() || "Retrying request shortly.",
	kind: retry.kind ?? "generic",
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
	aiGatewayDisabled?: boolean;
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
		aiGatewayDisabled = false,
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
	const upsertCacheMessages = useCallback(
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
			// Refresh the dedicated prompt-history cache when a user message arrives.
			const hasNewUserPrompt = messages.some((msg) => msg.role === "user");
			if (hasNewUserPrompt) {
				void queryClient.invalidateQueries({
					queryKey: chatPromptsKey(chatID),
					exact: true,
				});
			}
		},
		[chatID, queryClient],
	);

	const replaceCacheMessages = useCallback(
		(messages: readonly TypesGen.ChatMessage[]) => {
			if (!chatID) {
				return;
			}
			queryClient.setQueryData<
				InfiniteData<TypesGen.ChatMessagesResponse> | undefined
			>(chatMessagesKey(chatID), (currentData) => {
				if (!currentData?.pages?.length) {
					return currentData;
				}
				const firstPage = currentData.pages[0];
				const updatedMessages = [...messages].sort((a, b) => b.id - a.id);
				return {
					...currentData,
					pages: [{ ...firstPage, messages: updatedMessages, has_more: false }],
					pageParams: currentData.pageParams.slice(0, 1),
				};
			});
		},
		[chatID, queryClient],
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
				// added to the store after the last sync (for example
				// by the WS handler) are new, not stale, and must not
				// trigger the destructive replaceMessages path.
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
		// Suppression entries are scoped to the current chat; clear
		// them on chat change so a stale promote suppression doesn't
		// hide queued messages in another chat.
		store.clearSuppressedQueuedMessageIDs();
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
		store.applyAuthoritativeQueuedMessages(chatQueuedMessages);
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

		if (!chatID || !initialDataLoaded || aiGatewayDisabled) {
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

		// History replacement state lives at the effect scope because
		// the server may split a history_reset and its replacement
		// messages across multiple WS frames (the stream handler caps
		// frames at a fixed batch size). Replacement messages are
		// buffered until a non-message boundary event arrives; the
		// server always emits preview_reset after a history change in
		// the same sync, so the run is guaranteed to terminate.
		let historyResetPending = false;
		const historyReplacementBuf: TypesGen.ChatMessage[] = [];

		// ID of the current server-side stream connection, taken from
		// the stream_connected event the server sends first on every
		// connection (including reconnects). Included in error reports
		// so they can be correlated with server logs, which carry the
		// same stream ID.
		let serverStreamID: string | undefined;

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
		// the preview is reset or the stream is no longer active
		// so stale buffered parts are not applied after the
		// boundary event.
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
				console.error("Failed to parse chat stream update", payload.parseError);
				// The event data is typed as string, but binary frames
				// surface as Blob at runtime; normalize for reporting.
				const frame: unknown = payload.sourceEvent.data;
				const frameText = typeof frame === "string" ? frame : String(frame);
				reportClientError(
					payload.parseError ??
						new Error("chat stream frame parsed to a falsy value"),
					{
						chatId: chatID,
						streamId: serverStreamID ?? "unknown",
						frameSnippet: frameText,
						frameLength: String(frameText.length),
					},
				);
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
			// Collect durable messages for bulk upsert so the
			// entire batch produces one Map copy + one sort
			// instead of N copies and N sorts.
			const pendingMessages: TypesGen.ChatMessage[] = [];
			let needsStreamReset = false;

			// Atomically swap in the buffered replacement history. Called
			// when a boundary event signals the replacement run ended, so
			// a run split across frames never renders a truncated
			// conversation.
			const commitHistoryReplacement = () => {
				if (!historyResetPending) {
					return;
				}
				historyResetPending = false;
				const replacement = historyReplacementBuf.splice(0);
				store.replaceMessages(replacement);
				replaceCacheMessages(replacement);
			};

			// Wrap all store mutations in a batch so subscribers
			// are notified exactly once at the end, not per event.
			store.batch(() => {
				for (const streamEvent of streamEvents) {
					if (streamEvent.type === "stream_connected") {
						serverStreamID = streamEvent.stream_connected?.stream_id;
						continue;
					}
					if (streamEvent.type === "message_part") {
						if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
							continue;
						}
						commitHistoryReplacement();
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

					if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
						const nextStatus = streamEvent.status?.status;
						if (streamEvent.type === "status" && nextStatus) {
							store.setSubagentStatusOverride(streamEvent.chat_id, nextStatus);
						}
						continue;
					}

					if (streamEvent.type === "history_reset") {
						discardBufferedParts();
						store.clearStreamState();
						// A newer reset supersedes any in-flight replacement
						// run, so restart buffering instead of committing.
						historyResetPending = true;
						historyReplacementBuf.length = 0;
						pendingMessages.length = 0;
						needsStreamReset = false;
						continue;
					}

					// Any non-message event for this chat marks the end of
					// a replacement run (the server emits replacement
					// messages contiguously after history_reset).
					if (streamEvent.type !== "message") {
						commitHistoryReplacement();
					}

					if (streamEvent.type === "preview_reset") {
						discardBufferedParts();
						store.clearStreamState();
						continue;
					}

					// Only flush buffered parts before events that
					// need them applied first. `message` events
					// commit durable state that must include all
					// stream parts. `error` events should surface
					// partial output. Other events (status, retry,
					// queue_update) must not flush. Status changes
					// need to be visible before parts so the
					// Thinking indicator can render, and retry
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
							store.clearRetryState();
							if (historyResetPending) {
								historyReplacementBuf.push(message);
							} else {
								pendingMessages.push(message);
							}
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
							wsQueueUpdateReceivedRef.current = true;
							store.applyAuthoritativeQueuedMessages(
								streamEvent.queued_messages,
							);
							updateChatQueuedMessages(streamEvent.queued_messages);
							continue;
						case "status": {
							const nextStatus = streamEvent.status?.status;
							if (!nextStatus) {
								continue;
							}

							wsStatusReceivedRef.current = true;
							store.clearRetryState();
							store.setChatStatus(nextStatus);
							if (nextStatus === "pending" || nextStatus === "waiting") {
								discardBufferedParts();
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
							const reason = normalizeChatErrorPayload(streamEvent.error) ?? {
								kind: "generic",
								message: "Chat processing failed.",
							};
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
				// Drop any partial replacement run from the old
				// socket; the new socket replays a fresh snapshot.
				historyResetPending = false;
				historyReplacementBuf.length = 0;
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
	}, [
		aiGatewayDisabled,
		chatID,
		initialDataLoaded,
		queryClient,
		replaceCacheMessages,
		store,
		upsertCacheMessages,
	]);
	return {
		store,
		clearStreamError: () => {
			store.clearStreamError();
		},
		upsertCacheMessages,
	};
};
