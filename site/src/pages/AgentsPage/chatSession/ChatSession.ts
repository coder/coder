import type { InfiniteData } from "react-query";
import { watchChat } from "#/api/api";
import { chatMessagesKey, updateInfiniteChatsCache } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	OneWayMessageEvent,
	OneWayWebSocketApi,
} from "#/utils/OneWayWebSocket";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import {
	type ChatStoreState,
	chatQueuedMessagesEqualByID,
	createChatStore,
	isActiveChatStatus,
} from "../components/ChatConversation/chatStore";
import { mergeMessagesIntoInfiniteCache } from "../components/ChatConversation/messageCache";
import type { RetryState } from "../components/ChatConversation/types";
import type { ChatDetailError } from "../utils/usageLimitMessage";
import type {
	ChatLifecycleMode,
	ChatSessionManagerRuntimeDeps,
	ChatSessionRuntimeDeps,
	ChatSessionSnapshot,
	ChatViewportAnchor,
	EnterForegroundParams,
	HydrateFromRestParams,
	ReleaseVisibleParams,
	StreamParams,
} from "./types";

type StreamMode = Extract<ChatLifecycleMode, "background" | "foreground">;
type ChatStreamSocket = OneWayWebSocketApi<TypesGen.ChatStreamEvent[]>;

const createInitialSnapshot = (): ChatSessionSnapshot => ({
	lifecycleMode: "inactive",
	followMode: true,
	viewportAnchor: null,
	hasNewOffscreenContent: false,
});

const normalizeChatDetailError = (
	error: TypesGen.ChatStreamError | undefined,
): ChatDetailError => {
	const detail = error?.detail?.trim();
	return {
		message: error?.message.trim() || "Chat processing failed.",
		kind: error?.kind?.trim() || "generic",
		provider: error?.provider?.trim() || undefined,
		retryable: error?.retryable,
		statusCode: error?.status_code,
		...(detail ? { detail } : {}),
	};
};

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

const viewportAnchorsEqual = (
	left: ChatViewportAnchor | null,
	right: ChatViewportAnchor | null,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.messageId === right.messageId &&
		left.offsetTop === right.offsetTop &&
		left.newestMessageIdAtCapture === right.newestMessageIdAtCapture
	);
};

const latestMessageId = (
	messages: readonly TypesGen.ChatMessage[],
): number | undefined => messages[messages.length - 1]?.id;

const messagesChangedByReference = (
	left: readonly TypesGen.ChatMessage[],
	right: readonly TypesGen.ChatMessage[],
): boolean =>
	left.length !== right.length ||
	left.some((message, index) => message !== right[index]);

export class ChatSession {
	public readonly store = createChatStore();

	private socketDisposer: (() => void) | null = null;

	private activeStreamMode: StreamMode | null = null;

	private disposed = false;

	private connectionGeneration = 0;

	private reconnectPending = false;

	private wsStatusAuthority = false;

	private wsQueueAuthorityChatId: string | null = null;

	private queuedMessagesHydratedChatId: string | null = null;

	private lastSyncedMessages: readonly TypesGen.ChatMessage[] = [];

	private lastMessageId: number | undefined;

	private initialRestHydrationComplete = false;

	private pendingStreamRequest: StreamParams | null = null;

	private readonly partsBuffer: TypesGen.ChatMessagePart[] = [];

	private partsFlushTimer: ReturnType<typeof setTimeout> | null = null;

	private snapshot = createInitialSnapshot();

	private readonly metadataListeners = new Set<() => void>();

	private retentionTimer: ReturnType<typeof setTimeout> | null = null;

	public constructor(
		public readonly chatId: string,
		private deps: ChatSessionRuntimeDeps,
	) {}

	public getSnapshot(): ChatSessionSnapshot {
		return this.snapshot;
	}

	public subscribe(listener: () => void): () => void {
		this.metadataListeners.add(listener);
		return () => {
			this.metadataListeners.delete(listener);
		};
	}

	public setFollowMode(followMode: boolean): void {
		if (this.snapshot.followMode === followMode) {
			return;
		}
		this.replaceSnapshot({ ...this.snapshot, followMode });
	}

