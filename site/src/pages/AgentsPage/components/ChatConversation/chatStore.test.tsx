import { act, render, renderHook, waitFor } from "@testing-library/react";
import { watchChat } from "#/api/api";
import { chatMessagesKey, chatsKey } from "#/api/queries/chats";

// The infinite query key used by useInfiniteQuery(infiniteChats())
// is [...chatsKey, undefined] = ["chats", undefined].
const infiniteChatsTestKey = [...chatsKey, undefined];

type InfiniteData = {
	pages: TypesGen.Chat[][];
	pageParams: unknown[];
};

/** Seed the infinite chats cache in the format TanStack Query expects. */
const seedInfiniteChats = (
	queryClient: QueryClient,
	chats: TypesGen.Chat[],
) => {
	queryClient.setQueryData<InfiniteData>(infiniteChatsTestKey, {
		pages: [chats],
		pageParams: [0],
	});
};

/** Read chats back from the infinite query cache. */
const readInfiniteChats = (
	queryClient: QueryClient,
): TypesGen.Chat[] | undefined => {
	const data = queryClient.getQueryData<InfiniteData>(infiniteChatsTestKey);
	return data?.pages.flat();
};

import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	selectChatStatus,
	selectIsAwaitingFirstStreamChunk,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectReconnectState,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	useChatStore,
} from "./chatStore";

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

type MockSocketHelpers = {
	emitOpen: () => void;
	emitData: (event: TypesGen.ChatStreamEvent) => void;
	emitDataBatch: (events: readonly TypesGen.ChatStreamEvent[]) => void;
	emitParseError: () => void;
	emitError: () => void;
	emitClose: () => void;
};

type MockSocket = WatchChatSocket & MockSocketHelpers;

const mockWatchChatReturn = (socket: MockSocket): void => {
	vi.mocked(watchChat).mockReturnValue(socket);
};

const mockWatchChatReturnOnce = (socket: MockSocket): void => {
	vi.mocked(watchChat).mockReturnValueOnce(socket);
};

