import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "react-query";
import {
	type ChatExecutionOwnershipToken,
	claimChatExecutionOverlay,
	releaseChatExecutionOverlay,
} from "#/api/queries/chatExecutionOverlay";
import {
	type ChatDetailProjection,
	type ChatExecutionSnapshotEvent,
	cancelCachedChat,
	chat,
	getCachedChat,
	invalidateCachedChat,
	invalidateCachedChatPrompts,
	invalidateChatAfterExecutionStreamFailure,
	replaceCachedChatMessages,
	replaceCachedChatQueuedMessages,
	updateCachedChat,
	updateCachedChatExecutionSnapshot,
	updateInfiniteChatsCache,
	upsertCachedChatMessages,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatExecutionReconcilerPorts,
	reconcileChatExecutionSnapshotEvent,
} from "./chatExecutionReconciler";
import {
	type ChatPreviewPart,
	subscribeChatExecutionStream,
} from "./chatExecutionStream";
import {
	type ChatStreamStore,
	type ChatStreamStoreState,
	createChatStreamStore,
	isActiveChatStatus,
} from "./chatStreamStore";
import type { RetryState } from "./types";

const normalizeRetryState = (retry: TypesGen.ChatStreamRetry): RetryState => ({
	attempt: Math.max(1, retry.attempt),
	error: retry.error.trim() || "Retrying request shortly.",
	kind: retry.kind ?? "generic",
	provider: retry.provider?.trim() || undefined,
	delayMs: retry.delay_ms,
	retryingAt: retry.retrying_at.trim() || undefined,
});

const shouldSurfaceReconnectState = (
	state: ChatStreamStoreState,
	status: TypesGen.ChatStatus | null,
): boolean =>
	state.transientError === null &&
	(state.streamState !== null ||
		state.retryState !== null ||
		isActiveChatStatus(status));

interface UseChatStreamStoreOptions {
	chatID: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: ChatDetailProjection | undefined;
	isSharedViewer?: boolean;
	aiGatewayDisabled?: boolean;
}

