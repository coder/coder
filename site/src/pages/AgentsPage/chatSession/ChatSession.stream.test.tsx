import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { watchChat } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import { ChatSession } from "./ChatSession";
import type { ChatSessionRuntimeDeps } from "./types";

vi.mock("#/api/api", () => ({
	watchChat: vi.fn(),
}));

type MessageListener = (
	payload: OneWayMessageEvent<TypesGen.ChatStreamEvent[]>,
) => void;
type ErrorListener = (payload: Event) => void;
type OpenListener = (payload: Event) => void;
type CloseListener = (payload: CloseEvent) => void;
type WatchChatSocket = ReturnType<typeof watchChat>;

type MockSocket = WatchChatSocket & {
	emitOpen: () => void;
	emitData: (event: TypesGen.ChatStreamEvent) => void;
	emitParseError: () => void;
	emitError: () => void;
	emitClose: () => void;
};

const createMockSocket = (): MockSocket => {
	const messageListeners = new Set<MessageListener>();
	const errorListeners = new Set<ErrorListener>();
	const openListeners = new Set<OpenListener>();
	const closeListeners = new Set<CloseListener>();

	const addEventListener = ((
		event: "message" | "error" | "open" | "close",
		callback: MessageListener | ErrorListener | OpenListener | CloseListener,
	): void => {
		if (event === "message") {
			messageListeners.add(callback as MessageListener);
			return;
		}
		if (event === "open") {
			openListeners.add(callback as OpenListener);
			return;
		}
		if (event === "close") {
			closeListeners.add(callback as CloseListener);
			return;
		}
		errorListeners.add(callback as ErrorListener);
	}) as WatchChatSocket["addEventListener"];

	const removeEventListener = ((
		event: "message" | "error" | "open" | "close",
		callback: MessageListener | ErrorListener | OpenListener | CloseListener,
	): void => {
		if (event === "message") {
			messageListeners.delete(callback as MessageListener);
			return;
		}
		if (event === "open") {
			openListeners.delete(callback as OpenListener);
			return;
		}
		if (event === "close") {
			closeListeners.delete(callback as CloseListener);
			return;
		}
		errorListeners.delete(callback as ErrorListener);
	}) as WatchChatSocket["removeEventListener"];

	return {
		url: "ws://example.test/api/experimental/chats/chat-1/stream",
		addEventListener,
		removeEventListener,
		close: vi.fn(),
		emitOpen: () => {
			for (const listener of openListeners) {
				listener(new Event("open"));
			}
		},
		emitData: (event) => {
			const payload: OneWayMessageEvent<TypesGen.ChatStreamEvent[]> = {
				sourceEvent: {} as MessageEvent<string>,
				parseError: undefined,
				parsedMessage: [event],
			};
			for (const listener of messageListeners) {
				listener(payload);
			}
		},
		emitParseError: () => {
			const payload: OneWayMessageEvent<TypesGen.ChatStreamEvent[]> = {
				sourceEvent: {} as MessageEvent<string>,
				parseError: new Error("bad json"),
				parsedMessage: undefined,
			};
			for (const listener of messageListeners) {
				listener(payload);
			}
		},
		emitError: () => {
			for (const listener of errorListeners) {
				listener(new Event("error"));
			}
		},
		emitClose: () => {
			for (const listener of closeListeners) {
				listener(new CloseEvent("close"));
			}
		},
	};
};

const mockWatchChatWithFreshSockets = (): MockSocket[] => {
	const sockets: MockSocket[] = [];
	vi.mocked(watchChat).mockImplementation(() => {
		const socket = createMockSocket();
		sockets.push(socket);
		return socket;
	});
	return sockets;
};

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: 0,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

const makeRuntimeDeps = (
	queryClient = createTestQueryClient(),
): ChatSessionRuntimeDeps => ({
	queryClient,
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
});

const makeSession = (chatId = "chat-1") =>
	new ChatSession(chatId, makeRuntimeDeps());

const makeChat = (
	chatId: string,
	status: TypesGen.ChatStatus = "running",
): TypesGen.Chat => ({
	id: chatId,
	organization_id: "test-org-id",
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	title: "test",
	status,
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
	children: [],
});

const makeMessage = (
	chatId: string,
	id: number,
	role: TypesGen.ChatMessageRole = "assistant",
	text = `message ${id}`,
): TypesGen.ChatMessage => ({
	id,
	chat_id: chatId,
	created_at: `2025-01-01T00:00:00.${String(id).padStart(3, "0")}Z`,
	role,
	content: [{ type: "text", text }],
});

const makeMessagesData = (
	messages: readonly TypesGen.ChatMessage[] = [],
): TypesGen.ChatMessagesResponse => ({
	messages,
	queued_messages: [],
	has_more: false,
});

const hydrate = (
	session: ChatSession,
	messages: readonly TypesGen.ChatMessage[],
	status: TypesGen.ChatStatus = "running",
) => {
	session.hydrateFromRest({
		chatMessages: messages,
		chatRecord: makeChat(session.chatId, status),
		chatMessagesData: makeMessagesData(messages),
		chatQueuedMessages: [],
	});
};