	public setViewportAnchor(viewportAnchor: ChatViewportAnchor | null): void {
		if (viewportAnchorsEqual(this.snapshot.viewportAnchor, viewportAnchor)) {
			return;
		}
		this.replaceSnapshot({ ...this.snapshot, viewportAnchor });
	}

	public markNewOffscreenContent(): void {
		if (this.snapshot.hasNewOffscreenContent) {
			return;
		}
		this.replaceSnapshot({
			...this.snapshot,
			hasNewOffscreenContent: true,
		});
	}

	public clearNewOffscreenContent(): void {
		if (!this.snapshot.hasNewOffscreenContent) {
			return;
		}
		this.replaceSnapshot({
			...this.snapshot,
			hasNewOffscreenContent: false,
		});
	}

	public enterForeground(params: EnterForegroundParams = {}): void {
		const now = params.now ?? Date.now();
		const { backgroundedAt: _backgroundedAt, ...nextSnapshot } = this.snapshot;
		const next: ChatSessionSnapshot = {
			...nextSnapshot,
			lifecycleMode: "foreground",
			lastVisibleAt: now,
		};
		if (
			this.snapshot.lifecycleMode !== next.lifecycleMode ||
			this.snapshot.lastVisibleAt !== next.lastVisibleAt ||
			"backgroundedAt" in this.snapshot
		) {
			this.replaceSnapshot(next);
		}
		this.openStream({
			mode: "foreground",
			markRead: true,
			intentionalModeSwitch: true,
		});
	}

	public enterBackgroundNoRead(params: ReleaseVisibleParams = {}): void {
		const now = params.now ?? Date.now();
		const next: ChatSessionSnapshot = {
			...this.snapshot,
			lifecycleMode: "background",
			backgroundedAt: now,
		};
		if (
			this.snapshot.lifecycleMode !== next.lifecycleMode ||
			this.snapshot.backgroundedAt !== next.backgroundedAt
		) {
			this.replaceSnapshot(next);
		}
		this.openStream({
			mode: "background",
			markRead: false,
			intentionalModeSwitch: true,
		});
	}

	public disconnect(): void {
		this.pendingStreamRequest = null;
		this.cancelPartsFlush();
		this.partsBuffer.length = 0;
		this.connectionGeneration += 1;
		this.disposeActiveSocket();
		this.activeStreamMode = null;
		this.reconnectPending = false;
		if (this.snapshot.lifecycleMode !== "inactive") {
			const { backgroundedAt: _backgroundedAt, ...nextSnapshot } =
				this.snapshot;
			this.replaceSnapshot({
				...nextSnapshot,
				lifecycleMode: "inactive",
			});
		}
	}

	public dispose(): void {
		if (this.disposed) {
			return;
		}
		this.disposed = true;
		this.pendingStreamRequest = null;
		this.cancelPartsFlush();
		this.partsBuffer.length = 0;
		this.connectionGeneration += 1;
		this.disposeActiveSocket();
		this.activeStreamMode = null;
		this.reconnectPending = false;
		if (this.retentionTimer !== null) {
			clearTimeout(this.retentionTimer);
			this.retentionTimer = null;
		}
		this.metadataListeners.clear();
	}

	public hydrateFromRest(params: HydrateFromRestParams): void {
		const hasResolvedMessages = params.chatMessages !== undefined;
		this.store.batch(() => {
			if (params.chatMessages !== undefined) {
				this.hydrateMessagesFromRest(params.chatMessages);
			}
			if (!this.wsStatusAuthority) {
				this.store.setChatStatus(params.chatRecord?.status ?? null);
			}
			const wsQueueBlocksRest =
				this.wsQueueAuthorityChatId === this.chatId &&
				this.queuedMessagesHydratedChatId === this.chatId;
			if (params.chatMessagesData && !wsQueueBlocksRest) {
				this.queuedMessagesHydratedChatId = this.chatId;
				this.store.setQueuedMessages(params.chatQueuedMessages);
			}
		});

		if (hasResolvedMessages) {
			this.initialRestHydrationComplete = true;
			const pending = this.pendingStreamRequest;
			if (pending) {
				this.pendingStreamRequest = null;
				this.openStream(pending);
			}
		}
	}

