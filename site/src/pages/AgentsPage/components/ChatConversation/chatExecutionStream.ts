import { watchChat } from "#/api/api";
import type { ChatExecutionSnapshotEvent } from "#/api/queries/chats";
import type {
	OneWayMessageEvent,
	OneWayWebSocketApi,
} from "#/utils/OneWayWebSocket";
import {
	createReconnectingWebSocket,
	type ReconnectSchedule,
} from "#/utils/reconnectingWebSocket";

export type ChatPreviewPart = Readonly<{
	connectionEpoch: number;
	historyVersion?: number;
	generationAttempt?: number;
	seq?: number;
	role?: ChatExecutionSnapshotEvent["message_part"] extends
		| { role?: infer Role }
		| undefined
		? Role
		: never;
	part: NonNullable<ChatExecutionSnapshotEvent["message_part"]>["part"];
}>;

type ChatExecutionSocket = OneWayWebSocketApi<ChatExecutionSnapshotEvent[]>;

type SubscribeChatExecutionStreamOptions = {
	chatID: string;
	getAfterMessageID: () => number | undefined;
	connect?: (chatID: string, afterMessageID?: number) => ChatExecutionSocket;
	nextConnectionEpoch?: () => number;
	onBatch: (
		events: readonly ChatExecutionSnapshotEvent[],
		connectionEpoch: number,
	) => void;
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

const CHAT_EXECUTION_EVENT_TYPES = new Set<ChatExecutionSnapshotEvent["type"]>([
	"action_required",
	"error",
	"history_reset",
	"message",
	"message_part",
	"preview_reset",
	"queue_update",
	"retry",
	"status",
]);

const hasRequiredChatExecutionPayload = (
	event: ChatExecutionSnapshotEvent,
): boolean => {
	switch (event.type) {
		case "message":
			return event.message !== undefined;
		case "message_part":
			return event.message_part !== undefined;
		case "status":
			return event.status !== undefined;
		case "error":
			return event.error !== undefined;
		case "retry":
			return event.retry !== undefined;
		case "queue_update":
			return event.queued_messages !== undefined;
		case "action_required":
			return event.action_required !== undefined;
		case "history_reset":
		case "preview_reset":
			return true;
	}
};

const decodeChatExecutionBatch = (
	value: unknown,
	chatID: string,
): readonly ChatExecutionSnapshotEvent[] | Error => {
	if (!Array.isArray(value)) {
		return new Error("Chat execution stream payload must be an array.");
	}
	const events: ChatExecutionSnapshotEvent[] = [];
	for (const event of value) {
		if (!event || typeof event !== "object") {
			return new Error("Chat execution stream event must be an object.");
		}
		const candidate = event as { type?: unknown; chat_id?: unknown };
		if (
			typeof candidate.type !== "string" ||
			!CHAT_EXECUTION_EVENT_TYPES.has(
				candidate.type as ChatExecutionSnapshotEvent["type"],
			)
		) {
			return new Error("Chat execution stream event has an unknown type.");
		}
		if (candidate.chat_id !== chatID) {
			continue;
		}
		const typedEvent = event as ChatExecutionSnapshotEvent;
		if (!hasRequiredChatExecutionPayload(typedEvent)) {
			return new Error(
				`Chat execution stream ${typedEvent.type} event is missing its payload.`,
			);
		}
		events.push(typedEvent);
	}
	return events;
};

export const subscribeChatExecutionStream = ({
	chatID,
	getAfterMessageID,
	connect = watchChat,
	nextConnectionEpoch,
	onBatch,
	onOpen,
	onDisconnect,
	onDecodeError,
	baseMs,
	maxMs,
	factor,
	jitter,
	random,
}: SubscribeChatExecutionStreamOptions): (() => void) => {
	let disposed = false;
	let activeConnectionEpoch = 0;
	let lastOpenedEpoch = 0;
	const damagedConnectionEpochs = new Set<number>();
	const socketEpochs = new WeakMap<object, number>();

	const disposeReconnect = createReconnectingWebSocket({
		connect: () => {
			const connectionEpoch =
				nextConnectionEpoch?.() ?? activeConnectionEpoch + 1;
			activeConnectionEpoch = connectionEpoch;
			const socket = connect(chatID, getAfterMessageID());
			socketEpochs.set(socket, connectionEpoch);
			socket.addEventListener(
				"message",
				(payload: OneWayMessageEvent<ChatExecutionSnapshotEvent[]>) => {
					if (
						disposed ||
						connectionEpoch !== activeConnectionEpoch ||
						damagedConnectionEpochs.has(connectionEpoch)
					) {
						return;
					}
					if (payload.parseError) {
						damagedConnectionEpochs.add(connectionEpoch);
						onDecodeError?.(payload.parseError, connectionEpoch);
						socket.close();
						return;
					}
					const decoded = decodeChatExecutionBatch(
						payload.parsedMessage,
						chatID,
					);
					if (decoded instanceof Error) {
						damagedConnectionEpochs.add(connectionEpoch);
						onDecodeError?.(decoded, connectionEpoch);
						socket.close();
						return;
					}
					if (decoded.length > 0) {
						onBatch(decoded, connectionEpoch);
					}
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

export const toChatPreviewPart = (
	event: ChatExecutionSnapshotEvent,
	connectionEpoch: number,
): ChatPreviewPart | undefined => {
	if (event.type !== "message_part" || !event.message_part) {
		return undefined;
	}
	return {
		connectionEpoch,
		historyVersion: event.message_part.history_version,
		generationAttempt: event.message_part.generation_attempt,
		seq: event.message_part.seq,
		role: event.message_part.role,
		part: event.message_part.part,
	};
};