afterEach(() => {
	vi.clearAllTimers();
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe("ChatSession stream lifecycle", () => {
	it("waits for REST hydration before opening a foreground stream", () => {
		mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.enterForeground({ now: 1 });

		expect(watchChat).not.toHaveBeenCalled();
		hydrate(session, [makeMessage("chat-1", 5, "user")]);

		expect(watchChat).toHaveBeenCalledTimes(1);
		expect(watchChat).toHaveBeenLastCalledWith("chat-1", 5);
		expect(session.getSnapshot()).toMatchObject({
			lifecycleMode: "foreground",
			lastVisibleAt: 1,
		});
	});

	it("opens background streams without marking messages as read", () => {
		mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.enterBackgroundNoRead({ now: 2 });
		session.enterForeground({ now: 3 });
		session.enterBackgroundNoRead({ now: 4 });

		expect(watchChat).not.toHaveBeenCalled();
		hydrate(session, [makeMessage("chat-1", 7, "user")]);

		expect(watchChat).toHaveBeenCalledTimes(1);
		expect(watchChat).toHaveBeenLastCalledWith("chat-1", 7, {
			markRead: false,
		});
		expect(session.getSnapshot()).toMatchObject({
			lifecycleMode: "background",
			backgroundedAt: 4,
		});
	});

	it("switches stream modes without resetting retained store state", () => {
		vi.useFakeTimers();
		mockWatchChatWithFreshSockets();
		const session = makeSession();
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground({ now: 1 });
		const firstSocket = vi.mocked(watchChat).mock.results[0]
			.value as MockSocket;
		firstSocket.emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "running" },
		});
		firstSocket.emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "partial" } },
		});
		vi.advanceTimersByTime(0);
		firstSocket.emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 2, "user"),
		});
		session.store.setRetryState({
			attempt: 1,
			error: "retry",
			kind: "generic",
			delayMs: 100,
			retryingAt: "2025-01-01T00:00:01.000Z",
		});
		session.store.setReconnectState({
			attempt: 1,
			delayMs: 1000,
			retryingAt: "2025-01-01T00:00:01.000Z",
		});

		session.enterBackgroundNoRead({ now: 2 });

		expect(firstSocket.close).toHaveBeenCalledTimes(1);
		expect(watchChat).toHaveBeenCalledTimes(2);
		expect(watchChat).toHaveBeenLastCalledWith("chat-1", 2, {
			markRead: false,
		});
		expect(session.store.getSnapshot().messagesByID.has(2)).toBe(true);
		expect(session.store.getSnapshot().streamState).not.toBeNull();
		expect(session.store.getSnapshot().retryState).not.toBeNull();
		expect(session.store.getSnapshot().reconnectState).not.toBeNull();

		firstSocket.emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "completed" },
		});
		firstSocket.emitData({
			type: "queue_update",
			chat_id: "chat-1",
			queued_messages: [],
		});
		firstSocket.emitData({
			type: "error",
			chat_id: "chat-1",
			error: { message: "stale", retryable: false },
		});
		firstSocket.emitClose();

		expect(session.store.getSnapshot().chatStatus).toBe("running");
		expect(session.store.getSnapshot().streamError).toBeNull();
	});

	it("clears transport replay state after an active reconnect opens", () => {
		vi.useFakeTimers();
		vi.spyOn(Math, "random").mockReturnValue(0.5);
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground({ now: 1 });
		sockets[0].emitOpen();
		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "running" },
		});
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "before" } },
		});
		vi.advanceTimersByTime(0);
		session.store.setRetryState({
			attempt: 1,
			error: "retry",
			kind: "generic",
		});

		sockets[0].emitClose();

		expect(session.store.getSnapshot().reconnectState).toMatchObject({
			attempt: 1,
			delayMs: 1000,
		});
		session.store.setStreamError({ kind: "generic", message: "transient" });
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "stale" } },
		});
		vi.advanceTimersByTime(1000);
		expect(sockets).toHaveLength(2);
		sockets[1].emitOpen();

		expect(session.store.getSnapshot().streamState).toBeNull();
		expect(session.store.getSnapshot().retryState).toBeNull();
		expect(session.store.getSnapshot().reconnectState).toBeNull();
		expect(session.store.getSnapshot().streamError).toBeNull();
	});

	it("ignores stale callbacks after disconnect and dispose", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground({ now: 1 });
		session.disconnect();

		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "completed" },
		});
		sockets[0].emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 2, "assistant"),
		});
		sockets[0].emitParseError();
		sockets[0].emitClose();
		vi.advanceTimersByTime(0);

		expect(session.store.getSnapshot().chatStatus).toBe("running");
		expect(session.store.getSnapshot().messagesByID.has(2)).toBe(false);
		expect(session.store.getSnapshot().streamError).toBeNull();
		expect(session.store.getSnapshot().reconnectState).toBeNull();

		session.enterBackgroundNoRead({ now: 2 });
		expect(watchChat).toHaveBeenCalledTimes(2);
		session.dispose();
		sockets[1].emitData({
			type: "retry",
			chat_id: "chat-1",
			retry: {
				attempt: 1,
				delay_ms: 100,
				error: "stale",
				retrying_at: "2025-01-01T00:00:01.000Z",
			},
		});
		expect(session.store.getSnapshot().retryState).toBeNull();
	});
});