const mockWatchChatWithFreshSockets = (
	watchMock = vi.mocked(watchChat),
): MockSocket[] => {
	const sockets: MockSocket[] = [];
	watchMock.mockImplementation(() => {
		const socket = createMockSocket();
		sockets.push(socket);
		return socket;
	});
	return sockets;
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
		url: "ws://example.test/api/experimental/chats/mock-stream",
		addEventListener,
		removeEventListener,
		close: vi.fn(),
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
				parsedMessage: events as TypesGen.ChatStreamEvent[],
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
		emitOpen: () => {
			for (const listener of openListeners) {
				listener(new Event("open"));
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

const makeChat = (chatID: string): TypesGen.Chat => ({
	id: chatID,
	organization_id: "test-org-id",
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	title: "test",
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	last_error: null,
});

const makeMessage = (
	chatID: string,
	id: number,
	role: TypesGen.ChatMessageRole,
	text: string,
): TypesGen.ChatMessage => ({
	id,
	chat_id: chatID,
	created_at: "2025-01-01T00:00:00.000Z",
	role,
	content: [{ type: "text", text }],
});

const makeQueuedMessage = (
	chatID: string,
	id: number,
	text: string,
): TypesGen.ChatQueuedMessage => ({
	id,
	chat_id: chatID,
	created_at: "2025-01-01T00:00:00.000Z",
	content: [{ type: "text", text }],
});

const immediateAnimationFrame = (): void => {
	vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
		callback(0);
		return 1;
	});
	vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
};

afterEach(() => {
	vi.clearAllTimers();
	vi.useRealTimers();
	vi.restoreAllMocks();
	vi.mocked(watchChat).mockReset();
});

describe("useChatStore", () => {
	it("does not clear in-progress stream parts for duplicate snapshot messages", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "reconnect-part-one",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "reconnect-part-one" },
			]);
		});

		act(() => {
			const duplicateSnapshotMessage: TypesGen.ChatMessage = {
				...existingMessage,
				content: [...(existingMessage.content ?? [])],
			};
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: duplicateSnapshotMessage,
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "reconnect-part-one" },
			]);
		});
	});

	it("clears stream state when a new durable message arrives", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const newMessage = makeMessage(chatID, 2, "assistant", "done");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "working",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "working" },
			]);
		});

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: newMessage,
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});
	});

	it("clears stream state when a duplicate message id arrives with new content", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "assistant", "old");
		const updatedMessage = makeMessage(chatID, 1, "assistant", "updated");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "partial",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "partial" },
			]);
		});

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: updatedMessage,
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});
	});

	it("keeps non-stream selectors from rerendering during message_part updates", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		let streamRenderCount = 0;
		let queueRenderCount = 0;
		let orderedIDsRenderCount = 0;

		type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

		const StreamProbe: FC<{ store: ChatStoreHandle }> = ({ store }) => {
			useChatSelector(store, selectStreamState);
			streamRenderCount += 1;
			return null;
		};

		const QueueProbe: FC<{ store: ChatStoreHandle }> = ({ store }) => {
			useChatSelector(store, selectQueuedMessages);
			queueRenderCount += 1;
			return null;
		};

		const OrderedIDsProbe: FC<{ store: ChatStoreHandle }> = ({ store }) => {
			useChatSelector(store, selectOrderedMessageIDs);
			orderedIDsRenderCount += 1;
			return null;
		};

		const TestHarness: FC = () => {
			const { store } = useChatStore({
				chatID,
				chatMessages: [existingMessage],
				chatRecord: makeChat(chatID),
				chatMessagesData: {
					messages: [existingMessage],
					queued_messages: [],
					has_more: false,
				},
				chatQueuedMessages: [],
				setChatErrorReason,
				clearChatErrorReason,
			});
			return (
				<>
					<StreamProbe store={store} />
					<QueueProbe store={store} />
					<OrderedIDsProbe store={store} />
				</>
			);
		};

		render(
			<QueryClientProvider client={queryClient}>
				<TestHarness />
			</QueryClientProvider>,
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		const streamBaseline = streamRenderCount;
		const queueBaseline = queueRenderCount;
		const orderedIDsBaseline = orderedIDsRenderCount;

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "partial",
					},
				},
			});
		});

		await waitFor(() => {
			expect(streamRenderCount).toBeGreaterThan(streamBaseline);
		});
		expect(queueRenderCount).toBe(queueBaseline);
		expect(orderedIDsRenderCount).toBe(orderedIDsBaseline);
	});

	it("applies batched message_part events from one payload", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: {
							type: "text",
							text: "hello ",
						},
					},
				},
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: {
							type: "text",
							text: "world",
						},
					},
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "hello world" },
			]);
		});
	});

	it("ignores message_part updates while chat is pending", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "first",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "first" },
			]);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "pending" },
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "late",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});
	});

	it("does not restore stale queued messages after a stream queue_update", async () => {
		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const queuedMessage = makeQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();
		const initialOptions = {
			chatID,
			chatMessages: [existingMessage],
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: [existingMessage],
				queued_messages: [queuedMessage],
				has_more: false,
			},
			chatQueuedMessages: [queuedMessage],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{
				initialProps: initialOptions,
				wrapper,
			},
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});
		expect(result.current.queuedMessages.map((message) => message.id)).toEqual([
			queuedMessage.id,
		]);

		act(() => {
			mockSocket.emitData({
				type: "queue_update",
				chat_id: chatID,
				queued_messages: [],
			});
		});

		await waitFor(() => {
			expect(result.current.queuedMessages).toEqual([]);
		});

		rerender({
			...initialOptions,
			chatMessagesData: {
				messages: [existingMessage],
				queued_messages: [queuedMessage],
				has_more: false,
			},
			chatQueuedMessages: [queuedMessage],
		});

		await waitFor(() => {
			expect(result.current.queuedMessages).toEqual([]);
		});
	});

	it("corrects stale queued messages from cache when switching back to a chat", async () => {
		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const queuedMessage = makeQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		// Start with queued messages from a stale React Query cache.
		// This simulates coming back to a chat whose queue was drained
		// server-side while the user was viewing a different chat.
		const staleOptions = {
			chatID,
			chatMessages: [existingMessage],
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: [existingMessage],
				queued_messages: [queuedMessage],
				has_more: false,
			},
			chatQueuedMessages: [queuedMessage],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{
				initialProps: staleOptions,
				wrapper,
			},
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});
		// Initially shows the stale queued message from cache.
		expect(result.current.queuedMessages.map((m) => m.id)).toEqual([
			queuedMessage.id,
		]);

		// Simulate the REST query refetching and returning fresh
		// data with an empty queue (no queue_update from WS yet).
		rerender({
			...staleOptions,
			chatMessagesData: {
				messages: [existingMessage],
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [],
		});

		// The store should accept the fresh REST data because the
		// WebSocket hasn't sent a queue_update yet.
		await waitFor(() => {
			expect(result.current.queuedMessages).toEqual([]);
		});
	});

	it("writes queue_update snapshots into the chat query cache", async () => {
		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const queuedMessage = makeQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChatMessagesData: TypesGen.ChatMessagesResponse = {
			messages: [existingMessage],
			queued_messages: [queuedMessage],
			has_more: false,
		};
		// The cache is InfiniteData<ChatMessagesResponse> after the
		// migration to useInfiniteQuery for chat messages.
		queryClient.setQueryData(chatMessagesKey(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [queuedMessage],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "queue_update",
				chat_id: chatID,
				queued_messages: [],
			});
		});

		await waitFor(() => {
			expect(result.current.queuedMessages).toEqual([]);
		});
		const cachedData = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesKey(chatID));
		expect(cachedData?.pages[0]?.queued_messages).toEqual([]);
	});

	it("writes WebSocket message events into the chat query cache", async () => {
		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChatMessagesData: TypesGen.ChatMessagesResponse = {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		};
		queryClient.setQueryData(chatMessagesKey(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					orderedIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		const newMessage = makeMessage(chatID, 2, "assistant", "hi there");
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: newMessage,
			});
		});

		await waitFor(() => {
			expect(result.current.orderedIDs).toContain(2);
		});

		// The React Query cache should also contain the new message.
		const cachedData = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesKey(chatID));
		const cachedMessages = cachedData?.pages[0]?.messages ?? [];
		// Verifies insertion, preservation, and DESC order.
		expect(cachedMessages.map((m) => m.id)).toEqual([2, 1]);
		// Emitting the same message again should not change the
		// cache reference (reference stability).
		const refBefore = queryClient.getQueryData(chatMessagesKey(chatID));
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: newMessage,
			});
		});
		const refAfter = queryClient.getQueryData(chatMessagesKey(chatID));
		expect(refAfter).toBe(refBefore);

		// Emitting the same message ID with different content should
		// update the cached entry (content-update path).
		const revised = makeMessage(chatID, 2, "assistant", "revised");
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: revised,
			});
		});
		const updatedCache = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesKey(chatID));
		const updatedFirst = updatedCache?.pages[0]?.messages[0];
		expect(updatedFirst?.content).toEqual([{ type: "text", text: "revised" }]);
	});

	it("closes old WebSocket and resets state when chatID changes", async () => {
		immediateAnimationFrame();

		const chatID1 = "chat-1";
		const chatID2 = "chat-2";
		const msg1 = makeMessage(chatID1, 1, "user", "hello");
		const msg2 = makeMessage(chatID2, 10, "user", "world");

		const mockSocket1 = createMockSocket();
		const mockSocket2 = createMockSocket();
		// Use a fallback so that extra effect re-runs (caused by
		// dependency changes during rerender) get a valid socket.
		mockWatchChatReturn(mockSocket2);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: makeChat(chatID1),
			chatMessagesData: {
				messages: [msg1],
				queued_messages: [] as TypesGen.ChatQueuedMessage[],
				has_more: false,
			},
			chatQueuedMessages: [] as TypesGen.ChatQueuedMessage[],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID1, 1);
		});

		act(() => {
			mockSocket1.emitData({
				type: "message_part",
				chat_id: chatID1,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "chat1-stream",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "chat1-stream" },
			]);
		});

		rerender({
			...initialOptions,
			chatID: chatID2,
			chatMessages: [msg2],
			chatRecord: makeChat(chatID2),
			chatMessagesData: {
				messages: [msg2],
				queued_messages: [],
				has_more: false,
			},
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 10);
		});

		// The old WebSocket was closed during effect cleanup.
		expect(mockSocket1.close).toHaveBeenCalled();
		// Stream state was reset — no stale stream data from chat-1.
		expect(result.current.streamState).toBeNull();
	});

	it("ignores queue_update events for other chats", async () => {
		const chatID = "chat-1";
		const otherChatID = "chat-2";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const queuedMessage = makeQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [queuedMessage],
						has_more: false,
					},
					chatQueuedMessages: [queuedMessage],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "queue_update",
				chat_id: otherChatID,
				queued_messages: [],
			});
		});

		await waitFor(() => {
			expect(
				result.current.queuedMessages.map((message) => message.id),
			).toEqual([queuedMessage.id]);
		});
	});

	it("filters message events with mismatched chat_id", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Build up stream state so we can observe whether it gets cleared.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "streaming",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "streaming" },
			]);
		});

		// A message event with a mismatched chat_id should be ignored
		// and should NOT trigger scheduleStreamReset.
		const mismatchedMessage = makeMessage(
			"chat-2",
			99,
			"assistant",
			"wrong chat",
		);
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: "chat-2",
				message: mismatchedMessage,
			});
		});

		// Stream state should still be present — the mismatched event
		// was filtered and did not trigger a stream reset.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "streaming" },
			]);
		});

		// A message event with the correct chat_id should be processed
		// and trigger scheduleStreamReset, clearing stream state.
		const matchingMessage = makeMessage(chatID, 2, "assistant", "correct chat");
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: matchingMessage,
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});
	});

	it("cancels scheduled stream reset when message_part arrives after message", async () => {
		immediateAnimationFrame();

		const chatID = "chat-raf";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Build up stream state first.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "working" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "working" },
			]);
		});

		// Emit a durable message followed by a message_part in the
		// same batch. The message handler calls scheduleStreamReset
		// (via rAF), and the subsequent message_part handler calls
		// cancelScheduledStreamReset to prevent a flash. The final
		// flushMessageParts re-populates stream state.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: makeMessage(chatID, 2, "assistant", "done"),
				},
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: { type: "text", text: " more" },
					},
				},
			]);
		});

		// Stream state should be non-null because the message_part
		// after the message kept it populated.
		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
		});
	});

	it("startTransition deferred parts are discarded after chat switch", async () => {
		immediateAnimationFrame();

		const chatID1 = "chat-1";
		const chatID2 = "chat-2";
		const msg1 = makeMessage(chatID1, 1, "user", "hello");
		const msg2 = makeMessage(chatID2, 10, "user", "world");

		const mockSocket1 = createMockSocket();
		const mockSocket2 = createMockSocket();
		// Use a fallback so that extra effect re-runs get a valid socket.
		mockWatchChatReturn(mockSocket2);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: makeChat(chatID1),
			chatMessagesData: {
				messages: [msg1],
				queued_messages: [] as TypesGen.ChatQueuedMessage[],
				has_more: false,
			},
			chatQueuedMessages: [] as TypesGen.ChatQueuedMessage[],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID1, 1);
		});

		act(() => {
			mockSocket1.emitData({
				type: "message_part",
				chat_id: chatID1,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "stale-part",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "stale-part" },
			]);
		});

		rerender({
			...initialOptions,
			chatID: chatID2,
			chatMessages: [msg2],
			chatRecord: makeChat(chatID2),
			chatMessagesData: {
				messages: [msg2],
				queued_messages: [],
				has_more: false,
			},
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 10);
		});

		expect(result.current.streamState).toBeNull();
	});

	it("messages are cleared immediately on chat switch before new query resolves", async () => {
		immediateAnimationFrame();

		const chatID1 = "chat-1";
		const chatID2 = "chat-2";
		const msg1 = makeMessage(chatID1, 1, "user", "first");
		const queuedMsg = makeQueuedMessage(chatID1, 10, "queued");

		const mockSocket1 = createMockSocket();
		const mockSocket2 = createMockSocket();
		// Use a fallback so that extra effect re-runs get a valid socket.
		mockWatchChatReturn(mockSocket2);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: makeChat(chatID1),
			chatMessagesData: {
				messages: [msg1],
				queued_messages: [queuedMsg],
				has_more: false,
			},
			chatQueuedMessages: [queuedMsg],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID1, 1);
		});

		// Verify queued messages from chat-1 are present.
		expect(result.current.queuedMessages.map((m) => m.id)).toEqual([
			queuedMsg.id,
		]);

		// Switch to chat-2 with no messages and no queued messages
		// (simulating query not yet resolved for the new chat).
		rerender({
			...initialOptions,
			chatID: chatID2,
			chatMessages: [],
			chatRecord: makeChat(chatID2),
			chatMessagesData: {
				messages: [],
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [],
		});

		// After the switch, queued messages from chat-1 should NOT be
		// visible — the store resets them on chatID change.
		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, undefined);
		});
		expect(result.current.queuedMessages).toEqual([]);
	});

	it("does not apply message parts after status changes to waiting", async () => {
		immediateAnimationFrame();

		const chatID = "chat-status-guard";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Emit a batch with message_parts followed by a status change
		// to "waiting". The status handler clears stream state
		// synchronously, and the startTransition guard should prevent
		// the deferred applyMessageParts from re-populating it.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: { type: "text", text: "should be discarded" },
					},
				},
				{
					type: "status",
					chat_id: chatID,
					status: { status: "waiting" },
				},
			]);
		});

		// Stream state should be null — the status change cleared it,
		// and the deferred applyMessageParts should not have
		// re-populated it.
		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});
	});

	it("sets chatStatus to error and populates streamError on error event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
					streamError: useChatSelector(store, selectStreamError),
					retryState: useChatSelector(store, selectRetryState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: {
					message: "Rate limit exceeded",
					kind: "rate_limit",
					provider: "anthropic",
					retryable: true,
					status_code: 429,
				},
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
		});
		expect(result.current.streamError).toEqual({
			kind: "rate_limit",
			message: "Rate limit exceeded",
			provider: "anthropic",
			retryable: true,
			statusCode: 429,
		});
		expect(result.current.retryState).toBeNull();
		expect(setChatErrorReason).toHaveBeenCalledWith(chatID, {
			kind: "rate_limit",
			message: "Rate limit exceeded",
			provider: "anthropic",
			retryable: true,
			statusCode: 429,
		});
	});

	it("uses fallback message when error event has no message", async () => {
		immediateAnimationFrame();

		const chatID = "chat-error-empty";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamError: useChatSelector(store, selectStreamError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "  ", retryable: false },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Chat processing failed.",
				provider: undefined,
				retryable: false,
				statusCode: undefined,
			});
		});
	});

	it("populates retryState on retry event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					retryState: useChatSelector(store, selectRetryState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 2,
					error: "upstream timeout",
					kind: "timeout",
					provider: "anthropic",
					delay_ms: 5000,
					retrying_at: "2025-01-01T00:01:00.000Z",
				},
			});
		});

		await act(async () => {});
		expect(result.current.retryState).toEqual({
			attempt: 2,
			error: "upstream timeout",
			kind: "timeout",
			provider: "anthropic",
			delayMs: 5000,
			retryingAt: "2025-01-01T00:01:00.000Z",
		});
	});

	it("clears retryState when status transitions to running", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry-clear";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					retryState: useChatSelector(store, selectRetryState),
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Set retry state first.
		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 1,
					error: "rate limited",
					kind: "rate_limit",
					provider: "anthropic",
					delay_ms: 3000,
					retrying_at: "2025-01-01T00:00:30.000Z",
				},
			});
		});

		await waitFor(() => {
			expect(result.current.retryState).toEqual({
				attempt: 1,
				error: "rate limited",
				kind: "rate_limit",
				provider: "anthropic",
				delayMs: 3000,
				retryingAt: "2025-01-01T00:00:30.000Z",
			});
		});

		// Transition to running — should clear retry state.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});
		expect(result.current.retryState).toBeNull();
	});

	it("clears retryState when message_part arrives after retry", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry-message-part";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					retryState: useChatSelector(store, selectRetryState),
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 1,
					error: "rate limited",
					kind: "rate_limit",
					provider: "anthropic",
					delay_ms: 3000,
					retrying_at: "2025-01-01T00:00:30.000Z",
				},
			});
		});

		await waitFor(() => {
			expect(result.current.retryState).toEqual({
				attempt: 1,
				error: "rate limited",
				kind: "rate_limit",
				provider: "anthropic",
				delayMs: 3000,
				retryingAt: "2025-01-01T00:00:30.000Z",
			});
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "retry recovered" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.retryState).toBeNull();
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "retry recovered" },
			]);
		});
	});

	it("routes status events for other chatIDs to subagent overrides", async () => {
		immediateAnimationFrame();

		const chatID = "chat-main";
		const subagentChatID = "chat-subagent-1";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
					subagentStatusOverrides: useChatSelector(
						store,
						selectSubagentStatusOverrides,
					),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: subagentChatID,
				status: { status: "completed" },
			});
		});

		await waitFor(() => {
			expect(result.current.subagentStatusOverrides.get(subagentChatID)).toBe(
				"completed",
			);
		});
		// Main chat status should remain "running" from the initial
		// chatRecord — the subagent status event must not change it.
		expect(result.current.chatStatus).toBe("running");
	});

	it("sets reconnectState on WebSocket disconnect and clears it after reconnect", async () => {
		immediateAnimationFrame();
		vi.spyOn(Math, "random").mockReturnValue(0.5);

		const chatID = "chat-disconnect";
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
					reconnectState: useChatSelector(store, selectReconnectState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Simulate disconnect.
		act(() => {
			mockSocket1.emitError();
		});

		await waitFor(() => {
			expect(result.current.reconnectState).toMatchObject({
				attempt: 1,
				delayMs: 1000,
			});
			expect(result.current.reconnectState?.retryingAt).toEqual(
				expect.any(String),
			);
			expect(result.current.chatStatus).toBe("running");
		});

		// The reconnect timer fires after 1s. Since we're not
		// using fake timers, waitFor will naturally wait.
		const mockSocket2 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket2);

		await waitFor(
			() => {
				expect(watchChat).toHaveBeenCalledTimes(2);
			},
			{ timeout: 3_000 },
		);

		// Simulate successful reconnection.
		act(() => {
			mockSocket2.emitOpen();
		});

		await waitFor(() => {
			expect(result.current.reconnectState).toBeNull();
			expect(result.current.chatStatus).toBe("running");
		});
	});

	it("clears stale streamError when a reconnected socket opens", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		immediateAnimationFrame();

		const chatID = "chat-reconnect-clear-error";
		const watchMock = vi.mocked(watchChat);
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					store,
					streamError: useChatSelector(store, selectStreamError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchMock).toHaveBeenCalledWith(chatID, undefined);
		});

		const socket1 = sockets[0]!;
		act(() => {
			socket1.emitOpen();
			result.current.store.setStreamError({
				kind: "generic",
				message: "Stale transport failure.",
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Stale transport failure.",
			});
		});

		act(() => {
			socket1.emitClose();
		});

		await act(async () => {
			vi.advanceTimersByTime(1_500);
		});

		expect(watchMock).toHaveBeenCalledTimes(2);
		const socket2 = sockets[1]!;

		act(() => {
			socket2.emitOpen();
		});

		await waitFor(() => {
			expect(result.current.streamError).toBeNull();
		});
	});

	it("keeps terminal streamError when a WebSocket disconnect follows it", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect-existing";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamError: useChatSelector(store, selectStreamError),
					reconnectState: useChatSelector(store, selectReconnectState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Set an error via an error stream event first.
		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "Rate limit exceeded", retryable: false },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Rate limit exceeded",
				provider: undefined,
				retryable: false,
				statusCode: undefined,
			});
			expect(result.current.reconnectState).toBeNull();
		});

		// WebSocket disconnect should not overwrite the terminal error
		// or surface reconnect state once the turn has already failed.
		act(() => {
			mockSocket.emitError();
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Rate limit exceeded",
				provider: undefined,
				retryable: false,
				statusCode: undefined,
			});
			expect(result.current.reconnectState).toBeNull();
		});
	});

	it("does not surface reconnectState for completed chats", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect-completed";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: { ...makeChat(chatID), status: "completed" },
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
					reconnectState: useChatSelector(store, selectReconnectState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("completed");
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitError();
		});

		await waitFor(() => {
			expect(result.current.reconnectState).toBeNull();
			expect(result.current.chatStatus).toBe("completed");
		});
	});
	it("uses exponential backoff on consecutive disconnects", async () => {
		immediateAnimationFrame();

		const chatID = "chat-backoff";
		const watchMock = vi.mocked(watchChat);
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		renderHook(
			() =>
				useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchMock).toHaveBeenCalledTimes(1);
		});

		// Get the first socket and disconnect it.
		const socket1 = sockets[0]!;
		act(() => socket1.emitClose());

		// First reconnect after 1s.
		await waitFor(() => expect(watchMock).toHaveBeenCalledTimes(2), {
			timeout: 3_000,
		});

		// Second disconnect — reconnect after 2s.
		const socket2 = sockets[1]!;
		act(() => socket2.emitClose());

		await waitFor(() => expect(watchMock).toHaveBeenCalledTimes(3), {
			timeout: 5_000,
		});
	});

	it("passes latest message ID on reconnect for catch-up", async () => {
		immediateAnimationFrame();

		const chatID = "chat-catchup";
		const msg = makeMessage(chatID, 42, "assistant", "hello");
		const watchMock = vi.mocked(watchChat);
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		renderHook(
			() =>
				useChatStore({
					chatID,
					chatMessages: [msg],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [msg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper },
		);

		// First connect uses the last message ID from chatMessages.
		await waitFor(() => {
			expect(watchMock).toHaveBeenCalledWith(chatID, 42);
		});

		// Disconnect and reconnect.
		const socket1 = sockets[0]!;
		act(() => socket1.emitClose());

		// Second connect should also use the last message ID.
		await waitFor(
			() => {
				expect(watchMock).toHaveBeenCalledTimes(2);
				expect(watchMock).toHaveBeenLastCalledWith(chatID, 42);
			},
			{ timeout: 3_000 },
		);
	});

	it("does not duplicate streamed text after reconnect", async () => {
		// The reconnect timer in createReconnectingWebSocket
		// fires inside a setTimeout. With real timers the
		// callback runs outside any act() boundary, so
		// startTransition updates from the reconnected socket
		// never commit to React state. Fake timers with
		// shouldAdvanceTime let us control when the reconnect
		// timer fires (via advanceTimersByTime inside act)
		// while still letting waitFor's internal polling work.
		vi.useFakeTimers({ shouldAdvanceTime: true });
		immediateAnimationFrame();

		const chatID = "chat-reconnect-dedup";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const watchMock = vi.mocked(watchChat);

		// Return a fresh MockSocket for each connection attempt
		// so we can control the first and second sockets
		// independently.
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		// Wait for the first socket to be created.
		await waitFor(() => {
			expect(watchMock).toHaveBeenCalledWith(chatID, 1);
		});

		const socket1 = sockets[0]!;

		// Simulate the first socket opening successfully.
		act(() => socket1.emitOpen());

		// Stream "Hello world" on the first connection.
		act(() => {
			socket1.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "Hello" },
				},
			});
			socket1.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: " world" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "Hello world" },
			]);
		});

		// --- Disconnect the first socket ---
		act(() => socket1.emitClose());

		// Advance past the reconnect backoff (1 s base delay)
		// inside act() so the setTimeout callback fires within
		// React's scheduling context.
		await act(async () => {
			vi.advanceTimersByTime(1_500);
		});

		// A second socket should now exist.
		expect(watchMock).toHaveBeenCalledTimes(2);
		const socket2 = sockets[1]!;

		// Simulate the reconnected socket opening. This is
		// where onOpen fires resetTransportReplayState().
		act(() => socket2.emitOpen());

		// Replay the same parts the server would send on the
		// new connection.
		act(() => {
			socket2.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "Hello" },
				},
			});
			socket2.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: " world" },
				},
			});
		});

		// Without resetTransportReplayState() in onOpen the
		// replayed parts would append to the stale accumulator,
		// producing
		// "Hello worldHello world". The fix ensures a clean
		// slate so we get the correct single copy.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "Hello world" },
			]);
		});

		vi.useRealTimers();
	});

	it("clears chatErrorReason when status transitions to non-error", async () => {
		immediateAnimationFrame();

		const chatID = "chat-clear-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Transition to running — should call clearChatErrorReason.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(clearChatErrorReason).toHaveBeenCalledWith(chatID);
		});
	});

	it("removes stale messages when refetched set is smaller (edit truncation)", async () => {
		immediateAnimationFrame();

		const chatID = "chat-edit-truncation";
		const msg1 = makeMessage(chatID, 1, "user", "first");
		const msg2 = makeMessage(chatID, 2, "assistant", "second");
		const msg3 = makeMessage(chatID, 3, "user", "third");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper: FC<PropsWithChildren> = ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const noQueued: TypesGen.ChatQueuedMessage[] = [];
		const initialMessages = [msg1, msg2, msg3];

		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: initialMessages,
				queued_messages: noQueued,
				has_more: false,
			},
			chatQueuedMessages: noQueued,
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		// All three messages should be in the store.
		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
		});

		// Simulate a post-edit refetch that only returns the first
		// message (server truncated messages 2 and 3).
		rerender({
			...initialOptions,
			chatMessages: [msg1],
			chatMessagesData: {
				messages: [msg1],
				queued_messages: [],
				has_more: false,
			},
		});

		// Messages 2 and 3 should be removed — replaceMessages should
		// have been used instead of upsert because the store contained
		// IDs not present in the fetched set.
		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1]);
		});
	});

	it("does not wipe WebSocket-delivered message when queue_update triggers cache change", async () => {
		immediateAnimationFrame();

		const chatID = "chat-queue-promote";
		const msg1 = makeMessage(chatID, 1, "user", "hello");
		const msg2 = makeMessage(chatID, 2, "assistant", "hi");
		// The promoted message that will arrive via WebSocket.
		const promotedMsg = makeMessage(chatID, 3, "user", "follow-up");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper: FC<PropsWithChildren> = ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const queuedMsg = makeQueuedMessage(chatID, 10, "follow-up");
		const initialMessages = [msg1, msg2];

		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: initialMessages,
				queued_messages: [queuedMsg],
				has_more: false,
			},
			chatQueuedMessages: [queuedMsg],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2]);
			expect(result.current.queuedMessages).toHaveLength(1);
		});

		// Simulate the WebSocket delivering the promoted message
		// followed by a queue_update in the same batch (as the server
		// does when auto-promoting or when the promote endpoint runs).
		act(() => {
			mockSocket.emitOpen();
		});
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: promotedMsg,
				},
				{
					type: "queue_update",
					chat_id: chatID,
					queued_messages: [],
				},
			]);
		});

		// The promoted message should appear in the store and the
		// queue should be empty. Before the fix, the queue_update
		// caused updateChatQueuedMessages to mutate the React Query
		// cache, giving chatMessages a new reference that triggered
		// the sync effect. The effect detected the promoted message
		// as a "stale entry" (present in store but not in the REST
		// data) and called replaceMessages, wiping it.
		//
		// Now re-render so the updated query cache flows through
		// the chatMessages prop (simulating React rerender after
		// the query cache mutation).
		rerender({
			...initialOptions,
			// chatMessages still comes from REST (not refetched), so
			// it only has [msg1, msg2]. The promoted message lives
			// only in the store via the WebSocket delivery.
			//
			// Spread into a new array to simulate what actually
			// happens: updateChatQueuedMessages mutates the React
			// Query cache (changing queued_messages), which gives
			// chatMessagesQuery.data a new reference, causing the
			// chatMessagesList useMemo to return a new array with
			// the same elements. The new reference triggers the
			// sync effect.
			chatMessages: [...initialMessages],
			chatMessagesData: {
				messages: [...initialMessages],
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [],
		});
		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
			expect(result.current.queuedMessages).toHaveLength(0);
		});
	});

	it("does not wipe in-progress stream state when user message arrives in batch", async () => {
		immediateAnimationFrame();

		const chatID = "chat-promote-stream";
		const msg1 = makeMessage(chatID, 1, "user", "hello");
		const msg2 = makeMessage(chatID, 2, "assistant", "hi");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper: FC<PropsWithChildren> = ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const queuedMsg = makeQueuedMessage(chatID, 10, "follow-up");
		const initialMessages = [msg1, msg2];

		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: initialMessages,
				queued_messages: [queuedMsg],
				has_more: false,
			},
			chatQueuedMessages: [queuedMsg],
			setChatErrorReason,
			clearChatErrorReason,
		};

		const { result } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					store,
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useChatSelector(store, selectChatStatus),
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2]);
		});

		// Open the WebSocket and set the chat to running.
		act(() => {
			mockSocket.emitOpen();
		});
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});

		// Deliver a batch containing trailing message_parts for
		// the current response followed by the promoted user
		// message. The batch loop flushes pending parts when it
		// hits the message event (building stream state). Before
		// the fix, scheduleStreamReset would fire for the user
		// message because it only checked `changed`, and with
		// immediateAnimationFrame the RAF fires synchronously,
		// wiping the stream state that was just built.
		const promotedUser = makeMessage(chatID, 3, "user", "follow-up");

		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						part: { type: "text", text: "I am helping you" },
					},
				},
				{
					type: "message",
					chat_id: chatID,
					message: promotedUser,
				},
			]);
		});

		// The stream state must survive: the promoted user message
		// should not wipe the in-progress assistant stream.
		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toContain(3);
			expect(result.current.streamState).not.toBeNull();
			const blocks = result.current.streamState?.blocks ?? [];
			const textBlock = blocks.find((b) => b.type === "response");
			expect(textBlock).toBeDefined();
		});
	});

	it("does not let a stale REST chatRecord.status override WS-delivered status", async () => {
		immediateAnimationFrame();

		const chatID = "chat-stale-rest-status";
		const userMsg = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		// Start with a "running" chatRecord so the WS opens.
		const { result, rerender } = renderHook(
			(props: { chatRecord: TypesGen.Chat }) => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: props.chatRecord,
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{
				wrapper,
				initialProps: {
					chatRecord: makeChat(chatID),
				},
			},
		);

		// Wait for WS to connect.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});

		// Deliver a status event over WS so wsStatusReceivedRef is set.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});

		// Simulate a stale REST refetch returning "pending".
		rerender({
			chatRecord: { ...makeChat(chatID), status: "pending" },
		});

		// The store must ignore the stale REST value because the
		// WS already delivered a status event for this chat.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});
	});
});