	public clearStreamError(): void {
		this.store.clearStreamError();
		this.deps.clearChatErrorReason(this.chatId);
	}

	public upsertCacheMessages(messages: readonly TypesGen.ChatMessage[]): void {
		this.deps.queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatMessagesKey(this.chatId), (currentData) =>
			mergeMessagesIntoInfiniteCache(currentData, messages),
		);
	}

	public setRetentionTimer(handle: ReturnType<typeof setTimeout> | null): void {
		if (this.retentionTimer !== null) {
			clearTimeout(this.retentionTimer);
		}
		this.retentionTimer = handle;
	}

	public updateRuntimeDeps(next: ChatSessionManagerRuntimeDeps): void {
		this.deps = next;
	}

	private hydrateMessagesFromRest(
		chatMessages: readonly TypesGen.ChatMessage[],
	): void {
		const prev = this.lastSyncedMessages;
		const contentChanged = messagesChangedByReference(chatMessages, prev);
		const fetchedIds = new Set(chatMessages.map((message) => message.id));
		const prevIds = new Set(prev.map((message) => message.id));
		const storeSnapshot = this.store.getSnapshot();
		const hasStaleEntries =
			contentChanged &&
			storeSnapshot.orderedMessageIDs.some(
				(id) => !fetchedIds.has(id) && prevIds.has(id),
			);
		this.lastSyncedMessages = chatMessages;

		const restLatestMessageId = latestMessageId(chatMessages);
		if (hasStaleEntries) {
			this.store.replaceMessages(chatMessages);
			this.lastMessageId = restLatestMessageId;
			return;
		}

		this.store.upsertDurableMessages(chatMessages);
		if (
			restLatestMessageId !== undefined &&
			(this.lastMessageId === undefined ||
				restLatestMessageId > this.lastMessageId)
		) {
			this.lastMessageId = restLatestMessageId;
		}
	}

	private openStream(params: StreamParams): void {
		if (this.disposed) {
			return;
		}
		if (!this.initialRestHydrationComplete) {
			this.pendingStreamRequest = params;
			return;
		}
		if (this.activeStreamMode === params.mode) {
			return;
		}

		if (this.socketDisposer) {
			this.flushMessageParts();
			this.connectionGeneration += 1;
			this.disposeActiveSocket();
			if (params.intentionalModeSwitch) {
				this.reconnectPending = false;
			}
		}

		this.activeStreamMode = params.mode;
		const generation = this.connectionGeneration;
		this.socketDisposer = createReconnectingWebSocket({
			connect: () => {
				const socket = this.watchStream(params);
				socket.addEventListener("message", (payload) => {
					this.handleSocketMessage(generation, payload);
				});
				return socket;
			},
			onOpen: () => {
				if (!this.isActiveGeneration(generation)) {
					return;
				}
				if (!this.reconnectPending) {
					return;
				}
				this.store.batch(() => {
					this.store.resetTransportReplayState();
					this.store.clearRetryState();
				});
				this.discardBufferedParts();
				this.reconnectPending = false;
			},
			onDisconnect: (reconnect) => {
				if (!this.isActiveGeneration(generation) || !this.activeStreamMode) {
					return;
				}
				this.reconnectPending = true;
				const snapshot = this.store.getSnapshot();
				if (shouldSurfaceReconnectState(snapshot)) {
					this.store.setReconnectState(reconnect);
				}
			},
		});
	}

	private watchStream(params: StreamParams): ChatStreamSocket {
		if (params.markRead) {
			return watchChat(this.chatId, this.lastMessageId);
		}
		return watchChat(this.chatId, this.lastMessageId, { markRead: false });
	}

	private handleSocketMessage(
		generation: number,
		payload: OneWayMessageEvent<TypesGen.ChatStreamEvent[]>,
	): void {
		if (!this.isActiveGeneration(generation)) {
			return;
		}
		if (payload.parseError || !payload.parsedMessage) {
			this.store.setStreamError({
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

		this.store.batch(() => {
			for (const streamEvent of streamEvents) {
				switch (streamEvent.type) {
					case "message_part": {
						this.handleMessagePartEvent(streamEvent);
						continue;
					}
					case "message": {
						const message = streamEvent.message;
						if (
							!message ||
							(streamEvent.chat_id && streamEvent.chat_id !== this.chatId)
						) {
							continue;
						}
						this.flushMessageParts();
						this.store.clearRetryState();
						this.maybeMarkNewDurableOffscreenContent(message.id);
						pendingMessages.push(message);
						this.advanceLastMessageId(message.id);
						if (message.role === "assistant") {
							needsStreamReset = true;
						}
						continue;
					}
					case "queue_update": {
						if (streamEvent.chat_id && streamEvent.chat_id !== this.chatId) {
							continue;
						}
						this.wsQueueAuthorityChatId = this.chatId;
						this.queuedMessagesHydratedChatId = this.chatId;
						this.store.setQueuedMessages(streamEvent.queued_messages);
						this.updateChatQueuedMessages(streamEvent.queued_messages);
						continue;
					}
					case "status": {
						this.handleStatusEvent(streamEvent);
						continue;
					}
					case "error": {
						this.handleErrorEvent(streamEvent);
						continue;
					}
					case "retry": {
						this.handleRetryEvent(streamEvent);
						continue;
					}
					default:
						continue;
				}
			}

			this.schedulePartsFlush(generation);
			if (pendingMessages.length > 0) {
				this.store.upsertDurableMessages(pendingMessages);
				this.upsertCacheMessages(pendingMessages);
			}
			if (needsStreamReset) {
				this.store.clearStreamState();
				this.flushNextTurnParts();
			}
		});
	}

	private handleMessagePartEvent(streamEvent: TypesGen.ChatStreamEvent): void {
		if (streamEvent.chat_id && streamEvent.chat_id !== this.chatId) {
			return;
		}
		if (!this.shouldApplyMessagePart()) {
			return;
		}
		const part = streamEvent.message_part?.part;
		if (!part) {
			return;
		}
		this.maybeMarkStreamPartOffscreenContent();
		this.store.clearRetryState();
		this.partsBuffer.push(part);
	}

	private maybeMarkNewDurableOffscreenContent(
		messageId: number | undefined,
	): void {
		if (this.snapshot.followMode !== false) {
			return;
		}
		const captureId = this.snapshot.viewportAnchor?.newestMessageIdAtCapture;
		if (captureId === undefined) {
			return;
		}
		if (messageId === undefined) {
			return;
		}
		if (messageId <= captureId) {
			return;
		}
		if (this.snapshot.hasNewOffscreenContent) {
			return;
		}
		this.markNewOffscreenContent();
	}

	private maybeMarkStreamPartOffscreenContent(): void {
		if (this.snapshot.followMode !== false) {
			return;
		}
		if (this.snapshot.hasNewOffscreenContent) {
			return;
		}
		this.markNewOffscreenContent();
	}

	private handleStatusEvent(streamEvent: TypesGen.ChatStreamEvent): void {
		const nextStatus = streamEvent.status?.status;
		if (!nextStatus) {
			return;
		}
		if (streamEvent.chat_id && streamEvent.chat_id !== this.chatId) {
			this.store.setSubagentStatusOverride(streamEvent.chat_id, nextStatus);
			return;
		}

		this.wsStatusAuthority = true;
		this.store.clearRetryState();
		this.store.setChatStatus(nextStatus);
		if (nextStatus === "pending" || nextStatus === "waiting") {
			this.discardBufferedParts();
			this.store.clearStreamState();
			this.store.clearRetryState();
		}
		if (nextStatus !== "error") {
			this.deps.clearChatErrorReason(this.chatId);
		}
		this.updateSidebarChat((chat) =>
			chat.status === nextStatus ? chat : { ...chat, status: nextStatus },
		);
	}

	private handleErrorEvent(streamEvent: TypesGen.ChatStreamEvent): void {
		if (streamEvent.chat_id && streamEvent.chat_id !== this.chatId) {
			return;
		}
		this.flushMessageParts();
		const reason = normalizeChatDetailError(streamEvent.error);
		this.wsStatusAuthority = true;
		this.store.setChatStatus("error");
		this.store.setStreamError(reason);
		this.store.clearRetryState();
		this.deps.setChatErrorReason(this.chatId, reason.message);
		this.updateSidebarChat((chat) =>
			chat.status === "error" ? chat : { ...chat, status: "error" },
		);
	}

	private handleRetryEvent(streamEvent: TypesGen.ChatStreamEvent): void {
		if (streamEvent.chat_id && streamEvent.chat_id !== this.chatId) {
			return;
		}
		const retry = streamEvent.retry;
		if (!retry) {
			return;
		}
		this.discardBufferedParts();
		this.store.clearStreamState();
		this.store.setRetryState(normalizeRetryState(retry));
	}

	private schedulePartsFlush(generation: number): void {
		if (this.partsFlushTimer !== null || this.partsBuffer.length === 0) {
			return;
		}
		this.partsFlushTimer = setTimeout(() => {
			this.partsFlushTimer = null;
			if (!this.isActiveGeneration(generation)) {
				return;
			}
			const parts = this.partsBuffer.splice(0);
			if (parts.length === 0 || !this.shouldApplyMessagePart()) {
				return;
			}
			this.store.applyMessageParts(parts);
		}, 0);
	}

	private flushMessageParts(): void {
		if (this.partsBuffer.length === 0) {
			return;
		}
		this.cancelPartsFlush();
		const parts = this.partsBuffer.splice(0);
		if (!this.shouldApplyMessagePart()) {
			return;
		}
		this.store.applyMessageParts(parts);
	}

	private flushNextTurnParts(): void {
		if (this.partsBuffer.length === 0) {
			return;
		}
		this.cancelPartsFlush();
		const parts = this.partsBuffer.splice(0);
		if (this.shouldApplyMessagePart()) {
			this.store.applyMessageParts(parts);
		}
	}

	private discardBufferedParts(): void {
		this.partsBuffer.length = 0;
		this.cancelPartsFlush();
	}

	private cancelPartsFlush(): void {
		if (this.partsFlushTimer !== null) {
			clearTimeout(this.partsFlushTimer);
			this.partsFlushTimer = null;
		}
	}

	private shouldApplyMessagePart(): boolean {
		const currentStatus = this.store.getSnapshot().chatStatus;
		return currentStatus !== "pending" && currentStatus !== "waiting";
	}

	private updateSidebarChat(
		updater: (chat: TypesGen.Chat) => TypesGen.Chat,
	): void {
		updateInfiniteChatsCache(this.deps.queryClient, (chats) => {
			let didUpdate = false;
			const nextChats = chats.map((chat) => {
				if (chat.id !== this.chatId) {
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
	}

	private updateChatQueuedMessages(
		queuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined,
	): void {
		const nextQueuedMessages = queuedMessages ?? [];
		this.deps.queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatMessagesKey(this.chatId), (currentData) => {
			if (!currentData?.pages?.length) {
				return currentData;
			}
			const firstPage = currentData.pages[0];
			if (!firstPage) {
				return currentData;
			}
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
	}

	private replaceSnapshot(next: ChatSessionSnapshot): void {
		this.snapshot = next;
		for (const listener of this.metadataListeners) {
			listener();
		}
	}

	private disposeActiveSocket(): void {
		const disposeSocket = this.socketDisposer;
		this.socketDisposer = null;
		if (disposeSocket) {
			disposeSocket();
		}
	}

	private isActiveGeneration(generation: number): boolean {
		return (
			!this.disposed &&
			this.activeStreamMode !== null &&
			generation === this.connectionGeneration
		);
	}

	private advanceLastMessageId(messageId: number | undefined): void {
		if (
			messageId !== undefined &&
			(this.lastMessageId === undefined || messageId > this.lastMessageId)
		) {
			this.lastMessageId = messageId;
		}
	}
}
