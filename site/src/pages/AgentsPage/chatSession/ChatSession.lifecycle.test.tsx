import type { InfiniteData } from "react-query";
import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { watchChat } from "#/api/api";
import { chatMessagesKey, chatsKey } from "#/api/queries/chats";
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
type ChatMessagesInfiniteData = InfiniteData<TypesGen.ChatMessagesResponse>;
type InfiniteChatsData = {
	pages: TypesGen.Chat[][];
	pageParams: unknown[];
};

type MockSocket = WatchChatSocket & {
	emitOpen: () => void;
	emitData: (event: TypesGen.ChatStreamEvent) => void;
	emitDataBatch: (events: readonly TypesGen.ChatStreamEvent[]) => void;
	emitParseError: () => void;
};

const infiniteChatsTestKey = [...chatsKey, undefined] as const;

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
		emitDataBatch: (events) => {
			const payload: OneWayMessageEvent<TypesGen.ChatStreamEvent[]> = {
				sourceEvent: {} as MessageEvent<string>,
				parseError: undefined,
				parsedMessage: [...events],
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

type SetChatErrorReasonMock = ReturnType<
	typeof vi.fn<(chatId: string, reason: string) => void>
>;
type ClearChatErrorReasonMock = ReturnType<
	typeof vi.fn<(chatId: string) => void>
>;

const makeRuntimeDeps = (
	queryClient = createTestQueryClient(),
): ChatSessionRuntimeDeps & {
	setChatErrorReason: SetChatErrorReasonMock;
	clearChatErrorReason: ClearChatErrorReasonMock;
} => {
	const setChatErrorReason = vi.fn<(chatId: string, reason: string) => void>();
	const clearChatErrorReason = vi.fn<(chatId: string) => void>();
	return {
		queryClient,
		setChatErrorReason,
		clearChatErrorReason,
	};
};

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
	title: `chat ${chatId}`,
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

const hydrate = (
	session: ChatSession,
	messages: readonly TypesGen.ChatMessage[],
	status: TypesGen.ChatStatus = "running",
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
) => {
	session.hydrateFromRest({
		chatMessages: messages,
		chatRecord: makeChat(session.chatId, status),
		chatMessagesData: makeMessagesData(messages, queuedMessages),
		chatQueuedMessages: queuedMessages,
	});
};

const seedMessagesCache = (
	queryClient: QueryClient,
	chatId: string,
	messages: readonly TypesGen.ChatMessage[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
) => {
	queryClient.setQueryData<ChatMessagesInfiniteData>(chatMessagesKey(chatId), {
		pages: [makeMessagesData(messages, queuedMessages)],
		pageParams: [undefined],
	});
};

const readMessagesCache = (
	queryClient: QueryClient,
	chatId: string,
): ChatMessagesInfiniteData | undefined =>
	queryClient.getQueryData<ChatMessagesInfiniteData>(chatMessagesKey(chatId));

const seedInfiniteChats = (
	queryClient: QueryClient,
	chats: readonly TypesGen.Chat[],
) => {
	queryClient.setQueryData<InfiniteChatsData>(infiniteChatsTestKey, {
		pages: [[...chats]],
		pageParams: [0],
	});
};

const readInfiniteChats = (
	queryClient: QueryClient,
): TypesGen.Chat[] | undefined =>
	queryClient
		.getQueryData<InfiniteChatsData>(infiniteChatsTestKey)
		?.pages.flat();

const streamText = (session: ChatSession): string =>
	(session.store.getSnapshot().streamState?.blocks ?? [])
		.map((block) => {
			if (block.type === "response" || block.type === "thinking") {
				return block.text;
			}
			return "";
		})
		.join("");

afterEach(() => {
	vi.clearAllTimers();
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe("ChatSession stream events", () => {
	it("coalesces message parts and ignores them while pending or waiting", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();

		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "hello" } },
		});
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: " world" } },
		});
		expect(session.store.getSnapshot().streamState).toBeNull();
		vi.advanceTimersByTime(0);
		expect(streamText(session)).toBe("hello world");

		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "pending" },
		});
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "hello world" },
		]);
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: " ignored" } },
		});
		vi.advanceTimersByTime(0);
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "hello world" },
		]);

		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "running" },
		});
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "discard me" } },
		});
		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "waiting" },
		});
		vi.advanceTimersByTime(0);
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "hello world" },
		]);
	});

	it("preserves stream state when status transitions to waiting", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();

		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "thinking..." } },
		});
		vi.advanceTimersByTime(0);
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "thinking..." },
		]);

		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "waiting" },
		});

		expect(session.store.getSnapshot().streamState).not.toBeNull();
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "thinking..." },
		]);
	});

	it("clears stream state when durable message follows waiting status", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();

		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "partial response" } },
		});
		vi.advanceTimersByTime(0);
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "partial response" },
		]);

		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "waiting" },
		});
		expect(session.store.getSnapshot().streamState?.blocks).toEqual([
			{ type: "response", text: "partial response" },
		]);

		sockets[0].emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 2, "assistant", "partial response"),
		});

		const snapshot = session.store.getSnapshot();
		expect(snapshot.streamState).toBeNull();
		expect(snapshot.orderedMessageIDs).toContain(2);
	});

	it("flushes buffered parts before durable messages and errors", () => {
		vi.useFakeTimers();
		const queryClient = createTestQueryClient();
		const deps = makeRuntimeDeps(queryClient);
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", deps);
		seedInfiniteChats(queryClient, [makeChat("chat-1", "running")]);
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();

		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "partial" } },
		});
		sockets[0].emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 2, "user"),
		});
		expect(streamText(session)).toBe("partial");
		expect(session.store.getSnapshot().messagesByID.has(2)).toBe(true);

		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: " before error" } },
		});
		sockets[0].emitData({
			type: "error",
			chat_id: "chat-1",
			error: {
				message: "provider failed",
				detail: "details",
				kind: "overloaded",
				provider: "test-provider",
				retryable: true,
				status_code: 503,
			},
		});

		expect(streamText(session)).toBe("partial before error");
		expect(session.store.getSnapshot().chatStatus).toBe("error");
		expect(session.store.getSnapshot().streamError).toMatchObject({
			message: "provider failed",
			detail: "details",
			kind: "overloaded",
			provider: "test-provider",
			retryable: true,
			statusCode: 503,
		});
		expect(deps.setChatErrorReason).toHaveBeenCalledWith(
			"chat-1",
			"provider failed",
		);
		expect(readInfiniteChats(queryClient)?.[0]?.status).toBe("error");
	});

	it("commits assistant messages and clears transient stream state in one batch", () => {
		vi.useFakeTimers();
		const queryClient = createTestQueryClient();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps(queryClient));
		seedMessagesCache(queryClient, "chat-1", [
			makeMessage("chat-1", 1, "user"),
		]);
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();
		const notifications: ReturnType<typeof session.store.getSnapshot>[] = [];
		session.store.subscribe(() => {
			notifications.push(session.store.getSnapshot());
		});

		sockets[0].emitDataBatch([
			{
				type: "message_part",
				chat_id: "chat-1",
				message_part: { part: { type: "text", text: "draft" } },
			},
			{
				type: "message",
				chat_id: "chat-1",
				message: makeMessage("chat-1", 2, "assistant", "final"),
			},
		]);

		expect(notifications).toHaveLength(1);
		expect(notifications[0].streamState).toBeNull();
		expect(notifications[0].messagesByID.get(2)?.content).toEqual([
			{ type: "text", text: "final" },
		]);
		expect(readMessagesCache(queryClient, "chat-1")?.pages[0].messages).toEqual(
			[
				makeMessage("chat-1", 2, "assistant", "final"),
				makeMessage("chat-1", 1, "user"),
			],
		);
	});

	it("surfaces parse errors without corrupting the current stream", () => {
		vi.useFakeTimers();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();
		sockets[0].emitData({
			type: "message_part",
			chat_id: "chat-1",
			message_part: { part: { type: "text", text: "still here" } },
		});
		vi.advanceTimersByTime(0);

		sockets[0].emitParseError();

		expect(streamText(session)).toBe("still here");
		expect(session.store.getSnapshot().streamError).toEqual({
			kind: "generic",
			message: "Failed to parse chat stream update.",
		});
	});
});