describe("thinking indicator event ordering", () => {
	it("shows starting phase when message_part arrives before status:running in same batch", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		immediateAnimationFrame();

		const chatID = "chat-thinking-parts-before-status";
		const userMsg = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...makeChat(chatID), status: "running" },
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useChatSelector(store, selectChatStatus),
					isAwaiting: useChatSelector(store, selectIsAwaitingFirstStreamChunk),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Server sends message_part BEFORE status:running in the same
		// WebSocket frame. This is the event ordering that previously
		// caused the "Thinking..." indicator to be skipped.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						part: { type: "reasoning", text: "Let me think..." },
					},
				},
				{
					type: "status",
					chat_id: chatID,
					status: { status: "running" },
				},
			]);
		});

		// After the batch, the status should be "running" but stream
		// parts should NOT have been applied yet (deferred to
		// setTimeout). This is the window where "Thinking..." shows.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
			expect(result.current.streamState).toBeNull();
			expect(result.current.isAwaiting).toBe(true);
		});

		// Let the deferred parts flush fire (setTimeout 0).
		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		// Now stream state should be populated.
		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
			expect(result.current.isAwaiting).toBe(false);
		});
	});

	it("shows starting phase when status:running arrives before message_part in same batch", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		immediateAnimationFrame();

		const chatID = "chat-thinking-status-before-parts";
		const userMsg = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...makeChat(chatID), status: "running" },
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useChatSelector(store, selectChatStatus),
					isAwaiting: useChatSelector(store, selectIsAwaitingFirstStreamChunk),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Server sends status:running BEFORE message_part (the "good" order).
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "status",
					chat_id: chatID,
					status: { status: "running" },
				},
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						part: { type: "text", text: "Hello" },
					},
				},
			]);
		});

		// Same contract: status set, parts deferred.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
			expect(result.current.streamState).toBeNull();
			expect(result.current.isAwaiting).toBe(true);
		});

		// Let the deferred parts flush fire.
		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
			expect(result.current.isAwaiting).toBe(false);
		});
	});

	it("discards buffered parts when status transitions to pending", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		immediateAnimationFrame();

		const chatID = "chat-thinking-discard-pending";
		const userMsg = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...makeChat(chatID), status: "running" },
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Server sends message_part then immediately transitions to pending.
		// The buffered parts must be discarded (not applied) because
		// pending status clears stream state.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						part: { type: "text", text: "partial response" },
					},
				},
				{
					type: "status",
					chat_id: chatID,
					status: { status: "pending" },
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("pending");
			expect(result.current.streamState).toBeNull();
		});

		// Even after timers fire, parts should not re-appear.
		await act(async () => {
			vi.advanceTimersByTime(50);
		});

		expect(result.current.streamState).toBeNull();
	});
});

