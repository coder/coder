import { useEffect, useEffectEvent } from "react";
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
import type { ChatDetailError } from "./chatError";
import {
	type ChatStore,
	type ChatStoreLoad,
	type ChatStoreState,
	createChatStore,
	isActiveChatStatus,
	selectChatStatus,
	selectIsLoaded,
	selectLoad,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	useChatSelector,
} from "./chatStore";

const shouldSurfaceReconnectState = (state: ChatStoreState): boolean =>
	selectStreamError(state) === null &&
	(selectStreamState(state) !== null ||
		selectRetryState(state) !== null ||
		isActiveChatStatus(selectChatStatus(state)));

export const chatStoreKey = (chatID: string) => ["chatStore", chatID] as const;

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
	load: ChatStoreLoad;
	retryLoad: () => void;
	clearStreamError: () => void;
	loadOlderMessages: () => void;
	isLoadingOlderMessages: boolean;
	hasLoadOlderMessagesError: boolean;
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
	// from the store's snapshot version.
	const { data: store } = useQuery({
		queryKey: chatStoreKey(chatID),
		queryFn: () =>
			queryClient.getQueryData<ChatStore>(chatStoreKey(chatID)) ??
			createChatStore(),
		initialData: createChatStore,
		staleTime: Number.POSITIVE_INFINITY,
		structuralSharing: false,
	});
	const load = useChatSelector(store, selectLoad);
	const loaded = useChatSelector(store, selectIsLoaded);

	// Wrap error-reason callbacks so the WebSocket effect can call
	// them without including them in its dependency array.
	const setChatErrorReasonEvent = useEffectEvent(setChatErrorReason);
	const clearChatErrorReasonEvent = useEffectEvent(clearChatErrorReason);

	const onDurableMessages = useEffectEvent(
		(messages: readonly TypesGen.ChatMessage[]) => {
			// Refresh the dedicated prompt-history cache when a user message arrives.
			if (messages.some((msg) => msg.role === "user")) {
				void invalidateChatPrompts(queryClient, chatID);
			}
			void invalidateChatSearches(queryClient);
		},
	);

	// A store loads its newest page once; afterwards only the stream writes it.
	useEffect(() => {
		store.load(() => fetchNewestChatMessages(chatID));
	}, [chatID, store]);

	const {
		mutate: loadOlderMessages,
		isPending: isLoadingOlderMessages,
		isError: hasLoadOlderMessagesError,
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

		store.resetTransientState();

		if (!loaded || aiGatewayDisabled) {
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

		// The events of the sync in progress. A sync ends with its checkpoint
		// and is applied whole there, so the view never shows part of one. A
		// sync cut off by a disconnect is dropped; the reconnect sends it again.
		const syncEvents: TypesGen.ChatStreamEvent[] = [];

		const schedulePartsFlush = () => {
			if (partsFlushTimer !== null || partsBuf.length === 0) {
				return;
			}
			partsFlushTimer = setTimeout(() => {
				partsFlushTimer = null;
				if (disposed) {
					return;
				}
				store.applyMessageParts(partsBuf.splice(0));
			}, 0);
		};

		// Discard buffered parts without applying them, so parts from before
		// a preview_reset or from a closed socket never reach the store.
		const discardBufferedParts = () => {
			partsBuf.length = 0;
			if (partsFlushTimer !== null) {
				clearTimeout(partsFlushTimer);
				partsFlushTimer = null;
			}
		};

		const applyTurn = (turn: TypesGen.ChatStreamTurn) => {
			updateSidebarChat((chat) =>
				chat.status === turn.status ? chat : { ...chat, status: turn.status },
			);
			const error = store.getSnapshot().turnError;
			if (error) {
				setChatErrorReasonEvent(chatID, error);
			} else {
				clearChatErrorReasonEvent(chatID);
			}
		};

		const applySync = (events: readonly TypesGen.ChatStreamEvent[]) => {
			// Consecutive messages are upserted together: one Map copy and one
			// sort instead of one per message.
			const durable: TypesGen.ChatMessage[] = [];
			const pendingMessages: TypesGen.ChatMessage[] = [];
			const flushMessages = () => {
				if (pendingMessages.length > 0) {
					store.upsertDurableMessages(pendingMessages);
					durable.push(...pendingMessages.splice(0));
				}
			};
			let historyReset = false;
			let queueUpdate = false;
			let userMessage = false;
			let turn: TypesGen.ChatStreamTurn | undefined;

			for (const streamEvent of events) {
				if (streamEvent.type === "message") {
					if (streamEvent.message) {
						pendingMessages.push(streamEvent.message);
						if (streamEvent.message.role === "user") {
							userMessage = true;
						}
					}
					continue;
				}
				flushMessages();

				switch (streamEvent.type) {
					case "history_reset":
						store.resetHistoryFrom(
							streamEvent.history_reset?.from_message_id ?? 0,
						);
						historyReset = true;
						continue;
					case "queue_update":
						store.setQueuedMessages(streamEvent.queued_messages ?? []);
						queueUpdate = true;
						continue;
					case "preview_reset":
						discardBufferedParts();
						store.clearStreamState();
						continue;
					case "checkpoint":
						if (streamEvent.checkpoint) {
							store.applyCheckpoint(streamEvent.checkpoint, {
								userMessage,
								queueUpdate,
								historyReset,
							});
							turn = streamEvent.checkpoint.turn;
						}
						continue;
					default:
						continue;
				}
			}
			flushMessages();

			if (turn) {
				applyTurn(turn);
			}
			if (durable.length > 0) {
				onDurableMessages(durable);
			}
			if (historyReset) {
				void invalidateChatSearches(queryClient);
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
				// A later checkpoint would advance the reconnect version past
				// the lost events, so resync from the last applied one instead.
				syncEvents.length = 0;
				discardBufferedParts();
				store.setStreamError({
					kind: "generic",
					message: "Failed to parse chat stream update.",
				});
				socket.close();
				return;
			}

			// Wrap all store mutations in a batch so subscribers are notified
			// once per frame, not per event.
			store.batch(() => {
				for (const streamEvent of payload.parsedMessage ?? []) {
					if (streamEvent.chat_id && streamEvent.chat_id !== chatID) {
						const nextStatus = streamEvent.status?.status;
						if (streamEvent.type === "status" && nextStatus) {
							store.setSubagentStatusOverride(streamEvent.chat_id, nextStatus);
						}
						continue;
					}
					// The server never sends a part inside a sync.
					if (streamEvent.type === "message_part") {
						const part = streamEvent.message_part?.part;
						if (part) {
							partsBuf.push(part);
						}
						continue;
					}
					syncEvents.push(streamEvent);
					if (streamEvent.type === "checkpoint") {
						applySync(syncEvents.splice(0));
					}
				}

				// Schedule a coalesced flush for any remaining parts. If a sync
				// already flushed or discarded them, this is a no-op.
				schedulePartsFlush();
			});
		};
		const disposeSocket = createReconnectingWebSocket({
			connect() {
				// The server sends only changes made after this version.
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
				// Drop the unfinished sync of the old socket; the new socket
				// sends it again.
				syncEvents.length = 0;
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
	}, [aiGatewayDisabled, chatID, loaded, queryClient, store]);

	return {
		store,
		load,
		retryLoad: () => store.load(() => fetchNewestChatMessages(chatID)),
		clearStreamError: () => {
			store.clearStreamError();
		},
		loadOlderMessages: () => loadOlderMessages(),
		isLoadingOlderMessages,
		hasLoadOlderMessagesError,
	};
};
