import { watchChats } from "#/api/api";
import type { ChatWatchProjection } from "#/api/queries/chats";
import { ChatWatchEventKinds } from "#/api/typesGenerated";
import type {
	OneWayMessageEvent,
	OneWayWebSocketApi,
} from "#/utils/OneWayWebSocket";
import {
	createReconnectingWebSocket,
	type ReconnectSchedule,
} from "#/utils/reconnectingWebSocket";

type ChatProjectionHint = ChatWatchProjection;

const CHAT_PROJECTION_HINT_KINDS = new Set<string>(ChatWatchEventKinds);

export const decodeChatProjectionHint = (
	value: unknown,
): ChatProjectionHint | Error => {
	if (!value || typeof value !== "object") {
		return new Error("Chat projection hint must be an object.");
	}
	const candidate = value as {
		kind?: unknown;
		chat?: { id?: unknown };
	};
	if (
		typeof candidate.kind !== "string" ||
		!CHAT_PROJECTION_HINT_KINDS.has(candidate.kind)
	) {
		return new Error("Chat projection hint has an unknown kind.");
	}
	if (!candidate.chat || typeof candidate.chat.id !== "string") {
		return new Error("Chat projection hint is missing a chat ID.");
	}
	return value as ChatProjectionHint;
};

type ProjectionHintSocket = OneWayWebSocketApi<ChatWatchProjection>;

type SubscribeChatProjectionHintsOptions = {
	connect?: () => ProjectionHintSocket;
	onHint: (hint: ChatProjectionHint, connectionEpoch: number) => void;
	onOpen?: (connectionEpoch: number) => void;
	onDisconnect?: (
		connectionEpoch: number,
		reconnect: ReconnectSchedule,
	) => void;
	onDecodeError?: (error: Error, connectionEpoch: number) => void;
	baseMs?: number;
	maxMs?: number;
	factor?: number;
	jitter?: number;
	random?: () => number;
};

export const subscribeChatProjectionHints = ({
	connect = watchChats,
	onHint,
	onOpen,
	onDisconnect,
	onDecodeError,
	baseMs,
	maxMs,
	factor,
	jitter,
	random,
}: SubscribeChatProjectionHintsOptions): (() => void) => {
	let disposed = false;
	let activeConnectionEpoch = 0;
	let lastOpenedEpoch = 0;
	const socketEpochs = new WeakMap<object, number>();

	const disposeReconnect = createReconnectingWebSocket({
		connect: () => {
			const connectionEpoch = activeConnectionEpoch + 1;
			activeConnectionEpoch = connectionEpoch;
			const socket = connect();
			socketEpochs.set(socket, connectionEpoch);
			socket.addEventListener(
				"message",
				(payload: OneWayMessageEvent<ChatWatchProjection>) => {
					if (disposed || connectionEpoch !== activeConnectionEpoch) {
						return;
					}
					if (payload.parseError) {
						onDecodeError?.(payload.parseError, connectionEpoch);
						return;
					}
					const decoded = decodeChatProjectionHint(payload.parsedMessage);
					if (decoded instanceof Error) {
						onDecodeError?.(decoded, connectionEpoch);
						return;
					}
					onHint(decoded, connectionEpoch);
				},
			);
			return socket;
		},
		onOpen: (socket) => {
			const connectionEpoch = socketEpochs.get(socket);
			if (
				disposed ||
				connectionEpoch === undefined ||
				connectionEpoch !== activeConnectionEpoch
			) {
				return;
			}
			lastOpenedEpoch = connectionEpoch;
			onOpen?.(connectionEpoch);
		},
		onDisconnect: (reconnect) => {
			if (disposed) {
				return;
			}
			onDisconnect?.(lastOpenedEpoch || activeConnectionEpoch, reconnect);
		},
		baseMs,
		maxMs,
		factor,
		jitter,
		random,
	});

	return () => {
		disposed = true;
		activeConnectionEpoch += 1;
		disposeReconnect();
	};
};

export type DirtyChatProjectionHints = ReadonlyMap<
	string,
	ReadonlySet<ChatProjectionHint["kind"]>
>;

type ChatProjectionHintFreshnessCoordinatorOptions = {
	reconcileLiveHint: (hint: ChatProjectionHint) => void;
	resynchronizeBaseline: () => Promise<void>;
	resynchronizeDirty: (dirty: DirtyChatProjectionHints) => Promise<void>;
	onError?: (error: unknown) => void;
};

/**
 * Coordinates the unversioned global watch with REST metadata projections.
 * Full hint payloads are never replayed after a REST baseline. Only dirty chat
 * IDs and event kinds survive while synchronization is in flight.
 */