describe("updateSidebarChat via stream events", () => {
	it("updates sidebar chat status on status stream event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-sidebar-status";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChat = makeChat(chatID);
		// Seed the chats list so updateSidebarChat can find it.
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "completed" },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].status).toBe("completed");
		});
	});

	it("does not change sidebar updated_at on message stream event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-sidebar-message";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChat = makeChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					orderedIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		const messageTimestamp = "2025-06-15T12:00:00.000Z";
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: {
					...makeMessage(chatID, 42, "assistant", "hello"),
					created_at: messageTimestamp,
				},
			});
		});

		// The per-chat WebSocket does not write updated_at — only the
		// global chat-list WebSocket delivers the authoritative server
		// timestamp. Verify it stays at the original value.
		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].updated_at).toBe(initialChat.updated_at);
		});
	});

	it("updates sidebar chat status to error on error stream event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-sidebar-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChat = makeChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "something went wrong", retryable: false },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].status).toBe("error");
		});
	});

	it("does not update sidebar for a different chatID", async () => {
		immediateAnimationFrame();

		const chatID = "chat-active";
		const otherChatID = "chat-other";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const activeChat = makeChat(chatID);
		const otherChat = makeChat(otherChatID);
		seedInfiniteChats(queryClient, [activeChat, otherChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: activeChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Emit a status event for the *active* chat.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "completed" },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.find((c) => c.id === chatID)?.status).toBe(
				"completed",
			);
		});

		// The other chat should remain unchanged.
		const sidebarChats = readInfiniteChats(queryClient);
		expect(sidebarChats?.find((c) => c.id === otherChatID)?.status).toBe(
			"running",
		);
	});

	it("does not regress updated_at on message events", async () => {
		immediateAnimationFrame();

		const chatID = "chat-no-regress-msg";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const futureTimestamp = "2099-01-01T00:00:00.000Z";
		const initialChat = { ...makeChat(chatID), updated_at: futureTimestamp };
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					orderedIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// The per-chat WS no longer writes updated_at, so any
		// message event should leave it untouched.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: {
					...makeMessage(chatID, 99, "assistant", "old message"),
					created_at: "2020-01-01T00:00:00.000Z",
				},
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].updated_at).toBe(futureTimestamp);
		});
	});

	it("does not change updated_at on status events", async () => {
		immediateAnimationFrame();

		const chatID = "chat-no-regress-status";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChat = makeChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "completed" },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			// Status should update, but updated_at must stay untouched.
			expect(sidebarChats?.[0].status).toBe("completed");
			expect(sidebarChats?.[0].updated_at).toBe(initialChat.updated_at);
		});
	});

	it("does not change updated_at on error events", async () => {
		immediateAnimationFrame();

		const chatID = "chat-no-regress-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});
		const initialChat = makeChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "something broke", retryable: false },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].status).toBe("error");
			expect(sidebarChats?.[0].updated_at).toBe(initialChat.updated_at);
		});
	});
});

