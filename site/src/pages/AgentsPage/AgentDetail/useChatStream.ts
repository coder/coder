import { watchChat } from "api/api";
import {
	chatDiffContentsKey,
	chatDiffStatusKey,
	chatsKey,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { asRecord, asString } from "components/ai-elements/runtimeTypeUtils";
import {
	startTransition,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { useQueryClient } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { applyMessagePartToStreamState } from "./streamState";
import type { StreamState } from "./types";

const VALID_CHAT_STATUSES: ReadonlySet<string> = new Set<TypesGen.ChatStatus>([
	"pending",
	"running",
	"completed",
	"error",
	"paused",
	"waiting",
]);

/** Narrow an unknown value to a recognised ChatStatus string. */
function isValidChatStatus(value: unknown): value is TypesGen.ChatStatus {
	return typeof value === "string" && VALID_CHAT_STATUSES.has(value);
}

/** Type guard for ChatStreamEvent coming from SSE. */
function isChatStreamEvent(
	data: unknown,
): data is TypesGen.ChatStreamEvent & Record<string, unknown> {
	return (
		typeof data === "object" &&
		data !== null &&
		"type" in data &&
		typeof (data as Record<string, unknown>).type === "string"
	);
}

interface UseChatStreamOptions {
	chatId: string | undefined;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	chatRecord: TypesGen.Chat | undefined;
	chatData: TypesGen.ChatWithMessages | undefined;
	chatQueuedMessages: readonly TypesGen.ChatQueuedMessage[] | undefined;
	setChatErrorReason: (chatId: string, reason: string) => void;
	clearChatErrorReason: (chatId: string) => void;
}

interface UseChatStreamResult {
	messagesById: Map<number, TypesGen.ChatMessage>;
	streamState: StreamState | null;
	chatStatus: TypesGen.ChatStatus | null;
	streamError: string | null;
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	clearStreamError: () => void;
}

export function useChatStream(
	options: UseChatStreamOptions,
): UseChatStreamResult {
	const {
		chatId,
		chatMessages,
		chatRecord,
		chatData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	} = options;

	const queryClient = useQueryClient();

	const [messagesById, setMessagesById] = useState<
		Map<number, TypesGen.ChatMessage>
	>(new Map());
	const messagesByIdRef = useRef<Map<number, TypesGen.ChatMessage>>(new Map());
	const [streamState, setStreamState] = useState<StreamState | null>(null);
	const [streamError, setStreamError] = useState<string | null>(null);
	const [queuedMessages, setQueuedMessages] = useState<
		readonly TypesGen.ChatQueuedMessage[]
	>([]);
	const [chatStatus, setChatStatus] = useState<TypesGen.ChatStatus | null>(
		null,
	);
	const [subagentStatusOverrides, setSubagentStatusOverrides] = useState<
		Map<string, TypesGen.ChatStatus>
	>(new Map());

	const streamResetFrameRef = useRef<number | null>(null);

	const updateSidebarChat = useCallback(
		(updater: (chat: TypesGen.Chat) => TypesGen.Chat) => {
			if (!chatId) {
				return;
			}

			queryClient.setQueryData<readonly TypesGen.Chat[] | undefined>(
				chatsKey,
				(currentChats) => {
					if (!currentChats) {
						return currentChats;
					}

					let didUpdate = false;
					const nextChats = currentChats.map((chat) => {
						if (chat.id !== chatId) {
							return chat;
						}
						didUpdate = true;
						return updater(chat);
					});

					return didUpdate ? nextChats : currentChats;
				},
			);
		},
		[chatId, queryClient],
	);

	const cancelScheduledStreamReset = useCallback(() => {
		if (streamResetFrameRef.current === null) {
			return;
		}
		window.cancelAnimationFrame(streamResetFrameRef.current);
		streamResetFrameRef.current = null;
	}, []);

	const scheduleStreamReset = useCallback(() => {
		cancelScheduledStreamReset();
		streamResetFrameRef.current = window.requestAnimationFrame(() => {
			setStreamState(null);
			streamResetFrameRef.current = null;
		});
	}, [cancelScheduledStreamReset]);

	// Sync chatMessages from query data into local messagesById map.
	useEffect(() => {
		if (!chatMessages) {
			messagesByIdRef.current = new Map();
			setMessagesById(new Map());
			return;
		}
		const nextMessagesByID = new Map(
			chatMessages.map((message) => [message.id, message]),
		);
		messagesByIdRef.current = nextMessagesByID;
		setMessagesById(nextMessagesByID);
	}, [chatMessages]);

	// Sync chatRecord status into local chatStatus state.
	useEffect(() => {
		if (!chatRecord) {
			setChatStatus(null);
			return;
		}
		setChatStatus(chatRecord.status);
	}, [chatRecord]);

	// Sync queued messages from query data into local state.
	useEffect(() => {
		if (!chatData) {
			setQueuedMessages([]);
			return;
		}
		setQueuedMessages(chatQueuedMessages ?? []);
	}, [chatData, chatQueuedMessages]);

	// SSE event handler: watches the chat stream and updates local state.
	useEffect(() => {
		if (!chatId) {
			return;
		}

		cancelScheduledStreamReset();
		setStreamState(null);
		setStreamError(null);
		setSubagentStatusOverrides(new Map());

		const socket = watchChat(chatId);
		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
		) => {
			if (payload.parseError || !payload.parsedMessage) {
				setStreamError("Failed to parse chat stream update.");
				return;
			}
			if (payload.parsedMessage.type !== "data") {
				return;
			}

			const streamEvent = payload.parsedMessage.data;
			if (!isChatStreamEvent(streamEvent)) {
				return;
			}

			switch (streamEvent.type) {
				case "message": {
					const message = streamEvent.message;
					if (!message) {
						return;
					}

					const isDuplicateMessage = messagesByIdRef.current.has(message.id);
					setMessagesById((prev) => {
						if (prev.get(message.id) === message) {
							return prev;
						}
						const next = new Map(prev);
						next.set(message.id, message);
						messagesByIdRef.current = next;
						return next;
					});
					if (!isDuplicateMessage) {
						scheduleStreamReset();
					}
					updateSidebarChat((chat) => ({
						...chat,
						updated_at: message.created_at ?? new Date().toISOString(),
					}));
					void queryClient.invalidateQueries({ queryKey: chatsKey });
					return;
				}
				case "message_part": {
					const part = asRecord(streamEvent.message_part?.part);
					if (!part) {
						return;
					}
					cancelScheduledStreamReset();
					startTransition(() => {
						setStreamState((prev) => applyMessagePartToStreamState(prev, part));
					});
					return;
				}
				case "queue_update": {
					const queuedMsgs = streamEvent.queued_messages;
					setQueuedMessages(queuedMsgs ?? []);
					return;
				}
				case "status": {
					const status = asRecord(streamEvent.status);
					const nextStatus = asString(status?.status);
					if (!isValidChatStatus(nextStatus)) {
						return;
					}

					const eventChatID = asString(streamEvent.chat_id);
					if (eventChatID && eventChatID !== chatId) {
						setSubagentStatusOverrides((prev) => {
							if (prev.get(eventChatID) === nextStatus) {
								return prev;
							}
							const next = new Map(prev);
							next.set(eventChatID, nextStatus);
							return next;
						});
						return;
					}

					setChatStatus(nextStatus);
					if (chatId && nextStatus !== "error") {
						clearChatErrorReason(chatId);
					}
					updateSidebarChat((chat) => ({
						...chat,
						status: nextStatus,
						updated_at: new Date().toISOString(),
					}));
					if (chatId) {
						void Promise.all([
							queryClient.invalidateQueries({
								queryKey: chatDiffStatusKey(chatId),
							}),
							queryClient.invalidateQueries({
								queryKey: chatDiffContentsKey(chatId),
							}),
						]);
					}
					const shouldRefreshQueries =
						nextStatus === "completed" ||
						nextStatus === "error" ||
						nextStatus === "paused" ||
						nextStatus === "waiting";
					if (shouldRefreshQueries) {
						// Hierarchical: chatsKey covers chatKey(chatId).
						void queryClient.invalidateQueries({
							queryKey: chatsKey,
						});
					}
					return;
				}
				case "error": {
					const error = asRecord(streamEvent.error);
					const reason =
						asString(error?.message).trim() || "Chat processing failed.";
					setChatStatus("error");
					setStreamError(reason);
					if (chatId) {
						setChatErrorReason(chatId, reason);
					}
					updateSidebarChat((chat) => ({
						...chat,
						status: "error",
						updated_at: new Date().toISOString(),
					}));
					void queryClient.invalidateQueries({ queryKey: chatsKey });
					return;
				}
				default:
					break;
			}
		};

		const handleError = () => {
			setStreamError((current) => current ?? "Chat stream disconnected.");
			void queryClient.invalidateQueries({ queryKey: chatsKey });
		};

		socket.addEventListener("message", handleMessage);
		socket.addEventListener("error", handleError);

		return () => {
			socket.removeEventListener("message", handleMessage);
			socket.removeEventListener("error", handleError);
			socket.close();
			cancelScheduledStreamReset();
		};
	}, [
		chatId,
		cancelScheduledStreamReset,
		clearChatErrorReason,
		queryClient,
		scheduleStreamReset,
		setChatErrorReason,
		updateSidebarChat,
	]);

	return {
		messagesById,
		streamState,
		chatStatus,
		streamError,
		queuedMessages,
		subagentStatusOverrides,
		clearStreamError: useCallback(() => setStreamError(null), []),
	};
}
