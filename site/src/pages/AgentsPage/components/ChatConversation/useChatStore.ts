import {
	useCallback,
	useEffect,
	useEffectEvent,
	useLayoutEffect,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { watchChat } from "#/api/api";
import {
	fetchNewestChatMessages,
	fetchOlderChatMessages,
	invalidateChatPrompts,
	invalidateChatSearches,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	createReconnectingWebSocket,
	type ReconnectSchedule,
} from "#/utils/reconnectingWebSocket";
import { type ChatDetailError, normalizeChatErrorPayload } from "./chatError";
import {
	type ChatStore,
	type ChatStoreState,
	createChatStore,
	isActiveChatStatus,
	selectShownError,
	selectStreamState,
} from "./chatStore";
import type { RetryState } from "./types";

const normalizeRetryState = (retry: TypesGen.ChatStreamRetry): RetryState => ({
	attempt: Math.max(1, retry.attempt),
	error: retry.error.trim() || "Retrying request shortly.",
	kind: retry.kind ?? "generic",
	provider: retry.provider?.trim() || undefined,
	retryingAt: retry.retrying_at.trim() || undefined,
});

const shouldSurfaceReconnectState = (state: ChatStoreState): boolean =>
	selectShownError(state) === null &&
	(selectStreamState(state) !== null ||
		state.retryState !== null ||
		isActiveChatStatus(state.chatStatus));

/** Exported so story fixtures can put a loaded store in the query cache. */
export const chatStoreKey = (chatID: string) => ["chatStore", chatID] as const;

/** Progress of loading a chat's newest messages page. */
type ChatLoadState =
	| { status: "loading" }
	| { status: "loaded" }
	| { status: "failed"; error: unknown };

type UseChatStoreOptions = {
	chatID: string;
	setChatErrorReason: (chatID: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatID: string) => void;
	aiGatewayDisabled?: boolean;
};

export const useChatStore = (
	options: UseChatStoreOptions,
): {
	store: ChatStore;
	loadState: ChatLoadState;
	retryLoad: () => void;
	clearClientError: () => void;
	fetchMoreMessages: () => Promise<unknown>;
	isFetchingMoreMessages: boolean;
	hasFetchMoreError: boolean;
} => {
	const {
		chatID,
		setChatErrorReason,
		clearChatErrorReason,
		aiGatewayDisabled = false,
	} = options;

	const queryClient = useQueryClient();
	// A chat keeps its store as long as the query cache keeps an unused query,
	// so reopening it shows the chat as it was left and the stream resumes
	// from the store's snapshot version. After the newest page, only stream
	// events and older pages change the store, so the query never refetches.
	const storeQuery = useQuery({
		queryKey: chatStoreKey(chatID),
		queryFn: async ({ signal }) => {
			const page = await fetchNewestChatMessages(chatID, signal);
			const store = createChatStore();
			store.seed(page);
			return store;
		},
		staleTime: "static",
		retry: false,
		structuralSharing: false,
	});
	// Stands in until the newest page loads. The page shows its loading view
	// then, so nothing acts on this store.
	const [emptyStore] = useState(createChatStore);
	const store = storeQuery.data ?? emptyStore;
	const initialDataLoaded = storeQuery.isSuccess;
	const loadState: ChatLoadState = storeQuery.isSuccess
		? { status: "loaded" }
		: storeQuery.isError
			? { status: "failed", error: storeQuery.error }
			: { status: "loading" };

	// Wrap error-reason callbacks so the WebSocket effect can call
	// them without including them in its dependency array.
	const setChatErrorReasonEvent = useEffectEvent(setChatErrorReason);
	const clearChatErrorReasonEvent = useEffectEvent(clearChatErrorReason);

	const onDurableMessages = useCallback(
		(messages: readonly TypesGen.ChatMessage[]) => {
			if (messages.length === 0) {
				return;
			}
			// Refresh the dedicated prompt-history cache when a user message arrives.
			const hasNewUserPrompt = messages.some((msg) => msg.role === "user");
			if (hasNewUserPrompt) {
				void invalidateChatPrompts(queryClient, chatID);
			}
			void invalidateChatSearches(queryClient);
		},
		[chatID, queryClient],
	);

	const {
		mutateAsync: fetchMoreMessages,
		isPending: isFetchingMoreMessages,
		isError: hasFetchMoreError,
	} = useMutation({
		mutationFn: async () => {
			const oldestID = store.getSnapshot().orderedMessageIDs[0];
			if (oldestID === undefined) {
				return;
			}
			const resetCount = store.getHistoryResetCount();
			const page = await fetchOlderChatMessages(chatID, oldestID);
			store.prependOlderMessages(page.messages, page.has_more, resetCount);
		},
	});

	// A reopened chat's store holds the previous visit's stream state. A layout
	// effect clears it before the first render paints.
	useLayoutEffect(() => {
		store.resetTransientState();
	}, [store, aiGatewayDisabled]);

	useEffect(() => {
		const updateSidebarChat = (
			updater: (chat: TypesGen.Chat) => TypesGen.Chat,
		) => {
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

		if (!initialDataLoaded || aiGatewayDisabled) {
			return;
		}

		// Local disposed flag so the message handler (which lives
		// outside the utility) can bail out after cleanup.
		let disposed = false;

		// Parts buffer lives at the effect scope so it persists
		// across WebSocket messages. A rAF-based flush coalesces
		// parts from multiple WS messages into a single render,
		// capping stream renders to once per animation frame.
		const partsBuf: TypesGen.ChatMessagePart[] = [];
		let partsFlushTimer: ReturnType<typeof setTimeout> | null = null;

		// Set when the stream reports "waiting", cleared by any other
		// stream status. While set, parts are dropped: they are late
		// leftovers from the finished turn.
		let streamReportedWaiting = false;

		const shouldKeepMessagePart = (): boolean => !streamReportedWaiting;

		const schedulePartsFlush = () => {
			if (partsFlushTimer !== null || partsBuf.length === 0) {
				return;
			}
			partsFlushTimer = setTimeout(() => {
				partsFlushTimer = null;
				if (disposed) {
					return;
				}
				if (!shouldKeepMessagePart()) {
					partsBuf.length = 0;
					return;
				}
				store.applyMessageParts(partsBuf.splice(0));
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
			if (!shouldKeepMessagePart()) {
				partsBuf.length = 0;
				return;
			}
			store.applyMessageParts(partsBuf.splice(0));
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
			socket: { close: () => void },
		) => {
			if (disposed) {
				return;
			}
			if (payload.parseError || !payload.parsedMessage) {
				// A lost frame can hold events that a later event's version would
				// skip on reconnect, so resync from the last applied version.
				discardBufferedParts();
				store.setClientError({
					kind: "generic",
					message: "Failed to parse a chat stream update. Reconnecting.",
				});
				socket.close();
				return;
			}

			const streamEvents = payload.parsedMessage;
			if (streamEvents.length === 0) {
				return;
			}
			// Consecutive messages are upserted together: one Map copy and one
			// sort instead of one per message.
			const pendingMessages: TypesGen.ChatMessage[] = [];
			let userMessageVersion = 0;
			let needsStreamReset = false;

			const flushMessages = () => {
				if (pendingMessages.length > 0) {
					store.upsertDurableMessages(pendingMessages, userMessageVersion);
					onDurableMessages(pendingMessages.splice(0));
					userMessageVersion = 0;
				}
			};

			// Wrap all store mutations in a batch so subscribers
			// are notified exactly once at the end, not per event.
			store.batch(() => {
				for (const streamEvent of streamEvents) {
					if (streamEvent.type === "message_part") {
						if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
							continue;
						}
						if (!shouldKeepMessagePart()) {
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

					// The stream sets it on message, queue_update and history_reset.
					const version = streamEvent.snapshot_version ?? 0;
					store.advanceSnapshotVersion(version);

					// Every other event applies after the messages before it.
					if (streamEvent.type !== "message") {
						flushMessages();
					}

					if (streamEvent.type === "history_reset") {
						discardBufferedParts();
						store.clearStreamState();
						store.resetHistoryFrom(
							streamEvent.history_reset?.from_message_id ?? 0,
							version,
						);
						void invalidateChatSearches(queryClient);
						needsStreamReset = false;
						continue;
					}

					if (streamEvent.type === "preview_reset") {
						discardBufferedParts();
						store.clearStreamState();
						continue;
					}

					// Flush buffered parts before a durable message
					// commits them. Other events must not flush because
					// some clear the stream and a flush would restore it.
					if (streamEvent.type === "message") {
						flushMessageParts();
					}

					switch (streamEvent.type) {
						case "message": {
							const message = streamEvent.message;
							if (!message) {
								continue;
							}
							store.clearRetryState();
							pendingMessages.push(message);
							if (message.role === "user") {
								userMessageVersion = Math.max(userMessageVersion, version);
							}
							if (message.role === "assistant") {
								needsStreamReset = true;
							}
							continue;
						}
						case "queue_update":
							store.setQueuedMessages(
								streamEvent.queued_messages ?? [],
								version,
							);
							continue;
						case "status": {
							const nextStatus = streamEvent.status?.status;
							if (!nextStatus) {
								continue;
							}

							streamReportedWaiting = nextStatus === "waiting";
							store.clearRetryState();
							const prevStatus = store.getSnapshot().chatStatus;
							store.applyServerChatStatus(nextStatus);
							if (nextStatus === "waiting") {
								discardBufferedParts();
							}
							if (nextStatus !== "error") {
								clearChatErrorReasonEvent(chatID);
								// Only an errored chat starting a new turn
								// supersedes the stored error. A status from a
								// turn that was already running must not clear
								// an unrelated request failure.
								if (prevStatus === "error") {
									store.clearClientError();
								}
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
							// An error ends the turn. Clear the partial
							// stream so no tool keeps spinning. Parts stay
							// ungated because the next turn can send a part
							// before its running status.
							discardBufferedParts();
							store.applyServerChatStatus("error");
							store.setTurnError(reason);
							store.clearStreamState();
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

				flushMessages();

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
						if (shouldKeepMessagePart()) {
							store.applyMessageParts(partsBuf.splice(0));
						}
					}
				}
			});
		};
		const disposeSocket = createReconnectingWebSocket({
			connect() {
				// The server sends the changes made at or after this version.
				const socket = watchChat(chatID, store.getSyncedSnapshotVersion());
				socket.addEventListener("message", (payload) =>
					handleMessage(payload, socket),
				);
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
			onDisconnect(reconnectState: ReconnectSchedule) {
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
		};
	}, [
		aiGatewayDisabled,
		chatID,
		initialDataLoaded,
		onDurableMessages,
		queryClient,
		store,
	]);
	return {
		store,
		loadState,
		retryLoad: () => {
			void storeQuery.refetch();
		},
		clearClientError: () => {
			store.clearClientError();
		},
		// The catch drops the error, because hasFetchMoreError reports
		// it.
		fetchMoreMessages: () => fetchMoreMessages().catch(() => undefined),
		isFetchingMoreMessages,
		hasFetchMoreError,
	};
};