describe("stream-to-durable transition (Bug 1)", () => {
	it("does not render both stream state and durable message after assistant message commits", async () => {
		immediateAnimationFrame();

		const chatID = "chat-b1-overlap";
		const userMsg = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					orderedIDs: useChatSelector(store, selectOrderedMessageIDs),
					messagesByID: useChatSelector(store, selectMessagesByID),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Build up streaming content.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "response" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "response" },
			]);
		});

		// Commit the assistant message as durable. With the old
		// code, streamState stayed non-null here because
		// clearStreamState was deferred to a rAF. Both the
		// durable message and the stream content coexisted,
		// causing duplicate rendering.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: makeMessage(chatID, 2, "assistant", "response"),
			});
		});

		// The durable message must be present AND streamState
		// must be null in the same snapshot.
		await waitFor(() => {
			expect(result.current.orderedIDs).toContain(2);
			expect(result.current.messagesByID.get(2)?.role).toBe("assistant");
			expect(result.current.streamState).toBeNull();
		});
	});

	it("no snapshot ever has both durable assistant and stream state", async () => {
		immediateAnimationFrame();

		const chatID = "chat-b1-atomic";
		const userMsg = makeMessage(chatID, 1, "user", "hi");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		// Track every snapshot emitted to subscribers.
		const snapshots: Array<{
			hasStream: boolean;
			hasDurableAssistant: boolean;
		}> = [];

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				const streamState = useChatSelector(store, selectStreamState);
				const messagesByID = useChatSelector(store, selectMessagesByID);
				const hasDurableAssistant = Array.from(messagesByID.values()).some(
					(m) => m.role === "assistant",
				);

				snapshots.push({
					hasStream: streamState !== null,
					hasDurableAssistant,
				});

				return { streamState, messagesByID };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "hello" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
		});

		// Clear snapshot history before the critical transition.
		snapshots.length = 0;

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: makeMessage(chatID, 2, "assistant", "hello"),
			});
		});

		await waitFor(() => {
			expect(result.current.messagesByID.has(2)).toBe(true);
		});

		// No snapshot should ever have BOTH a durable assistant
		// message AND non-null stream state.
		const overlapping = snapshots.filter(
			(s) => s.hasStream && s.hasDurableAssistant,
		);
		expect(overlapping).toEqual([]);
	});
});

