import {
	useCallback,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import {
	type InfiniteData,
	type QueryClient,
	useInfiniteQuery,
	useQueryClient,
} from "react-query";
import { watchChat } from "#/api/api";
import {
	type MessagesCacheWrite,
	writeMessagesCache,
} from "#/api/queries/chatMessagesCache";
import {
	applyServerChatStatusToCaches,
	chatKeys,
	chatMessagesForInfiniteScroll,
	readCachedChatSnapshotVersion,
	readCachedChatStatus,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { normalizeChatErrorPayload } from "./chatError";
import {
	type ChatStore,
	type ChatStoreState,
	chatMessagesEqualByValue,
	chatQueuedMessagesEqualByValue,
	createChatStore,
	isActiveChatStatus,
} from "./chatStore";
import {
	readCanonicalQueuedMessages,
	selectCanonicalQueuedMessages,
} from "./durableChat";
import type { RetryState } from "./types";

type ChatMessagesCacheData = InfiniteData<TypesGen.ChatMessagesResponse>;

/**
 * Canonicalizes the incoming messages into page 0.
 *
 * Every incoming ID is removed from EVERY page first, so an in-place revision
 * of a message that is also held by an older page cannot leave the stale copy
 * behind. Page metadata (`has_more`, `queued_messages`) is preserved per page.
 *
 * A non-zero page emptied by that removal is dropped together with its page
 * param: `getNextPageParam` reads the LAST page, so an empty last page would
 * flip `hasNextPage` to false and strand the rest of the history. Dropping the
 * page param alongside the page is safe because only `pageParams[0]` is reused
 * on a refetch; later cursors are recomputed from the refetched pages.
 */
const upsertMessagesIntoCacheData = (
	currentData: ChatMessagesCacheData,
	incoming: readonly TypesGen.ChatMessage[],
): ChatMessagesCacheData => {
	const incomingByID = new Map<number, TypesGen.ChatMessage>();
	for (const message of incoming) {
		incomingByID.set(message.id, message);
	}

	const [firstPage, ...olderPages] = currentData.pages;
	const firstPageByID = new Map(
		firstPage.messages.map((message) => [message.id, message]),
	);
	const changesFirstPage = Array.from(incomingByID.values()).some((message) => {
		const existing = firstPageByID.get(message.id);
		return !existing || !chatMessagesEqualByValue(existing, message);
	});
	const hasOlderPageCopy = olderPages.some((page) =>
		page.messages.some((message) => incomingByID.has(message.id)),
	);
	if (!changesFirstPage && !hasOlderPageCopy) {
		return currentData;
	}

	const nextPages: TypesGen.ChatMessagesResponse[] = [];
	const nextPageParams: unknown[] = [];
	for (const [index, page] of olderPages.entries()) {
		const kept = page.messages.filter(
			(message) => !incomingByID.has(message.id),
		);
		if (kept.length === 0 && page.messages.length > 0) {
			continue;
		}
		nextPages.push(
			kept.length === page.messages.length ? page : { ...page, messages: kept },
		);
		// olderPages is offset by one from currentData.pages.
		nextPageParams.push(currentData.pageParams[index + 1]);
	}

	// Newest first, matching the API's page order.
	const nextFirstPageMessages = [
		...firstPage.messages.filter((message) => !incomingByID.has(message.id)),
		...incomingByID.values(),
	].sort((left, right) => right.id - left.id);

	return {
		...currentData,
		pages: [{ ...firstPage, messages: nextFirstPageMessages }, ...nextPages],
		pageParams: [currentData.pageParams[0], ...nextPageParams],
	};
};

/**
 * Installs a replacement history as the only page. The socket sends the full
 * replacement history after a `history_reset`, so every older page it
 * superseded goes away with it; `pageParams` collapses to the uncursored
 * initial page param.
 */
const replaceMessagesInCacheData = (
	currentData: ChatMessagesCacheData,
	messages: readonly TypesGen.ChatMessage[],
): ChatMessagesCacheData => ({
	...currentData,
	pages: [
		{
			...currentData.pages[0],
			messages: [...messages].sort((left, right) => right.id - left.id),
			has_more: false,
		},
	],
	pageParams: [undefined],
});

/**
 * Replaces the queue snapshot the messages cache carries on page 0. The queue
 * travels with the messages response, so it shares the cache entry, the fetch
 * serialization, and the initialized-cache requirement with durable messages.
 *
 * No equality guard: `setQueryData` runs `replaceEqualDeep`, so a deep-equal
 * write collapses to the previous reference and notifies nothing.
 */
const patchQueuedMessagesInCacheData = (
	currentData: ChatMessagesCacheData,
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
): ChatMessagesCacheData => {
	const firstPage = currentData.pages[0];
	return {
		...currentData,
		pages: [
			{ ...firstPage, queued_messages: queuedMessages },
			...currentData.pages.slice(1),
		],
	};
};

// The cache is canonical for the queue, so only SERVER-DERIVED values are
// written here: an authoritative snapshot the gate accepted, or the
// promoted-head send reconciliation's projection. Optimistic removals are
// read-time markers instead and never reach this function.
//
// The SINGLE place the cache echo is armed. The cache arm of the gate would
// otherwise re-gate this write when it observes it, retiring the markers the
// write depends on. Only a write that CHANGES the cached value is owed an
// echo: `setQueryData` collapses a deep-equal write through structural
// sharing, so no observation follows it, and an expectation armed for one
// would sit there and swallow the next genuine snapshot of that value. A
// projection that merely confirms the cached queue is exactly such a no-op
// write, which is why the gate never arms its own.
//
// Goes through the shared write coordinator: a queue snapshot committed while
// a pagination fetch is in flight would otherwise be clobbered by the pages
// that fetch captured, resurrecting entries the server already removed.
const writeQueuedMessagesToCache = (
	queryClient: QueryClient,
	store: ChatStore,
	chatID: string | undefined,
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
): void => {
	if (!chatID) {
		return;
	}
	const cachedQueue = readCanonicalQueuedMessages(queryClient, chatID);
	if (
		cachedQueue !== undefined &&
		!chatQueuedMessagesEqualByValue(cachedQueue, queuedMessages)
	) {
		store.noteLocalQueueProjection(queuedMessages);
	}
	writeMessagesCache(queryClient, chatKeys.messages(chatID), {
		kind: "queue",
		apply: (currentData) =>
			patchQueuedMessagesInCacheData(currentData, queuedMessages),
	});
};

const normalizeRetryState = (retry: TypesGen.ChatStreamRetry): RetryState => ({
	attempt: Math.max(1, retry.attempt),
	error: retry.error.trim() || "Retrying request shortly.",
	kind: retry.kind ?? "generic",
	provider: retry.provider?.trim() || undefined,
	retryingAt: retry.retrying_at.trim() || undefined,
});

const shouldSurfaceReconnectState = (
	state: ChatStoreState,
	chatStatus: TypesGen.ChatStatus | null,
): boolean =>
	state.streamError === null &&
	(state.streamState !== null ||
		state.retryState !== null ||
		isActiveChatStatus(chatStatus));

interface UseChatStoreOptions {
	chatID: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: TypesGen.Chat | undefined;
	setChatErrorReason: (chatID: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatID: string) => void;
	aiGatewayDisabled?: boolean;
}

export const useChatStore = (
	options: UseChatStoreOptions,
): {
	store: ChatStore;
	clearStreamError: () => void;
	// Writes a server-derived queue value into the canonical cache. Used by the
	// promoted-head send reconciliation only.
	writeCanonicalQueuedMessages: (
		queuedMessages: readonly TypesGen.ChatQueuedMessage[],
	) => void;
	// The RAW cached queue, suppression markers included.
	readCanonicalQueuedMessages: () =>
		| readonly TypesGen.ChatQueuedMessage[]
		| undefined;
	upsertCacheMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
} => {
	const {
		chatID,
		chatMessages,
		chatRecord,
		setChatErrorReason,
		clearChatErrorReason,
		aiGatewayDisabled = false,
	} = options;

	const queryClient = useQueryClient();
	const [store] = useState(createChatStore);
	const activeChatIDRef = useRef<string | null>(null);

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

	// True once the initial REST pages have resolved for the current chat. The
	// WebSocket effect gates on this so lastMessageIdRef is populated before
	// the socket opens (otherwise the server replays the entire message history
	// as its snapshot, defeating pagination), and so the detail cache exists
	// before the socket can write a status into it. Gating on a derived boolean
	// rather than the chat record itself keeps every detail write from
	// reconnecting the socket.
	const initialDataLoaded =
		chatMessages !== undefined && chatRecord !== undefined;

	// Write WebSocket-delivered durable messages into the React
	// Query infinite cache. The cache is canonical for durable
	// messages and the socket is their single ordered writer.
	//
	// Every write goes through the shared per-chat coordinator in
	// `chatMessagesCache`, which serializes it against fetches on the
	// same cache entry (see that module for why). The queue writes
	// and the edit mutation share the coordinator, so all writers to
	// this entry are ordered against each other and against fetches.
	const writeMessagesToCache = useCallback(
		(write: MessagesCacheWrite) => {
			if (!chatID) {
				return;
			}
			writeMessagesCache(queryClient, chatKeys.messages(chatID), write);
		},
		[chatID, queryClient],
	);

	const upsertCacheMessages = useCallback(
		(messages: readonly TypesGen.ChatMessage[]) => {
			if (!chatID || messages.length === 0) {
				return;
			}
			writeMessagesToCache({
				kind: "upsert",
				apply: (currentData) =>
					upsertMessagesIntoCacheData(currentData, messages),
			});
			// Refresh the dedicated prompt-history cache when a user message arrives.
			const hasNewUserPrompt = messages.some((msg) => msg.role === "user");
			if (hasNewUserPrompt) {
				void queryClient.invalidateQueries({
					queryKey: chatKeys.prompts(chatID),
					exact: true,
				});
			}
		},
		[chatID, queryClient, writeMessagesToCache],
	);

	const replaceCacheMessages = useCallback(
		(messages: readonly TypesGen.ChatMessage[]) => {
			writeMessagesToCache({
				kind: "replace",
				apply: (currentData) =>
					replaceMessagesInCacheData(currentData, messages),
			});
		},
		[writeMessagesToCache],
	);

	useEffect(() => {
		if (!chatID) {
			return;
		}
		// Markers and the gate's snapshot bookkeeping describe the open chat, so
		// they are dropped on the way out: a stale promote suppression must not
		// hide a queued message in the chat opened next.
		return () => {
			store.clearSuppressedQueuedMessageIDs();
		};
	}, [chatID, store]);

	// Cache arm of the authoritative-snapshot gate. A page-0 install is an
	// authoritative queue snapshot too, so it has to reconcile the markers
	// exactly like a `queue_update` does. It is a POST-INSTALL observer, not a
	// write gate: by the time the effect runs the value IS the cached one, so a
	// rejected snapshot stays installed and is only withheld from the marker
	// reconciliation. The store consumes the echo of a write this client made,
	// so observing the socket arm's own write is not mistaken for a server
	// statement.
	const cachedQueuedMessages = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(chatID ?? ""),
		enabled: Boolean(chatID),
		select: selectCanonicalQueuedMessages,
	}).data;
	useEffect(() => {
		if (!chatID || cachedQueuedMessages === undefined) {
			return;
		}
		store.acceptAuthoritativeQueueSnapshot(cachedQueuedMessages, "cache");
	}, [cachedQueuedMessages, chatID, store]);

	useEffect(() => {
		store.resetTransientState();
		activeChatIDRef.current = chatID ?? null;
		store.setActiveChatID(chatID ?? null);

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

		const shouldApplyMessagePart = (): boolean =>
			readCachedChatStatus(queryClient, activeChatID) !== "waiting";

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
			// Collect durable messages for one bulk cache write so the
			// entire batch produces a single query-cache notification.
			const pendingMessages: TypesGen.ChatMessage[] = [];
			// The newest assistant message committed in this batch. Its ID is the
			// identity the overlay hands off to, so the tail stays on screen until
			// exactly that message is readable from the durable cache.
			let finalizedAssistantMessageID: number | undefined;

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
				replaceCacheMessages(replacement);
			};

			// Wrap all store mutations in a batch so subscribers
			// are notified exactly once at the end, not per event.
			store.batch(() => {
				for (const streamEvent of streamEvents) {
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
						finalizedAssistantMessageID = undefined;
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
						// The backend emits the durable `message` for a snapshot BEFORE
						// the `preview_reset` that retires the preview, usually in the
						// same frame. Clearing the overlay here would strand the tail
						// that just finalized: the end-of-batch handoff would find no
						// overlay to keep, and nothing would render the finalized turn
						// until its cache write became readable. So when a handoff is
						// pending, the handoff performs the clear instead: it moves the
						// overlay into the finalizing snapshot keyed by that message's
						// ID, which every clear path and the next turn's first part
						// still retire.
						const finalizationPending =
							finalizedAssistantMessageID !== undefined ||
							store.getSnapshot().finalizingMessageID !== null;
						if (!finalizationPending) {
							store.clearStreamState();
						}
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
								lastMessageIdRef.current === undefined ||
								message.id > lastMessageIdRef.current
							) {
								lastMessageIdRef.current = message.id;
							}
							if (message.role === "assistant") {
								finalizedAssistantMessageID = message.id;
							}
							continue;
						}
						case "queue_update": {
							// The gate decides; a rejected snapshot must not reach the
							// cache. An accepted one is written VERBATIM, because the
							// cache holds server truth and the suppression markers hide
							// the transient rows at read time.
							const snapshot = streamEvent.queued_messages ?? [];
							if (!store.acceptAuthoritativeQueueSnapshot(snapshot, "socket")) {
								continue;
							}
							writeQueuedMessagesToCache(queryClient, store, chatID, snapshot);
							continue;
						}
						case "status": {
							const nextStatus = streamEvent.status?.status;
							if (!nextStatus) {
								continue;
							}

							// Writes detail, parent details, list rows, embedded
							// children, and search rows in one pass, each ordered
							// against its own cached version. A superseded or
							// versionless status is not this chat's current one,
							// so its stream side effects must not run either.
							if (
								!applyServerChatStatusToCaches(
									queryClient,
									activeChatID,
									nextStatus,
									streamEvent.snapshot_version,
								)
							) {
								continue;
							}
							store.clearRetryState();
							if (nextStatus === "waiting") {
								discardBufferedParts();
							}
							if (nextStatus !== "error") {
								clearChatErrorReasonEvent(activeChatID);
							}
							continue;
						}
						case "error": {
							const reason = normalizeChatErrorPayload(streamEvent.error) ?? {
								kind: "generic",
								message: "Chat processing failed.",
							};
							// An error without a snapshot version did not come from a
							// committed chat snapshot (a subscription or transport
							// failure), so it reports the failure without claiming the
							// chat itself reached the error status.
							const errorVersion = streamEvent.snapshot_version;
							if (errorVersion !== undefined) {
								const applied = applyServerChatStatusToCaches(
									queryClient,
									activeChatID,
									"error",
									errorVersion,
								);
								// The server stamps a status event and the error event
								// detailing it with one snapshot version, and the status
								// event is emitted first. So an equal-version error is the
								// companion of an error status already applied, not a
								// duplicate: the reason it carries is the only place the
								// failure detail arrives. A strictly older error still
								// loses, because the chat has moved past its snapshot.
								const detailsAppliedErrorStatus =
									readCachedChatStatus(queryClient, activeChatID) === "error" &&
									readCachedChatSnapshotVersion(queryClient, activeChatID) ===
										errorVersion;
								if (!applied && !detailsAppliedErrorStatus) {
									continue;
								}
							}
							store.setStreamError(reason);
							store.clearRetryState();
							setChatErrorReasonEvent(activeChatID, reason);
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

				// Commit the batch's durable messages, then hand the overlay over to
				// the message that superseded it: cache first so any reader woken by
				// the store notification already sees that message. The handoff keeps
				// the finalized tail on screen until the durable-reading component
				// renders it, which is also what retires the handoff.
				if (pendingMessages.length > 0) {
					upsertCacheMessages(pendingMessages);
				}

				if (finalizedAssistantMessageID !== undefined) {
					store.beginStreamFinalization(finalizedAssistantMessageID);
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
				if (
					shouldSurfaceReconnectState(
						snapshot,
						readCachedChatStatus(queryClient, activeChatID),
					)
				) {
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
			store.setActiveChatID(null);
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
		writeCanonicalQueuedMessages: (queuedMessages) => {
			writeQueuedMessagesToCache(queryClient, store, chatID, queuedMessages);
		},
		readCanonicalQueuedMessages: () =>
			readCanonicalQueuedMessages(queryClient, chatID),
		upsertCacheMessages,
	};
};
