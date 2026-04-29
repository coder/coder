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

const makeQueuedMessage = (
	chatId: string,
	id: number,
): TypesGen.ChatQueuedMessage => ({
	id,
	chat_id: chatId,
	created_at: `2025-01-01T00:00:00.${String(id).padStart(3, "0")}Z`,
	content: [{ type: "text", text: `queued ${id}` }],
});

const makeMessagesData = (
	messages: readonly TypesGen.ChatMessage[] = [],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
): TypesGen.ChatMessagesResponse => ({
	messages,
	queued_messages: queuedMessages,
	has_more: false,
});

const startForegroundStream = (
	session: ChatSession,
	sockets: readonly MockSocket[],
): MockSocket => {
	session.hydrateFromRest({
		chatMessages: [],
		chatRecord: makeChat(session.chatId, "running"),
		chatMessagesData: makeMessagesData(),
		chatQueuedMessages: [],
	});
	session.enterForeground({ now: 1 });
	const socket = sockets[0];
	if (!socket) {
		throw new Error("Expected a stream socket to be created.");
	}
	socket.emitOpen();
	return socket;
};

afterEach(() => {
	vi.clearAllTimers();
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe("ChatSession metadata snapshot", () => {
	it("starts with the default metadata snapshot", () => {
		const session = makeSession();

		expect(session.getSnapshot()).toEqual({
			lifecycleMode: "inactive",
			followMode: true,
			viewportAnchor: null,
			hasNewOffscreenContent: false,
		});
		expect("backgroundedAt" in session.getSnapshot()).toBe(false);
	});

	it("notifies metadata subscribers when any viewport anchor field changes", () => {
		const session = makeSession();
		const snapshots: ReturnType<typeof session.getSnapshot>[] = [];
		session.subscribe(() => {
			snapshots.push(session.getSnapshot());
		});

		session.setViewportAnchor({
			messageId: 1,
			offsetTop: 10,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(1);
		const anchoredSnapshot = session.getSnapshot();

		session.setViewportAnchor({
			messageId: 1,
			offsetTop: 10,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(1);
		expect(session.getSnapshot()).toBe(anchoredSnapshot);

		session.setViewportAnchor({
			messageId: 2,
			offsetTop: 10,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(2);

		session.setViewportAnchor({
			messageId: 2,
			offsetTop: 11,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(3);

		session.setViewportAnchor({
			messageId: 2,
			offsetTop: 11,
			newestMessageIdAtCapture: 2,
		});
		expect(snapshots).toHaveLength(4);
	});

	it("notifies metadata subscribers only on semantic metadata changes", () => {
		const session = makeSession();
		const initialSnapshot = session.getSnapshot();
		const snapshots: ReturnType<typeof session.getSnapshot>[] = [];
		const unsubscribe = session.subscribe(() => {
			snapshots.push(session.getSnapshot());
		});

		session.setFollowMode(true);
		expect(snapshots).toHaveLength(0);
		expect(session.getSnapshot()).toBe(initialSnapshot);

		session.setFollowMode(false);
		expect(snapshots).toHaveLength(1);
		expect(snapshots[0]).not.toBe(initialSnapshot);
		expect(snapshots[0].followMode).toBe(false);

		session.setViewportAnchor({
			messageId: 1,
			offsetTop: 10,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(2);
		const anchoredSnapshot = session.getSnapshot();
		session.setViewportAnchor({
			messageId: 1,
			offsetTop: 10,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(2);
		expect(session.getSnapshot()).toBe(anchoredSnapshot);

		session.setViewportAnchor({
			messageId: 1,
			offsetTop: 11,
			newestMessageIdAtCapture: undefined,
		});
		expect(snapshots).toHaveLength(3);

		session.markNewOffscreenContent();
		expect(snapshots).toHaveLength(4);
		expect(session.getSnapshot().hasNewOffscreenContent).toBe(true);
		session.markNewOffscreenContent();
		expect(snapshots).toHaveLength(4);

		session.clearNewOffscreenContent();
		expect(snapshots).toHaveLength(5);
		expect(session.getSnapshot().hasNewOffscreenContent).toBe(false);
		session.clearNewOffscreenContent();
		expect(snapshots).toHaveLength(5);

		session.enterForeground({ now: 100 });
		expect(snapshots).toHaveLength(6);
		expect(session.getSnapshot()).toMatchObject({
			lifecycleMode: "foreground",
			lastVisibleAt: 100,
		});
		expect("backgroundedAt" in session.getSnapshot()).toBe(false);
		session.enterForeground({ now: 100 });
		expect(snapshots).toHaveLength(6);

		session.enterBackgroundNoRead({ now: 200 });
		expect(snapshots).toHaveLength(7);
		expect(session.getSnapshot()).toMatchObject({
			lifecycleMode: "background",
			backgroundedAt: 200,
		});
		session.enterBackgroundNoRead({ now: 200 });
		expect(snapshots).toHaveLength(7);

		session.disconnect();
		expect(snapshots).toHaveLength(8);
		expect(session.getSnapshot().lifecycleMode).toBe("inactive");

		unsubscribe();
		session.setFollowMode(true);
		expect(snapshots).toHaveLength(8);
	});

	it("marks new offscreen content once for durable messages newer than the captured viewport", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.setFollowMode(false);
		session.setViewportAnchor({
			messageId: 10,
			offsetTop: 100,
			newestMessageIdAtCapture: 10,
		});
		const socket = startForegroundStream(session, sockets);
		const listener = vi.fn();
		session.subscribe(listener);

		socket.emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 11, "user"),
		});

		expect(session.getSnapshot().hasNewOffscreenContent).toBe(true);
		expect(listener).toHaveBeenCalledTimes(1);

		socket.emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 12, "user"),
		});
		expect(listener).toHaveBeenCalledTimes(1);
	});

	it("does not mark new offscreen content for durable messages at or before the captured viewport", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.setFollowMode(false);
		session.setViewportAnchor({
			messageId: 10,
			offsetTop: 100,
			newestMessageIdAtCapture: 10,
		});
		const socket = startForegroundStream(session, sockets);
		const listener = vi.fn();
		session.subscribe(listener);

		for (const id of [9, 10]) {
			socket.emitData({
				type: "message",
				chat_id: "chat-1",
				message: makeMessage("chat-1", id, "user"),
			});
		}

		expect(session.getSnapshot().hasNewOffscreenContent).toBe(false);
		expect(listener).not.toHaveBeenCalled();
	});

	it("does not mark new offscreen content for durable messages while in follow mode", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.setViewportAnchor({
			messageId: 10,
			offsetTop: 100,
			newestMessageIdAtCapture: 10,
		});
		const socket = startForegroundStream(session, sockets);
		const listener = vi.fn();
		session.subscribe(listener);

		socket.emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 11, "user"),
		});

		expect(session.getSnapshot().hasNewOffscreenContent).toBe(false);
		expect(listener).not.toHaveBeenCalled();
	});

	it("does not mark new offscreen content for durable messages without a viewport anchor", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.setFollowMode(false);
		const socket = startForegroundStream(session, sockets);
		const listener = vi.fn();
		session.subscribe(listener);

		socket.emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 11, "user"),
		});

		expect(session.getSnapshot().hasNewOffscreenContent).toBe(false);
		expect(listener).not.toHaveBeenCalled();
	});

	it("marks new offscreen content once for stream parts outside follow mode", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		session.setFollowMode(false);
		const socket = startForegroundStream(session, sockets);
		const listener = vi.fn();
		session.subscribe(listener);

		socket.emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "first" } },
		});

		expect(session.getSnapshot().hasNewOffscreenContent).toBe(true);
		expect(listener).toHaveBeenCalledTimes(1);

		socket.emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "second" } },
		});
		expect(listener).toHaveBeenCalledTimes(1);
	});

	it("does not mark new offscreen content for stream parts while in follow mode", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = makeSession();
		const socket = startForegroundStream(session, sockets);
		const listener = vi.fn();
		session.subscribe(listener);

		socket.emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "first" } },
		});

		expect(session.getSnapshot().hasNewOffscreenContent).toBe(false);
		expect(listener).not.toHaveBeenCalled();
	});

	it("does not notify metadata subscribers for store and cache changes", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const queryClient = createTestQueryClient();
		const session = new ChatSession("chat-1", makeRuntimeDeps(queryClient));
		const message = makeMessage("chat-1", 1, "user");
		const queuedMessage = makeQueuedMessage("chat-1", 1);
		session.hydrateFromRest({
			chatMessages: [message],
			chatRecord: makeChat("chat-1", "pending"),
			chatMessagesData: makeMessagesData([message], [queuedMessage]),
			chatQueuedMessages: [queuedMessage],
		});
		session.enterForeground({ now: 1 });
		expect(sockets).toHaveLength(1);

		const listener = vi.fn();
		const unsubscribe = session.subscribe(listener);
		sockets[0].emitOpen();
		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "running" },
		});
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "stream" } },
		});
		vi.advanceTimersByTime(0);
		sockets[0].emitData({
			type: "retry",
			chat_id: "chat-1",
			retry: {
				attempt: 1,
				delay_ms: 100,
				error: "try again",
				retrying_at: "2025-01-01T00:00:01.000Z",
			},
		});
		sockets[0].emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 2, "assistant"),
		});
		sockets[0].emitData({
			type: "queue_update",
			chat_id: "chat-1",
			queued_messages: [makeQueuedMessage("chat-1", 2)],
		});
		session.store.setReconnectState({
			attempt: 1,
			delayMs: 1000,
			retryingAt: "2025-01-01T00:00:01.000Z",
		});
		session.upsertCacheMessages([makeMessage("chat-1", 3, "assistant")]);

		expect(listener).not.toHaveBeenCalled();
		unsubscribe();
	});
});