describe("partsBuf cleanup on reconnect (Bug 2)", () => {
	it("discards stale buffered parts when the socket reconnects", async () => {
		immediateAnimationFrame();

		const chatID = "chat-b2-reconnect";
		const userMsg = makeMessage(chatID, 1, "user", "test");
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [userMsg],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Stream a message_part on the first socket.
		act(() => {
			mockSocket1.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "stale content" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "stale content" },
			]);
		});

		// Disconnect. The reconnecting websocket utility
		// schedules a reconnect after a 1s delay.
		act(() => {
			mockSocket1.emitError();
		});

		// Prepare the second socket and wait for the reconnect
		// timer to fire (real timers, ~1s).
		const mockSocket2 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket2);

		await waitFor(
			() => {
				expect(watchChat).toHaveBeenCalledTimes(2);
			},
			{ timeout: 3_000 },
		);

		// Open the new socket. This should clear stale state
		// including any buffered parts from socket1.
		act(() => {
			mockSocket2.emitOpen();
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});

		// Stream fresh content on the new socket.
		act(() => {
			mockSocket2.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "fresh content" },
				},
			});
		});

		// The stream should show only the new content, not a
		// mix of stale + fresh. With the old code, stale parts
		// from socket1 could leak into the new stream.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "fresh content" },
			]);
		});
	});
});