export const createChatProjectionHintFreshnessCoordinator = ({
	reconcileLiveHint,
	resynchronizeBaseline,
	resynchronizeDirty,
	onError,
}: ChatProjectionHintFreshnessCoordinatorOptions) => {
	let disposed = false;
	let activeConnectionEpoch = 0;
	let synchronizationGeneration = 0;
	let synchronizing = true;
	let dirtyHints = new Map<string, Set<ChatProjectionHint["kind"]>>();

	const recordDirtyHint = (hint: ChatProjectionHint) => {
		const kinds = dirtyHints.get(hint.chat.id) ?? new Set();
		kinds.add(hint.kind);
		dirtyHints.set(hint.chat.id, kinds);
	};

	const synchronize = async (
		connectionEpoch: number,
		clearDirty: boolean,
	): Promise<void> => {
		const generation = synchronizationGeneration + 1;
		synchronizationGeneration = generation;
		synchronizing = true;
		if (clearDirty) {
			dirtyHints.clear();
		}
		try {
			await resynchronizeBaseline();
			while (
				!disposed &&
				connectionEpoch === activeConnectionEpoch &&
				generation === synchronizationGeneration &&
				dirtyHints.size > 0
			) {
				const dirty = dirtyHints;
				dirtyHints = new Map();
				await resynchronizeDirty(dirty);
			}
			if (
				!disposed &&
				connectionEpoch === activeConnectionEpoch &&
				generation === synchronizationGeneration
			) {
				synchronizing = false;
			}
		} catch (error) {
			if (
				!disposed &&
				connectionEpoch === activeConnectionEpoch &&
				generation === synchronizationGeneration
			) {
				onError?.(error);
			}
		}
	};

	return {
		onOpen(connectionEpoch: number) {
			if (disposed || connectionEpoch < activeConnectionEpoch) {
				return;
			}
			activeConnectionEpoch = connectionEpoch;
			void synchronize(connectionEpoch, true);
		},
		onDisconnect(connectionEpoch: number) {
			if (disposed || connectionEpoch !== activeConnectionEpoch) {
				return;
			}
			synchronizationGeneration += 1;
			synchronizing = true;
			dirtyHints.clear();
		},
		onHint(hint: ChatProjectionHint, connectionEpoch: number) {
			if (disposed || connectionEpoch !== activeConnectionEpoch) {
				return;
			}
			if (synchronizing) {
				recordDirtyHint(hint);
				return;
			}
			reconcileLiveHint(hint);
		},
		onDecodeError(connectionEpoch: number) {
			if (disposed || connectionEpoch !== activeConnectionEpoch) {
				return;
			}
			void synchronize(connectionEpoch, false);
		},
		dispose() {
			disposed = true;
			synchronizationGeneration += 1;
			dirtyHints.clear();
		},
	};
};

const FILTER_MEMBERSHIP_EVENT_KINDS = new Set<ChatProjectionHint["kind"]>([
	"diff_status_change",
	"status_change",
	"action_required",
]);

export const shouldInvalidateFilteredChatList = (
	chat: ChatProjectionHint["chat"],
	eventKind: ChatProjectionHint["kind"],
): boolean =>
	!chat.parent_chat_id && FILTER_MEMBERSHIP_EVENT_KINDS.has(eventKind);

export type ChatProjectionHintReconcilerPorts = {
	getPreviousStatus: (
		chatID: string,
	) => ChatProjectionHint["chat"]["status"] | undefined;
	playChime: (
		previousStatus: ChatProjectionHint["chat"]["status"] | undefined,
		nextStatus: ChatProjectionHint["chat"]["status"],
		chatID: string,
		activeChatID: string | undefined,
	) => void;
	removeDeletedChat: (chat: ChatProjectionHint["chat"]) => void;
	invalidateDiff: (chatID: string) => void;
	cancelListRefetches: () => void;
	hasCachedDetail: (chatID: string) => boolean;
	cancelDetailRefetch: (chatID: string) => void;
	addChild: (chat: ChatProjectionHint["chat"], parentID: string) => void;
	prependRoot: (chat: ChatProjectionHint["chat"]) => void;
	mergeProjection: (
		chat: ChatProjectionHint["chat"],
		eventKind: ChatProjectionHint["kind"],
		activeChatID: string | undefined,
	) => void;
	invalidateCollections: () => void;
	invalidateDetail: (chatID: string) => void;
	repairParent: (parentID: string) => void;
};

export const reconcileChatProjectionHint = ({
	hint,
	activeChatID,
	ports,
}: {
	hint: ChatProjectionHint;
	activeChatID: string | undefined;
	ports: ChatProjectionHintReconcilerPorts;
}): void => {
	const updatedChat = hint.chat;
	if (!updatedChat.parent_chat_id) {
		const previousStatus = ports.getPreviousStatus(updatedChat.id);
		if (previousStatus !== undefined) {
			ports.playChime(
				previousStatus,
				updatedChat.status,
				updatedChat.id,
				activeChatID,
			);
		}
	}

	if (hint.kind === "deleted") {
		ports.removeDeletedChat(updatedChat);
		if (updatedChat.parent_chat_id) {
			ports.repairParent(updatedChat.parent_chat_id);
		}
		return;
	}

	if (hint.kind === "diff_status_change") {
		ports.invalidateDiff(updatedChat.id);
	}

	ports.cancelListRefetches();
	if (ports.hasCachedDetail(updatedChat.id)) {
		ports.cancelDetailRefetch(updatedChat.id);
	}

	if (hint.kind === "created") {
		if (updatedChat.parent_chat_id) {
			ports.addChild(updatedChat, updatedChat.parent_chat_id);
			ports.repairParent(updatedChat.parent_chat_id);
		} else {
			ports.prependRoot(updatedChat);
			ports.invalidateCollections();
		}
		return;
	}

	ports.mergeProjection(updatedChat, hint.kind, activeChatID);
	if (updatedChat.parent_chat_id) {
		ports.repairParent(updatedChat.parent_chat_id);
	}
	if (shouldInvalidateFilteredChatList(updatedChat, hint.kind)) {
		ports.invalidateCollections();
	}
	if (hint.kind === "context_dirty") {
		ports.invalidateDetail(updatedChat.id);
	}
};