describe("ChatSession REST and authority rules", () => {
	it("preserves WebSocket messages until a previous REST id disappears", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		const first = makeMessage("chat-1", 1, "user");
		const second = makeMessage("chat-1", 2, "assistant");
		hydrate(session, [first, second]);
		session.enterForeground();
		sockets[0].emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 3, "user"),
		});

		hydrate(session, [first, second]);
		expect(session.store.getSnapshot().messagesByID.has(3)).toBe(true);

		hydrate(session, [first]);
		expect(session.store.getSnapshot().orderedMessageIDs).toEqual([1]);
	});

	it("keeps WebSocket status and queue updates authoritative over REST", () => {
		const queryClient = createTestQueryClient();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps(queryClient));
		const initialQueued = makeQueuedMessage("chat-1", 1);
		const streamedQueued = makeQueuedMessage("chat-1", 2);
		seedMessagesCache(queryClient, "chat-1", [], [initialQueued]);
		hydrate(session, [makeMessage("chat-1", 1, "user")], "pending", [
			initialQueued,
		]);
		session.enterForeground();
		sockets[0].emitData({
			type: "status",
			chat_id: "chat-1",
			status: { status: "running" },
		});
		sockets[0].emitData({
			type: "queue_update",
			chat_id: "chat-1",
			queued_messages: [streamedQueued],
		});

		hydrate(session, [makeMessage("chat-1", 1, "user")], "completed", [
			initialQueued,
		]);

		expect(session.store.getSnapshot().chatStatus).toBe("running");
		expect(session.store.getSnapshot().queuedMessages).toEqual([
			streamedQueued,
		]);
		expect(
			readMessagesCache(queryClient, "chat-1")?.pages[0].queued_messages,
		).toEqual([streamedQueued]);
	});

	it("leaves queued message cache references unchanged when ids match", () => {
		const queryClient = createTestQueryClient();
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps(queryClient));
		const queued = makeQueuedMessage("chat-1", 1);
		seedMessagesCache(queryClient, "chat-1", [], [queued]);
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();
		const before = readMessagesCache(queryClient, "chat-1");

		sockets[0].emitData({
			type: "queue_update",
			chat_id: "chat-1",
			queued_messages: [makeQueuedMessage("chat-1", 1)],
		});
		expect(readMessagesCache(queryClient, "chat-1")).toBe(before);

		sockets[0].emitData({
			type: "queue_update",
			chat_id: "chat-1",
			queued_messages: [makeQueuedMessage("chat-1", 2)],
		});
		expect(readMessagesCache(queryClient, "chat-1")).not.toBe(before);
	});

	it("routes other chat statuses to subagent overrides", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		hydrate(session, [makeMessage("chat-1", 1, "user")], "running");
		session.enterForeground();

		sockets[0].emitData({
			type: "status",
			chat_id: "child-chat",
			status: { status: "completed" },
		});

		expect(session.store.getSnapshot().chatStatus).toBe("running");
		expect(
			session.store.getSnapshot().subagentStatusOverrides.get("child-chat"),
		).toBe("completed");
	});

	it("uses the latest durable message id when reopening a retained session", () => {
		const sockets = mockWatchChatWithFreshSockets();
		const session = new ChatSession("chat-1", makeRuntimeDeps());
		hydrate(session, [makeMessage("chat-1", 1, "user")]);
		session.enterForeground();
		expect(watchChat).toHaveBeenLastCalledWith("chat-1", 1);

		sockets[0].emitData({
			type: "message",
			chat_id: "chat-1",
			message: makeMessage("chat-1", 2, "user"),
		});
		session.enterBackgroundNoRead();

		expect(watchChat).toHaveBeenCalledTimes(2);
		expect(watchChat).toHaveBeenLastCalledWith("chat-1", 2, {
			markRead: false,
		});
	});
});