describe("store/cache desync protection", () => {
	it("does not wipe a message added via upsertDurableMessage when a genuine refetch follows", async () => {
		// RED TEST: Simulates what handleSend does today — it calls
		// store.upsertDurableMessage without writing to the React
		// Query cache. A subsequent rerender with new message refs
		// (genuine refetch) should NOT wipe the store-only message.
		immediateAnimationFrame();

		const chatID = "chat-send-desync";
		const msg1 = makeMessage(chatID, 1, "user", "hello");
		const msg2 = makeMessage(chatID, 2, "assistant", "hi");
		const msg3 = makeMessage(chatID, 3, "user", "follow-up");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatMessagesKey(chatID), {
			pages: [
				{
					messages: [msg2, msg1],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});
		const wrapper: FC<PropsWithChildren> = ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const initialMessages = [msg1, msg2];
		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: initialMessages,
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [] as TypesGen.ChatQueuedMessage[],
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
					store,
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2]);
		});

		act(() => {
			mockSocket.emitOpen();
		});

		// Simulate handleSend: write to store only, no cache write.
		act(() => {
			result.current.store.upsertDurableMessage(msg3);
		});

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
		});

		// Genuine refetch: new object refs for msg1 and msg2,
		// msg3 absent from the fetched set.
		const msg1New = makeMessage(chatID, 1, "user", "hello");
		const msg2New = makeMessage(chatID, 2, "assistant", "hi");
		rerender({
			...initialOptions,
			chatMessages: [msg1New, msg2New],
			chatMessagesData: {
				messages: [msg1New, msg2New],
				queued_messages: [],
				has_more: false,
			},
		});

		// msg3 was added to the store AFTER the last sync. It
		// should NOT be classified as stale — it's new, not
		// something the server removed.
		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
		});
	});

	it("still removes messages that were in the previous sync but are absent from a refetch (edit truncation)", async () => {
		immediateAnimationFrame();

		const chatID = "chat-edit-truncation";
		const msg1 = makeMessage(chatID, 1, "user", "hello");
		const msg2 = makeMessage(chatID, 2, "assistant", "hi");
		const msg3 = makeMessage(chatID, 3, "user", "more");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatMessagesKey(chatID), {
			pages: [
				{
					messages: [msg3, msg2, msg1],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});
		const wrapper: FC<PropsWithChildren> = ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const initialOptions = {
			chatID,
			chatMessages: [msg1, msg2, msg3],
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: [msg1, msg2, msg3],
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [] as TypesGen.ChatQueuedMessage[],
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
		});

		act(() => {
			mockSocket.emitOpen();
		});

		// Simulate edit truncation: rerender with only msg1.
		const msg1New = makeMessage(chatID, 1, "user", "hello");
		rerender({
			...initialOptions,
			chatMessages: [msg1New],
			chatMessagesData: {
				messages: [msg1New],
				queued_messages: [],
				has_more: false,
			},
		});

		// msg2 and msg3 WERE in the previous sync data and are
		// now absent — they are genuinely stale (edit truncation)
		// and should be removed.
		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1]);
		});
	});

	it("reflects optimistic and authoritative history-edit cache updates through the normal sync effect", async () => {
		immediateAnimationFrame();

		const chatID = "chat-local-edit-sync";
		const msg1 = makeMessage(chatID, 1, "user", "first");
		const msg2 = makeMessage(chatID, 2, "assistant", "second");
		const msg3 = makeMessage(chatID, 3, "user", "third");
		const optimisticReplacement = {
			...msg3,
			content: [{ type: "text" as const, text: "edited draft" }],
		};
		const authoritativeReplacement = makeMessage(chatID, 9, "user", "edited");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper: FC<PropsWithChildren> = ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const initialOptions = {
			chatID,
			chatMessages: [msg1, msg2, msg3],
			chatRecord: makeChat(chatID),
			chatMessagesData: {
				messages: [msg1, msg2, msg3],
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [] as TypesGen.ChatQueuedMessage[],
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(options);
				return {
					store,
					messagesByID: useChatSelector(store, selectMessagesByID),
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
		});

		act(() => {
			mockSocket.emitOpen();
		});

		rerender({
			...initialOptions,
			chatMessages: [msg1, msg2, optimisticReplacement],
			chatMessagesData: {
				messages: [msg1, msg2, optimisticReplacement],
				queued_messages: [],
				has_more: false,
			},
		});

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
			expect(result.current.messagesByID.get(3)?.content).toEqual(
				optimisticReplacement.content,
			);
		});

		rerender({
			...initialOptions,
			chatMessages: [msg1, msg2, authoritativeReplacement],
			chatMessagesData: {
				messages: [msg1, msg2, authoritativeReplacement],
				queued_messages: [],
				has_more: false,
			},
		});

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 9]);
			expect(result.current.messagesByID.has(3)).toBe(false);
			expect(result.current.messagesByID.get(9)?.content).toEqual(
				authoritativeReplacement.content,
			);
		});
	});
});