export const useChatStreamStore = (
	options: UseChatStreamStoreOptions,
): {
	store: ChatStreamStore;
	clearTransientError: () => void;
	upsertCacheMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
} => {
	const {
		chatID,
		chatMessages,
		chatRecord,
		isSharedViewer = false,
		aiGatewayDisabled = false,
	} = options;

	const queryClient = useQueryClient();
	const [store] = useState(createChatStreamStore);
	const nextConnectionEpochRef = useRef(0);
	const activeChatIDRef = useRef<string | null>(null);

	// Compute the last REST-fetched message ID so the stream can
	// skip messages the client already has. We use a ref so the
	// socket effect can read the latest value without including
	// chatMessages in its dependency array (which would cause
	// unnecessary reconnections).
	const lastMessageIdRef = useRef<number | undefined>(undefined);
	useEffect(() => {
		lastMessageIdRef.current = chatMessages?.reduce<number | undefined>(
			(maximum, message) =>
				maximum === undefined ? message.id : Math.max(maximum, message.id),
			undefined,
		);
	});

	// True once the initial REST page has resolved for the current
	// chat. The WebSocket effect gates on this so that
	// lastMessageIdRef is populated before the socket opens;
	// otherwise the server replays the entire message history as
	// its snapshot, defeating pagination.
	const initialDataLoaded =
		chatMessages !== undefined && chatRecord !== undefined;

	const [sharedReaderReady, setSharedReaderReady] = useState(
		() => !isSharedViewer || document.visibilityState === "visible",
	);
	useEffect(() => {
		if (!isSharedViewer || !chatID) {
			setSharedReaderReady(true);
			return;
		}

		let disposed = false;
		let authorizationAttempt = 0;
		const reauthorizeSharedReader = () => {
			if (document.visibilityState !== "visible") {
				setSharedReaderReady(false);
				return;
			}
			const attempt = authorizationAttempt + 1;
			authorizationAttempt = attempt;
			setSharedReaderReady(false);
			void queryClient
				.fetchQuery({ ...chat(chatID), staleTime: 0 })
				.then(() => {
					if (
						!disposed &&
						attempt === authorizationAttempt &&
						document.visibilityState === "visible"
					) {
						setSharedReaderReady(true);
					}
				})
				.catch(() => {
					if (!disposed && attempt === authorizationAttempt) {
						setSharedReaderReady(false);
					}
				});
		};
		const handleVisibilityChange = () => {
			if (document.visibilityState === "hidden") {
				authorizationAttempt += 1;
				setSharedReaderReady(false);
				return;
			}
			reauthorizeSharedReader();
		};
		document.addEventListener("visibilitychange", handleVisibilityChange);
		addEventListener("focus", reauthorizeSharedReader);
		if (document.visibilityState === "hidden") {
			setSharedReaderReady(false);
		}
		return () => {
			disposed = true;
			authorizationAttempt += 1;
			document.removeEventListener("visibilitychange", handleVisibilityChange);
			removeEventListener("focus", reauthorizeSharedReader);
		};
	}, [chatID, isSharedViewer, queryClient]);

	const upsertCacheMessages = useCallback(
		(messages: readonly TypesGen.ChatMessage[]) => {
			if (!chatID || messages.length === 0) {
				return;
			}
			upsertCachedChatMessages(queryClient, chatID, messages);
			if (messages.some((message) => message.role === "user")) {
				void invalidateCachedChatPrompts(queryClient, chatID);
			}
		},
		[chatID, queryClient],
	);

	const replaceCacheMessages = useCallback(
		(messages: readonly TypesGen.ChatMessage[]) => {
			if (chatID) {
				replaceCachedChatMessages(queryClient, chatID, messages);
			}
		},
		[chatID, queryClient],
	);

	useEffect(() => {
		if (chatID && chatRecord && !getCachedChat(queryClient, chatID)) {
			updateCachedChat(queryClient, chatID, () => chatRecord);
		}
	}, [chatID, chatRecord, queryClient]);

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

		if (
			!chatID ||
			!initialDataLoaded ||
			!sharedReaderReady ||
			aiGatewayDisabled
		) {
			return;
		}

		// Capture chatID as a narrowed string for use in closures.
		const activeChatID = chatID;
		// Local disposed flag so the message handler (which lives
		// outside the utility) can bail out after cleanup.
		let disposed = false;
		let executionOwnershipToken: ChatExecutionOwnershipToken | undefined;

		// Parts buffer lives at the effect scope so it persists
		// across WebSocket messages. A rAF-based flush coalesces
		// parts from multiple WS messages into a single render,
		// capping stream renders to once per animation frame.
		const partsBuf: ChatPreviewPart[] = [];
		let partsFlushTimer: ReturnType<typeof setTimeout> | null = null;

		// History replacement state lives at the effect scope because
		// the server may split a history_reset and its replacement
		// messages across multiple WS frames (the stream handler caps
		// frames at a fixed batch size). Replacement messages are
		// buffered until a non-message boundary event arrives; the
		// server always emits preview_reset after a history change in
		// the same sync, so the run is guaranteed to terminate.
		let historyResetPending = false;
		let historyResetInterrupted = false;
		const historyReplacementBuf: TypesGen.ChatMessage[] = [];

		const shouldApplyMessagePart = (): boolean =>
			getCachedChat(queryClient, activeChatID)?.status !== "waiting";

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
				if (store.applyPreviewParts(parts)) {
					store.clearRetryState();
				}
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
			if (store.applyPreviewParts(parts)) {
				store.clearRetryState();
			}
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
			streamEvents: readonly ChatExecutionSnapshotEvent[],
			connectionEpoch: number,
		) => {
			if (disposed) {
				return;
			}
			if (streamEvents.length === 0) {
				return;
			}
			// Collect durable messages for bulk upsert so the
			// entire batch produces one Map copy + one sort
			// instead of N copies and N sorts.
			const pendingMessages: TypesGen.ChatMessage[] = [];
			let hasStagedRetry = false;
			let stagedRetry: RetryState | null = null;
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
				replaceCacheMessages(replacement);
				lastMessageIdRef.current = replacement.reduce<number | undefined>(
					(maximum, message) =>
						maximum === undefined ? message.id : Math.max(maximum, message.id),
					undefined,
				);
			};

			// Wrap all store mutations in a batch so subscribers
			// are notified exactly once at the end, not per event.
			store.batch(() => {
				const reconcilerPorts: ChatExecutionReconcilerPorts = {
					applyPreviewPart: (part) => {
						if (!shouldApplyMessagePart()) return;
						partsBuf.push(part);
					},
					beginHistoryReplacement: () => {
						discardBufferedParts();
						store.clearStreamState();
						historyResetPending = true;
						historyReplacementBuf.length = 0;
						pendingMessages.length = 0;
						needsStreamReset = false;
					},
					resetPreview: () => {
						discardBufferedParts();
						store.clearStreamState();
						store.clearRetryState();
					},
					commitMessage: (message) => {
						hasStagedRetry = true;
						stagedRetry = null;
						store.clearRetryState();
						if (historyResetPending) historyReplacementBuf.push(message);
						else pendingMessages.push(message);
						if (
							message.id !== undefined &&
							(lastMessageIdRef.current === undefined ||
								message.id > lastMessageIdRef.current)
						) {
							lastMessageIdRef.current = message.id;
						}
						if (message.role === "assistant") needsStreamReset = true;
					},
					replaceQueue: (queuedMessages) => {
						replaceCachedChatQueuedMessages(
							queryClient,
							activeChatID,
							queuedMessages,
						);
					},
					applyActionRequired: (actionRequired) => {
						updateCachedChatExecutionSnapshot(queryClient, activeChatID, {
							type: "action_required",
							chat_id: activeChatID,
							action_required: actionRequired,
						});
						hasStagedRetry = true;
						stagedRetry = null;
						store.clearRetryState();
						updateSidebarChat((chat) =>
							chat.status === "requires_action"
								? chat
								: { ...chat, status: "requires_action" },
						);
					},
					applyStatus: (nextStatus) => {
						updateCachedChatExecutionSnapshot(queryClient, activeChatID, {
							type: "status",
							chat_id: activeChatID,
							status: { status: nextStatus },
						});
						hasStagedRetry = true;
						stagedRetry = null;
						store.clearRetryState();
						if (nextStatus === "waiting") discardBufferedParts();
						updateSidebarChat((chat) =>
							chat.status === nextStatus
								? chat
								: { ...chat, status: nextStatus },
						);
					},
					applyError: (error) => {
						if (error) {
							updateCachedChatExecutionSnapshot(queryClient, activeChatID, {
								type: "error",
								chat_id: activeChatID,
								error,
							});
						}
						hasStagedRetry = true;
						stagedRetry = null;
						store.clearRetryState();
						updateSidebarChat((chat) =>
							chat.status === "error" ? chat : { ...chat, status: "error" },
						);
					},
					applyRetry: (retry) => {
						discardBufferedParts();
						store.clearStreamState();
						hasStagedRetry = true;
						stagedRetry = normalizeRetryState(retry);
					},
				};

				for (const streamEvent of streamEvents) {
					if (streamEvent.type === "message_part") {
						commitHistoryReplacement();
					} else if (
						streamEvent.type !== "message" &&
						streamEvent.type !== "history_reset"
					) {
						// Any non-message event marks the end of a replacement
						// run. The server emits replacement messages contiguously
						// after history_reset.
						commitHistoryReplacement();
					}

					// Committed messages must include buffered preview, while
					// errors retain partial output for diagnostics.
					if (streamEvent.type === "message" || streamEvent.type === "error") {
						flushMessageParts();
					}

					reconcileChatExecutionSnapshotEvent({
						event: streamEvent,
						connectionEpoch,
						ports: reconcilerPorts,
					});
				}

				// Schedule a coalesced flush for any remaining
				// parts. If parts were already flushed by a
				// non-message_part event above, this is a no-op.
				schedulePartsFlush();
				if (hasStagedRetry) {
					store.setRetryState(stagedRetry);
				}

				if (pendingMessages.length > 0) {
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
						if (
							shouldApplyMessagePart() &&
							store.applyPreviewParts(nextParts)
						) {
							store.clearRetryState();
						}
					}
				}
			});
		};
		const disposeSocket = subscribeChatExecutionStream({
			chatID: activeChatID,
			getAfterMessageID: () => lastMessageIdRef.current,
			nextConnectionEpoch: () => {
				nextConnectionEpochRef.current += 1;
				return nextConnectionEpochRef.current;
			},
			onBatch: handleMessage,
			onDecodeError() {
				store.setTransientError({
					kind: "generic",
					message: "Failed to parse chat stream update.",
				});
				void invalidateChatAfterExecutionStreamFailure(
					queryClient,
					activeChatID,
				);
			},
			onOpen() {
				executionOwnershipToken = claimChatExecutionOverlay(
					queryClient,
					activeChatID,
				);
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
				// The stream snapshot owns execution catch-up, while REST repairs
				// metadata fields the stream does not carry.
				void invalidateCachedChat(queryClient, activeChatID);
				if (historyResetInterrupted) {
					historyResetInterrupted = false;
					void invalidateChatAfterExecutionStreamFailure(
						queryClient,
						activeChatID,
					);
				}
			},
			onDisconnect(_connectionEpoch, reconnectState) {
				if (historyResetPending) {
					historyResetPending = false;
					historyResetInterrupted = true;
					historyReplacementBuf.length = 0;
				}
				// Only surface reconnecting when the disconnect
				// interrupted active response work. Idle watcher
				// reconnects stay silent.
				const snapshot = store.getSnapshot();
				if (
					shouldSurfaceReconnectState(
						snapshot,
						getCachedChat(queryClient, activeChatID)?.status ?? null,
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
			const token = executionOwnershipToken;
			if (token) {
				void cancelCachedChat(queryClient, activeChatID).finally(() => {
					releaseChatExecutionOverlay(queryClient, activeChatID, token);
				});
			}
			activeChatIDRef.current = null;
		};
	}, [
		aiGatewayDisabled,
		chatID,
		initialDataLoaded,
		queryClient,
		replaceCacheMessages,
		sharedReaderReady,
		store,
		upsertCacheMessages,
	]);
	return {
		store,
		clearTransientError: () => {
			store.clearTransientError();
		},
		upsertCacheMessages,
	};
};