describe("parse errors", () => {
	it("surfaces parseError as streamError", async () => {
		immediateAnimationFrame();

		const chatID = "chat-parse-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamError: useChatSelector(store, selectStreamError),
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitParseError();
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Failed to parse chat stream update.",
			});
		});
		expect(result.current.chatStatus).not.toBe("error");
	});

	it("does not corrupt in-progress stream state", async () => {
		immediateAnimationFrame();

		const chatID = "chat-parse-no-corrupt";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					streamError: useChatSelector(store, selectStreamError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Build up some stream state first.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "partial response" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "partial response" },
			]);
		});

		// Fire a parse error and verify the existing stream blocks survive.
		act(() => {
			mockSocket.emitParseError();
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Failed to parse chat stream update.",
			});
		});
		expect(result.current.streamState?.blocks).toEqual([
			{ type: "response", text: "partial response" },
		]);
	});

	it("continues processing after parse error", async () => {
		immediateAnimationFrame();

		const chatID = "chat-parse-recover";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					streamError: useChatSelector(store, selectStreamError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Trigger a parse error first.
		act(() => {
			mockSocket.emitParseError();
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Failed to parse chat stream update.",
			});
		});

		// Send a valid message_part after the parse error.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "recovered" },
				},
			});
		});

		// The stream should process the new part normally.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "recovered" },
			]);
		});

		// streamError is sticky and is not cleared by valid messages.
		expect(result.current.streamError).toEqual({
			kind: "generic",
			message: "Failed to parse chat stream update.",
		});
	});
});
