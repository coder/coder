import {
	act,
	render,
	renderHook,
	screen,
	waitFor,
} from "@testing-library/react";
import { watchChat } from "#/api/api";
import { chatKeys } from "#/api/queries/chats";

const infiniteChatsTestKey = chatKeys.list();

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
import { useEffect } from "react";
import {
	QueryClient,
	QueryClientProvider,
	useInfiniteQuery,
	useQueryClient,
} from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	projectEditedConversationIntoCache,
	reconcileEditedMessageInCache,
} from "#/api/queries/chatMessageEdits";
import {
	chatMessagesForInfiniteScroll,
	selectDurableMessages,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	type ChatStore,
	resolveOverlayStreamState,
	selectFinalizingMessageID,
	selectFinalizingStreamState,
	selectQueuedMessages,
	selectReconnectState,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	useChatStore,
} from "./chatStore";
import {
	shouldSuppressFinalizedOverlay,
	useDurableChatStatus,
	useDurableMessageList,
	useDurableQueuedMessages,
	useIsAwaitingFirstStreamChunk,
} from "./durableChat";

vi.mock("#/api/api", () => ({
	watchChat: vi.fn(),
	API: {
		experimental: {
			getChatMessages: vi.fn(),
		},
	},
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

/**
 * Deliberately not the shared helper: that one uses gcTime 0, which evicts a
 * cache entry seeded without an observer before the socket can write to it.
 */
const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

/**
 * In the app the infinite messages query IS the source of the durable
 * messages, and both the socket and every reader go through its cache entry.
 * Tests pass the flat list as a prop, so mirror it into the cache the way the
 * query would, unless the test seeded the entry itself.
 */
const useTestChatStore: typeof useChatStore = (options) => {
	const queryClient = useQueryClient();
	const chatRecord = options.chatRecord;
	if (
		chatRecord &&
		queryClient.getQueryData(chatKeys.detail(chatRecord.id)) === undefined
	) {
		queryClient.setQueryData(chatKeys.detail(chatRecord.id), chatRecord);
	}
	if (
		options.chatID &&
		options.chatMessages &&
		queryClient.getQueryData(chatKeys.messages(options.chatID)) === undefined
	) {
		queryClient.setQueryData(chatKeys.messages(options.chatID), {
			pages: [
				{
					// The API returns each page newest-first.
					messages: [...options.chatMessages].sort(
						(left, right) => right.id - left.id,
					),
					queued_messages: options.chatQueuedMessages ?? [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});
	}
	return useChatStore(options);
};

const createWrapper =
	(queryClient: QueryClient): FC<PropsWithChildren> =>
	({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);

const buildChat = (
	chatID: string,
	overrides?: Partial<TypesGen.Chat>,
): TypesGen.Chat => ({
	...MockChat,
	id: chatID,
	owner_id: "owner-1",
	owner_username: "owner",
	owner_name: undefined,
	last_model_config_id: "model-1",
	title: "test",
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	snapshot_version: 1,
	...overrides,
});

/**
 * Chat status is canonical in the detail cache, so any test that asserts or
 * depends on it has to seed the entry the socket writes into.
 */
const seedChatDetail = (
	queryClient: QueryClient,
	chat: TypesGen.Chat,
): void => {
	queryClient.setQueryData<TypesGen.Chat>(chatKeys.detail(chat.id), chat);
};

const readChatDetail = (
	queryClient: QueryClient,
	chatID: string,
): TypesGen.Chat | undefined =>
	queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatID));

const buildMessage = (
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

const buildMessageWithContent = (
	chatID: string,
	id: number,
	role: TypesGen.ChatMessageRole,
	content: readonly TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	id,
	chat_id: chatID,
	created_at: "2025-01-01T00:00:00.000Z",
	role,
	content,
});

const buildQueuedMessage = (
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

	it("does not open the WebSocket when the AI Gateway is disabled", async () => {
		const chatID = "chat-gateway-disabled";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() =>
				useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: [existingMessage],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
					aiGatewayDisabled: true,
				}),
			{ wrapper },
		);

		expect(watchChat).not.toHaveBeenCalled();
	});

	it("keeps create_workspace durable call without result after preview_reset", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-preview-reset-create-workspace";
		const existingMessage = buildMessage(chatID, 1, "user", "create workspace");
		const assistantMessage = buildMessageWithContent(chatID, 2, "assistant", [
			{
				type: "tool-call",
				tool_call_id: "create-workspace-1",
				tool_name: "create_workspace",
				args: { name: "dev" },
			},
		]);
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
					messages: useDurableMessageList({ store, chatId: chatID }),
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
					part: {
						type: "tool-call",
						tool_call_id: "create-workspace-1",
						tool_name: "create_workspace",
						args: { name: "dev" },
					},
				},
			});
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		await waitFor(() => {
			expect(
				result.current.streamState?.toolCalls["create-workspace-1"]?.name,
			).toBe("create_workspace");
		});

		act(() => {
			mockSocket.emitDataBatch([
				{ type: "message", chat_id: chatID, message: assistantMessage },
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2]);
			expect(
				result.current.messages.find((message) => message.id === 2)?.content,
			).toEqual(assistantMessage.content);
		});
	});

	it("clears stream state when preview_reset arrives after durable tool result", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-preview-reset-tool-result";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const assistantMessage = buildMessageWithContent(chatID, 2, "assistant", [
			{
				type: "tool-call",
				tool_call_id: "tool-1",
				tool_name: "read_template",
				args: { template_id: "template-1" },
			},
		]);
		const toolMessage = buildMessageWithContent(chatID, 3, "tool", [
			{
				type: "tool-result",
				tool_call_id: "tool-1",
				tool_name: "read_template",
				result: { name: "Template" },
			},
		]);
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
					messages: useDurableMessageList({ store, chatId: chatID }),
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
					part: {
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "read_template",
						args: { template_id: "template-1" },
					},
				},
			});
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		await waitFor(() => {
			expect(result.current.streamState?.toolCalls["tool-1"]?.name).toBe(
				"read_template",
			);
		});

		act(() => {
			mockSocket.emitDataBatch([
				{ type: "message", chat_id: chatID, message: assistantMessage },
				{ type: "preview_reset", chat_id: chatID },
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						part: {
							type: "tool-result",
							tool_call_id: "tool-1",
							tool_name: "read_template",
							result: { name: "Template" },
						},
					},
				},
				{ type: "message", chat_id: chatID, message: toolMessage },
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
			expect(result.current.streamState).toBeNull();
		});
	});

	it("preview_reset discards pending buffered parts", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-preview-reset-buffer";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
						part: { type: "text", text: "stale" },
					},
				},
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		expect(result.current.streamState).toBeNull();
	});

	it("keeps only post-reset parts after preview_reset in one batch", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-preview-reset-post-part";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const durableMessage = buildMessage(chatID, 2, "assistant", "done");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
					messages: useDurableMessageList({ store, chatId: chatID }),
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
						part: { type: "text", text: "stale" },
					},
				},
				{ type: "message", chat_id: chatID, message: durableMessage },
				{ type: "preview_reset", chat_id: chatID },
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: { type: "text", text: "fresh" },
					},
				},
			]);
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2]);
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "fresh" },
			]);
		});
	});

	it("replaces messages after history_reset", async () => {
		const chatID = "chat-history-reset";
		const initialMessages = [
			buildMessage(chatID, 1, "user", "old prompt"),
			buildMessage(chatID, 2, "assistant", "old answer"),
			buildMessage(chatID, 3, "user", "stale prompt"),
		];
		const replacementMessage = buildMessage(chatID, 1, "user", "new prompt");
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
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [
				{
					messages: [...initialMessages].reverse(),
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: initialMessages,
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: initialMessages,
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
		});

		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: { type: "text", text: "stale" },
					},
				},
				{ type: "history_reset", chat_id: chatID },
				{ type: "message", chat_id: chatID, message: replacementMessage },
				// The server always emits preview_reset after a history
				// change in the same sync; it terminates the replacement run.
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1]);
			expect(result.current.messages.find((m) => m.id === 1)?.content).toEqual(
				replacementMessage.content,
			);
			expect(result.current.streamState).toBeNull();
		});

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatKeys.messages(chatID));
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			1,
		]);
	});

	it("buffers a history_reset replacement split across WS frames", async () => {
		const chatID = "chat-history-reset-split";
		const initialMessages = [
			buildMessage(chatID, 1, "user", "old prompt"),
			buildMessage(chatID, 2, "assistant", "old answer"),
			buildMessage(chatID, 3, "user", "stale prompt"),
		];
		const replacementOne = buildMessage(chatID, 1, "user", "new prompt");
		const replacementTwo = buildMessage(chatID, 2, "assistant", "new answer");
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
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [
				{
					messages: [...initialMessages].reverse(),
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: initialMessages,
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: initialMessages,
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
		});

		// Frame 1: history_reset plus only part of the replacement
		// history. The conversation must not blank or truncate while
		// the rest of the run is in flight.
		act(() => {
			mockSocket.emitDataBatch([
				{ type: "history_reset", chat_id: chatID },
				{ type: "message", chat_id: chatID, message: replacementOne },
			]);
		});
		expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);

		// Frame 2: the rest of the replacement, terminated by the
		// preview_reset the server emits in the same sync.
		act(() => {
			mockSocket.emitDataBatch([
				{ type: "message", chat_id: chatID, message: replacementTwo },
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2]);
			expect(
				result.current.messages.find((message) => message.id === 1)?.content,
			).toEqual(replacementOne.content);
			expect(
				result.current.messages.find((message) => message.id === 2)?.content,
			).toEqual(replacementTwo.content);
		});

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatKeys.messages(chatID));
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			2, 1,
		]);
	});

	it("clears stream state when a new durable message arrives", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const newMessage = buildMessage(chatID, 2, "assistant", "done");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		const existingMessage = buildMessage(chatID, 1, "assistant", "old");
		const updatedMessage = buildMessage(chatID, 1, "assistant", "updated");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		let streamRenderCount = 0;
		let queueRenderCount = 0;
		let durableMessagesRenderCount = 0;

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

		const DurableMessagesProbe: FC<{ store: ChatStoreHandle }> = ({
			store,
		}) => {
			useDurableMessageList({ store, chatId: chatID });
			durableMessagesRenderCount += 1;
			return null;
		};

		const TestHarness: FC = () => {
			const { store } = useTestChatStore({
				chatID,
				chatMessages: [existingMessage],
				chatRecord: buildChat(chatID),
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
					<DurableMessagesProbe store={store} />
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
		const durableMessagesBaseline = durableMessagesRenderCount;

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
		expect(durableMessagesRenderCount).toBe(durableMessagesBaseline);
	});

	it("applies batched message_part events from one payload", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

	it("ignores message_part updates while chat is waiting", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
				snapshot_version: 2,
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			// Stream state is preserved after status=waiting (the
			// durable message event handles cleanup via
			// needsStreamReset). Only new message_parts should be
			// blocked by the shouldApplyMessagePart gate.
			expect(result.current.streamState).not.toBeNull();
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "first" },
			]);
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
			// The late message_part should not be applied because
			// shouldApplyMessagePart gates on waiting.
			// Stream state still shows the original "first".
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "first" },
			]);
		});
	});

	it("does not restore stale queued messages after a stream queue_update", async () => {
		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();
		const initialOptions = {
			chatID,
			chatMessages: [existingMessage],
			chatRecord: buildChat(chatID),
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
				const { store } = useTestChatStore(options);
				return {
					queuedMessages: useDurableQueuedMessages({
						store,
						chatId: chatID,
					}),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		// Start with queued messages from a stale React Query cache.
		// This simulates coming back to a chat whose queue was drained
		// server-side while the user was viewing a different chat.
		const staleOptions = {
			chatID,
			chatMessages: [existingMessage],
			chatRecord: buildChat(chatID),
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
				const { store } = useTestChatStore(options);
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
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
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		}>(chatKeys.messages(chatID));
		expect(cachedData?.pages[0]?.queued_messages).toEqual([]);
	});

	it("caches the filtered queue when a queue_update still contains a suppressed message", async () => {
		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
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
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [queuedMessage],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					store,
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			result.current.store.suppressQueuedMessageID(queuedMessage.id);
		});
		act(() => {
			mockSocket.emitData({
				type: "queue_update",
				chat_id: chatID,
				queued_messages: [queuedMessage],
			});
		});

		await waitFor(() => {
			expect(result.current.queuedMessages).toEqual([]);
		});
		const cachedData = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatKeys.messages(chatID));
		expect(cachedData?.pages[0]?.queued_messages).toEqual([]);
	});

	it("writes WebSocket message events into the chat query cache", async () => {
		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
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
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		const newMessage = buildMessage(chatID, 2, "assistant", "hi there");
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: newMessage,
			});
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toContain(2);
		});

		// The React Query cache should also contain the new message.
		const cachedData = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatKeys.messages(chatID));
		const cachedMessages = cachedData?.pages[0]?.messages ?? [];
		// Verifies insertion, preservation, and DESC order.
		expect(cachedMessages.map((m) => m.id)).toEqual([2, 1]);
		// Emitting the same message again should not change the
		// cache reference (reference stability).
		const refBefore = queryClient.getQueryData(chatKeys.messages(chatID));
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: newMessage,
			});
		});
		const refAfter = queryClient.getQueryData(chatKeys.messages(chatID));
		expect(refAfter).toBe(refBefore);

		// Emitting the same message ID with different content should
		// update the cached entry (content-update path).
		const revised = buildMessage(chatID, 2, "assistant", "revised");
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
		}>(chatKeys.messages(chatID));
		const updatedFirst = updatedCache?.pages[0]?.messages[0];
		expect(updatedFirst?.content).toEqual([{ type: "text", text: "revised" }]);
	});

	it("closes old WebSocket and resets state when chatID changes", async () => {
		immediateAnimationFrame();

		const chatID1 = "chat-1";
		const chatID2 = "chat-2";
		const msg1 = buildMessage(chatID1, 1, "user", "hello");
		const msg2 = buildMessage(chatID2, 10, "user", "world");

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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: buildChat(chatID1),
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
				const { store } = useTestChatStore(options);
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
			chatRecord: buildChat(chatID2),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		const mismatchedMessage = buildMessage(
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
		const matchingMessage = buildMessage(
			chatID,
			2,
			"assistant",
			"correct chat",
		);
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
					message: buildMessage(chatID, 2, "assistant", "done"),
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
		const msg1 = buildMessage(chatID1, 1, "user", "hello");
		const msg2 = buildMessage(chatID2, 10, "user", "world");

		const mockSocket1 = createMockSocket();
		const mockSocket2 = createMockSocket();
		// Use a fallback so that extra effect re-runs get a valid socket.
		mockWatchChatReturn(mockSocket2);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: buildChat(chatID1),
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
				const { store } = useTestChatStore(options);
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
			chatRecord: buildChat(chatID2),
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
		const msg1 = buildMessage(chatID1, 1, "user", "first");
		const queuedMsg = buildQueuedMessage(chatID1, 10, "queued");

		const mockSocket1 = createMockSocket();
		const mockSocket2 = createMockSocket();
		// Use a fallback so that extra effect re-runs get a valid socket.
		mockWatchChatReturn(mockSocket2);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1)
			.mockReturnValueOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: buildChat(chatID1),
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
				const { store } = useTestChatStore(options);
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
			chatRecord: buildChat(chatID2),
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					snapshot_version: 3,
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

	// Status ordering: snapshot_version is the shared key every writer
	// compares against, so these cover apply / reject / duplicate.
	const renderStatusHarness = (chatID: string, queryClient: QueryClient) =>
		renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
					streamError: useChatSelector(store, selectStreamError),
				};
			},
			{ wrapper: createWrapper(queryClient) },
		);

	it("applies a status carrying a newer snapshot version", async () => {
		const chatID = "chat-status-newer";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "waiting", snapshot_version: 4 }),
		);
		seedInfiniteChats(queryClient, [
			buildChat(chatID, { status: "waiting", snapshot_version: 4 }),
		]);

		const { result } = renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 5,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});
		expect(readChatDetail(queryClient, chatID)?.snapshot_version).toBe(5);
		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			status: "running",
			snapshot_version: 5,
		});
	});

	it("rejects a delayed status event carrying an older snapshot version", async () => {
		const chatID = "chat-status-older";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "waiting", snapshot_version: 4 }),
		);

		const { result } = renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 7,
				status: { status: "running" },
			});
		});
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 5,
				status: { status: "waiting" },
			});
		});

		expect(readChatDetail(queryClient, chatID)).toMatchObject({
			status: "running",
			snapshot_version: 7,
		});
	});

	it("treats an equal-version status event as a duplicate no-op", async () => {
		const chatID = "chat-status-duplicate";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "running", snapshot_version: 6 }),
		);

		renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});
		const before = readChatDetail(queryClient, chatID);

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 6,
				status: { status: "waiting" },
			});
		});

		// Same version, same snapshot: nothing to apply, not even a new object.
		expect(readChatDetail(queryClient, chatID)).toBe(before);
	});

	it("keeps the detail query's dataUpdatedAt when a status lands", async () => {
		const chatID = "chat-status-freshness";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "waiting", snapshot_version: 4 }),
		);
		const seededUpdatedAt = queryClient.getQueryState(
			chatKeys.detail(chatID),
		)?.dataUpdatedAt;

		const { result } = renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 5,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});
		expect(
			queryClient.getQueryState(chatKeys.detail(chatID))?.dataUpdatedAt,
		).toBe(seededUpdatedAt);
	});

	it("reports a versionless transport error without writing chat status", async () => {
		const chatID = "chat-transport-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "running", snapshot_version: 4 }),
		);

		const { result } = renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "Subscription failed", retryable: false },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Subscription failed",
				retryable: false,
			});
		});
		// No snapshot backs the failure, so the chat is still running.
		expect(readChatDetail(queryClient, chatID)).toMatchObject({
			status: "running",
			snapshot_version: 4,
		});
	});

	it("keeps buffered parts, retry state, and the error reason for a superseded status", async () => {
		immediateAnimationFrame();

		const chatID = "chat-status-superseded-effects";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "running", snapshot_version: 7 }),
		);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID, {
						status: "running",
						snapshot_version: 7,
					}),
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
					retryState: useChatSelector(store, selectRetryState),
				};
			},
			{ wrapper: createWrapper(queryClient) },
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
		expect(result.current.retryState).not.toBeNull();

		// A delayed "waiting" from an older snapshot. The cache rejects it, so
		// the turn it would end is not this chat's current one and its stream
		// side effects must not run either.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 5,
				status: { status: "waiting" },
			});
		});
		await act(async () => {});

		expect(clearChatErrorReason).not.toHaveBeenCalled();
		expect(result.current.retryState).not.toBeNull();
		expect(readChatDetail(queryClient, chatID)).toMatchObject({
			status: "running",
			snapshot_version: 7,
		});

		// Buffered parts survive the same rejected status, so the response
		// keeps streaming.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						role: "assistant",
						part: { type: "text", text: "still streaming" },
					},
				},
				{
					type: "status",
					chat_id: chatID,
					snapshot_version: 5,
					status: { status: "waiting" },
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "still streaming" },
			]);
		});

		// An accepted status ends the turn, clearing the retry indicator that
		// belonged to it.
		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 3,
					error: "upstream timeout",
					kind: "timeout",
					provider: "anthropic",
					delay_ms: 5000,
					retrying_at: "2025-01-01T00:02:00.000Z",
				},
			});
		});
		await act(async () => {});
		expect(result.current.retryState).not.toBeNull();

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				snapshot_version: 8,
				status: { status: "waiting" },
			});
		});
		await waitFor(() => {
			expect(result.current.retryState).toBeNull();
		});
	});

	it("surfaces an error event sharing its status event's snapshot version", async () => {
		const chatID = "chat-error-companion";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "running", snapshot_version: 7 }),
		);
		const setChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID, {
						status: "running",
						snapshot_version: 7,
					}),
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason,
					clearChatErrorReason: vi.fn(),
				});
				return {
					streamError: useChatSelector(store, selectStreamError),
				};
			},
			{ wrapper: createWrapper(queryClient) },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// The server emits the status transition and the error detailing it
		// from one chat snapshot, so both carry the same version.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "status",
					chat_id: chatID,
					snapshot_version: 8,
					status: { status: "error" },
				},
				{
					type: "error",
					chat_id: chatID,
					snapshot_version: 8,
					error: { message: "Upstream failed", retryable: false },
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.streamError).toMatchObject({
				kind: "generic",
				message: "Upstream failed",
			});
		});
		expect(setChatErrorReason).toHaveBeenCalledWith(
			chatID,
			expect.objectContaining({ message: "Upstream failed" }),
		);
		expect(readChatDetail(queryClient, chatID)).toMatchObject({
			status: "error",
			snapshot_version: 8,
		});
	});

	it("does not install a stream error for a superseded error event", async () => {
		const chatID = "chat-error-superseded";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "running", snapshot_version: 7 }),
		);

		const { result } = renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				snapshot_version: 5,
				error: { message: "Old failure", retryable: false },
			});
		});

		// The chat already moved past that snapshot, so the failure it reports
		// is not the current one.
		expect(result.current.streamError).toBeNull();
		expect(readChatDetail(queryClient, chatID)).toMatchObject({
			status: "running",
			snapshot_version: 7,
		});
	});

	it("rejects an older error event while the chat is already in error", async () => {
		const chatID = "chat-error-older-than-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		// A newer error snapshot already landed, so only its own companion may
		// report a reason.
		seedChatDetail(
			queryClient,
			buildChat(chatID, { status: "error", snapshot_version: 8 }),
		);

		const { result } = renderStatusHarness(chatID, queryClient);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				snapshot_version: 5,
				error: { message: "Old failure", retryable: false },
			});
		});

		expect(result.current.streamError).toBeNull();
		expect(readChatDetail(queryClient, chatID)).toMatchObject({
			status: "error",
			snapshot_version: 8,
		});
	});

	it("does not gate the socket on messages alone", async () => {
		const chatID = "chat-first-fetch";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { rerender } = renderHook(
			({ chatRecord }: { chatRecord: TypesGen.Chat | undefined }) => {
				useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord,
					chatMessagesData: {
						messages: [],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return null;
			},
			{
				wrapper,
				initialProps: { chatRecord: undefined } as {
					chatRecord: TypesGen.Chat | undefined;
				},
			},
		);

		// Messages resolved, detail did not: a status event would have nowhere
		// to land, so the socket stays closed.
		expect(watchChat).not.toHaveBeenCalled();

		seedChatDetail(queryClient, buildChat(chatID));
		rerender({ chatRecord: buildChat(chatID) });

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});
	});

	it("sets chatStatus to error and populates streamError on error event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
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
				snapshot_version: 5,
				chat_id: chatID,
				error: {
					message: "Rate limit exceeded",
					detail: "Image exceeds 5 MB maximum.",
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
			detail: "Image exceeds 5 MB maximum.",
			provider: "anthropic",
			retryable: true,
			statusCode: 429,
		});
		expect(result.current.retryState).toBeNull();
		expect(setChatErrorReason).toHaveBeenCalledWith(chatID, {
			kind: "rate_limit",
			message: "Rate limit exceeded",
			detail: "Image exceeds 5 MB maximum.",
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
				snapshot_version: 6,
				chat_id: chatID,
				error: { message: "  ", retryable: false },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Chat processing failed.",
			});
		});
	});

	it("populates retryState on retry event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
			retryingAt: "2025-01-01T00:01:00.000Z",
		});
	});

	it("clears retryState when status transitions to running", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry-clear";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
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
				retryingAt: "2025-01-01T00:00:30.000Z",
			});
		});

		// Transition to running, which must clear retry state.
		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 7,
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
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
				snapshot_version: 8,
				chat_id: subagentChatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			expect(result.current.subagentStatusOverrides.get(subagentChatID)).toBe(
				"waiting",
			);
		});
		// Main chat status should remain "running" from the initial
		// chatRecord: the subagent status event must not change it.
		expect(result.current.chatStatus).toBe("running");
	});

	it("sets reconnectState on WebSocket disconnect and clears it after reconnect", async () => {
		immediateAnimationFrame();
		vi.spyOn(Math, "random").mockReturnValue(0.5);

		const chatID = "chat-disconnect";
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
				snapshot_version: 9,
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

	it("does not surface reconnectState for settled chats", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect-settled";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: { ...buildChat(chatID), status: "waiting" },
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
					reconnectState: useChatSelector(store, selectReconnectState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("waiting");
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitError();
		});

		await waitFor(() => {
			expect(result.current.reconnectState).toBeNull();
			expect(result.current.chatStatus).toBe("waiting");
		});
	});
	it("uses exponential backoff on consecutive disconnects", async () => {
		immediateAnimationFrame();

		const chatID = "chat-backoff";
		const watchMock = vi.mocked(watchChat);
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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

		// Second disconnect, so the reconnect lands after 2s.
		const socket2 = sockets[1]!;
		act(() => socket2.emitClose());

		await waitFor(() => expect(watchMock).toHaveBeenCalledTimes(3), {
			timeout: 5_000,
		});
	});

	it("passes latest message ID on reconnect for catch-up", async () => {
		immediateAnimationFrame();

		const chatID = "chat-catchup";
		const msg = buildMessage(chatID, 42, "assistant", "hello");
		const watchMock = vi.mocked(watchChat);
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useTestChatStore({
					chatID,
					chatMessages: [msg],
					chatRecord: buildChat(chatID),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const watchMock = vi.mocked(watchChat);

		// Return a fresh MockSocket for each connection attempt
		// so we can control the first and second sockets
		// independently.
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// Transition to running, which must call clearChatErrorReason.
		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 10,
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(clearChatErrorReason).toHaveBeenCalledWith(chatID);
		});
	});

	it("does not wipe WebSocket-delivered message when queue_update triggers cache change", async () => {
		immediateAnimationFrame();

		const chatID = "chat-queue-promote";
		const msg1 = buildMessage(chatID, 1, "user", "hello");
		const msg2 = buildMessage(chatID, 2, "assistant", "hi");
		// The promoted message that will arrive via WebSocket.
		const promotedMsg = buildMessage(chatID, 3, "user", "follow-up");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const queuedMsg = buildQueuedMessage(chatID, 10, "follow-up");
		const initialMessages = [msg1, msg2];

		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: buildChat(chatID),
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
				const { store } = useTestChatStore(options);
				return {
					messages: useDurableMessageList({ store, chatId: chatID }),
					queuedMessages: useChatSelector(store, selectQueuedMessages),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2]);
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

		// The promoted message must survive in the canonical cache with an
		// empty queue. The socket is the only writer of durable messages, so a
		// queue_update that rewrites the cached page must leave the promoted
		// message in place.
		//
		// Re-render so the updated query cache flows through the props the way
		// it does after a React Query cache mutation.
		rerender({
			...initialOptions,
			// The REST props were not refetched, so they still carry only
			// [msg1, msg2]: the promoted message arrived over the socket.
			// Spread into a new array to reproduce the new reference a queue
			// cache write hands to every reader.
			chatMessages: [...initialMessages],
			chatMessagesData: {
				messages: [...initialMessages],
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [],
		});
		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
			expect(result.current.queuedMessages).toHaveLength(0);
		});
	});

	it("does not wipe in-progress stream state when user message arrives in batch", async () => {
		immediateAnimationFrame();

		const chatID = "chat-promote-stream";
		const msg1 = buildMessage(chatID, 1, "user", "hello");
		const msg2 = buildMessage(chatID, 2, "assistant", "hi");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const queuedMsg = buildQueuedMessage(chatID, 10, "follow-up");
		const initialMessages = [msg1, msg2];

		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: buildChat(chatID),
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
				const { store } = useTestChatStore(options);
				return {
					store,
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2]);
		});

		// Open the WebSocket and set the chat to running.
		act(() => {
			mockSocket.emitOpen();
		});
		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 11,
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
		const promotedUser = buildMessage(chatID, 3, "user", "follow-up");

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
			expect(result.current.messages.map((m) => m.id)).toContain(3);
			expect(result.current.streamState).not.toBeNull();
			const blocks = result.current.streamState?.blocks ?? [];
			const textBlock = blocks.find((b) => b.type === "response");
			expect(textBlock).toBeDefined();
		});
	});

	it("ignores a stale chat record prop, the cache owns the status", async () => {
		immediateAnimationFrame();

		const chatID = "chat-stale-rest-status";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		// Start with a "running" chatRecord so the WS opens.
		const { result, rerender } = renderHook(
			(props: { chatRecord: TypesGen.Chat }) => {
				const { store } = useTestChatStore({
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
				};
			},
			{
				wrapper,
				initialProps: {
					chatRecord: buildChat(chatID),
				},
			},
		);

		// Wait for WS to connect.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});

		// Deliver a status event over the socket, which owns the cache entry.
		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 12,
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});

		// Simulate a stale REST refetch returning "waiting".
		rerender({
			chatRecord: { ...buildChat(chatID), status: "waiting" },
		});

		// The chat record is no longer a status source: the socket wrote the
		// status into the detail cache and only a newer snapshot version can
		// replace it. Guards against reintroducing a REST-to-store hydration.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});
		expect(readChatDetail(queryClient, chatID)?.snapshot_version).toBe(12);
	});

	it("preserves stream state when status transitions to waiting", async () => {
		immediateAnimationFrame();

		const chatID = "chat-preserve-stream";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		// Build up stream state with a message_part.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "thinking..." },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "thinking..." },
			]);
		});

		// Deliver a status=waiting event (interrupt). Stream state
		// should be preserved so the user continues to see the
		// partial response until the durable message arrives.
		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 13,
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "thinking..." },
			]);
		});
	});

	it("clears stream state when durable message follows waiting status", async () => {
		immediateAnimationFrame();

		const chatID = "chat-durable-clears";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Build up stream state.
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

		// Deliver status=waiting (interrupt). Stream state should be
		// preserved so the user continues to see the partial response
		// until the durable message arrives.
		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 14,
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		// Stream state must still be present after the status change.
		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "partial response" },
			]);
		});

		// Now deliver the durable assistant message. This should
		// clear stream state via the needsStreamReset path.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "partial response"),
			});
		});

		// Stream state should now be null and the durable message
		// should be in the message store.
		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
			expect(result.current.messages.map((m) => m.id)).toContain(2);
		});
	});
});

describe("thinking indicator event ordering", () => {
	it("shows starting phase when message_part arrives before status:running in same batch", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		immediateAnimationFrame();

		const chatID = "chat-thinking-parts-before-status";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...buildChat(chatID), status: "running" },
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
					isAwaiting: useIsAwaitingFirstStreamChunk({
						store,
						chatId: chatID,
					}),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Server sends message_part BEFORE status:running in the same
		// WebSocket frame. This is the event ordering that previously
		// caused the Thinking indicator to be skipped.
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
					snapshot_version: 15,
					chat_id: chatID,
					status: { status: "running" },
				},
			]);
		});

		// After the batch, the status should be "running" but stream
		// parts should NOT have been applied yet (deferred to
		// setTimeout). This is the window where the Thinking indicator shows.
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
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...buildChat(chatID), status: "running" },
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
					isAwaiting: useIsAwaitingFirstStreamChunk({
						store,
						chatId: chatID,
					}),
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
					snapshot_version: 16,
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
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...buildChat(chatID), status: "running" },
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Server sends message_part then immediately transitions to
		// waiting. The buffered parts must be discarded (not applied)
		// because waiting status clears stream state.
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
					snapshot_version: 17,
					chat_id: chatID,
					status: { status: "waiting" },
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("waiting");
			expect(result.current.streamState).toBeNull();
		});

		// Even after timers fire, parts should not re-appear.
		await act(async () => {
			vi.advanceTimersByTime(50);
		});

		expect(result.current.streamState).toBeNull();
	});
});

describe("sidebar cache writes from stream events", () => {
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
		const initialChat = buildChat(chatID);
		// Seed the chats list so the status write can find the row.
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
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
				return { chatStatus: useDurableChatStatus({ store, chatId: chatID }) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 18,
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].status).toBe("waiting");
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
		const initialChat = buildChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
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
					messages: useDurableMessageList({ store, chatId: chatID }),
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
					...buildMessage(chatID, 42, "assistant", "hello"),
					created_at: messageTimestamp,
				},
			});
		});

		// The per-chat WebSocket does not write updated_at, only the
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
		const initialChat = buildChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
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
				return { chatStatus: useDurableChatStatus({ store, chatId: chatID }) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				snapshot_version: 19,
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
		const activeChat = buildChat(chatID);
		const otherChat = buildChat(otherChatID);
		seedInfiniteChats(queryClient, [activeChat, otherChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				useTestChatStore({
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
				snapshot_version: 20,
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.find((c) => c.id === chatID)?.status).toBe(
				"waiting",
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
		const initialChat = { ...buildChat(chatID), updated_at: futureTimestamp };
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
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
					messages: useDurableMessageList({ store, chatId: chatID }),
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
					...buildMessage(chatID, 99, "assistant", "old message"),
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
		const initialChat = buildChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
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
				return { chatStatus: useDurableChatStatus({ store, chatId: chatID }) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				snapshot_version: 21,
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			// Status should update, but updated_at must stay untouched.
			expect(sidebarChats?.[0].status).toBe("waiting");
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
		const initialChat = buildChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		renderHook(
			() => {
				const { store } = useTestChatStore({
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
				return { chatStatus: useDurableChatStatus({ store, chatId: chatID }) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				snapshot_version: 22,
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

describe("stream-to-durable finalization handoff", () => {
	type OverlayFrame = {
		overlayText: string | null;
		durableIDs: readonly number[];
	};

	const overlayTextOf = (
		state: ReturnType<typeof selectStreamState>,
	): string | null =>
		state === null
			? null
			: state.blocks
					.map((block) => ("text" in block ? block.text : ""))
					.join("");

	/**
	 * Stands in for LiveStreamTail: subscribes to the store for the overlay and
	 * takes the suppression decision from its durable-reading parent, resolving
	 * the visible overlay with the production rule.
	 */
	const TailFrameProbe: FC<{
		store: ChatStore;
		durableIDs: readonly number[];
		suppressFinalizedOverlay: boolean;
		frames: OverlayFrame[];
	}> = ({ store, durableIDs, suppressFinalizedOverlay, frames }) => {
		const streamState = useChatSelector(store, selectStreamState);
		const finalizingStreamState = useChatSelector(
			store,
			selectFinalizingStreamState,
		);
		const overlayText = overlayTextOf(
			resolveOverlayStreamState(
				streamState,
				finalizingStreamState,
				suppressFinalizedOverlay,
			),
		);
		// Record the COMMITTED frame, so a render React discards never counts
		// as something the user saw.
		useEffect(() => {
			frames.push({ overlayText, durableIDs });
		});
		return (
			<>
				<div data-testid="transcript">{durableIDs.join(",")}</div>
				<div data-testid="tail">{overlayText ?? ""}</div>
			</>
		);
	};

	/**
	 * Stands in for ChatPageTimeline: the only place the suppression decision is
	 * made, reading the finalizing ID and the cache-backed durable list in the
	 * same render.
	 */
	const FinalizationFrameHarness: FC<{
		chatID: string;
		initialMessages: readonly TypesGen.ChatMessage[];
		frames: OverlayFrame[];
		// Receives the pagination observer's fetchNextPage so a test can put a
		// history fetch in flight, which forces socket cache writes to buffer.
		paginationRef?: { current: (() => void) | null };
	}> = ({ chatID, initialMessages, frames, paginationRef }) => {
		const messagesQuery = useInfiniteQuery(
			chatMessagesForInfiniteScroll(chatID),
		);
		const { store } = useTestChatStore({
			chatID,
			chatMessages: [...initialMessages],
			chatRecord: buildChat(chatID),
			chatMessagesData: {
				messages: [...initialMessages].sort(
					(left, right) => right.id - left.id,
				),
				queued_messages: [],
				has_more: false,
			},
			chatQueuedMessages: [],
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		});
		const messages = useDurableMessageList({ store, chatId: chatID });
		const finalizingMessageID = useChatSelector(
			store,
			selectFinalizingMessageID,
		);
		const suppressFinalizedOverlay = shouldSuppressFinalizedOverlay(
			finalizingMessageID,
			messages,
		);
		// Same retirement the production parent performs.
		useEffect(() => {
			if (suppressFinalizedOverlay && finalizingMessageID !== null) {
				store.completeStreamFinalization(finalizingMessageID);
			}
		}, [store, suppressFinalizedOverlay, finalizingMessageID]);
		const fetchNextPage = messagesQuery.fetchNextPage;
		useEffect(() => {
			if (paginationRef) {
				paginationRef.current = () => {
					void fetchNextPage();
				};
			}
		}, [paginationRef, fetchNextPage]);
		return (
			<>
				<div data-testid="finalizing">{finalizingMessageID ?? ""}</div>
				<TailFrameProbe
					store={store}
					durableIDs={messages.map((message) => message.id)}
					suppressFinalizedOverlay={suppressFinalizedOverlay}
					frames={frames}
				/>
			</>
		);
	};

	it("shows the finalized tail exactly once in every committed frame", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-frames";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const frames: OverlayFrame[] = [];

		render(
			<FinalizationFrameHarness
				chatID={chatID}
				initialMessages={[userMsg]}
				frames={frames}
			/>,
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
					part: { type: "text", text: "response" },
				},
			});
		});

		await waitFor(() => {
			expect(screen.getByTestId("tail").textContent).toBe("response");
		});

		// Finalize: the cache write and the store's finalization land in one
		// batch, but the cache notification is a macrotask later.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "response"),
			});
		});

		await waitFor(() => {
			expect(screen.getByTestId("transcript").textContent).toBe("1,2");
		});

		// From the first frame that rendered the streamed tail onward, the
		// finalized content is visible in EXACTLY one place: the overlay, or the
		// durable transcript. Equality of the two booleans means a double render
		// (both) or a gap (neither).
		const handoffFrames = frames.slice(
			frames.findIndex((frame) => frame.overlayText === "response"),
		);
		expect(handoffFrames.length).toBeGreaterThan(1);
		const badFrames = handoffFrames.filter(
			(frame) =>
				(frame.overlayText === "response") === frame.durableIDs.includes(2),
		);
		expect(badFrames).toEqual([]);

		// The overlay ends cleared, with the durable message on screen, and the
		// handoff is retired instead of lingering into the idle chat.
		expect(screen.getByTestId("tail").textContent).toBe("");
		await waitFor(() => {
			expect(screen.getByTestId("finalizing").textContent).toBe("");
		});
		const lastFrame = frames.at(-1);
		expect(lastFrame?.overlayText).toBeNull();
		expect(lastFrame?.durableIDs).toContain(2);
	});

	it("starts a fresh overlay for the next turn instead of extending the finalized snapshot", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-next-turn";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const frames: OverlayFrame[] = [];

		render(
			<FinalizationFrameHarness
				chatID={chatID}
				initialMessages={[userMsg]}
				frames={frames}
			/>,
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
					part: { type: "text", text: "response" },
				},
			});
		});
		await waitFor(() => {
			expect(screen.getByTestId("tail").textContent).toBe("response");
		});

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "response"),
			});
		});
		await waitFor(() => {
			expect(screen.getByTestId("transcript").textContent).toBe("1,2");
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "next turn" },
				},
			});
		});

		await waitFor(() => {
			expect(screen.getByTestId("tail").textContent).toBe("next turn");
		});

		// The finalized snapshot never accumulates the next turn's tokens, and
		// the durable transcript keeps the finalized message.
		expect(
			frames.some((frame) => frame.overlayText?.includes("responsenext turn")),
		).toBe(false);
		expect(screen.getByTestId("transcript").textContent).toBe("1,2");
	});

	// The bridge earns its keep here: the cache write buffers behind the
	// in-flight fetch, so the store records the finalization long before the
	// durable message is readable. Clearing the overlay on the store
	// notification alone would blank the tail until the fetch resolves.
	it("keeps the finalized tail visible while its cache write waits behind an in-flight fetch", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-during-fetch";
		const older = buildMessage(chatID, 30, "assistant", "older answer");
		const userMsg = buildMessage(chatID, 31, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [
				{
					messages: [userMsg, older],
					queued_messages: [],
					has_more: true,
				},
			],
			pageParams: [undefined],
		});

		const frames: OverlayFrame[] = [];
		const paginationRef: { current: (() => void) | null } = {
			current: null,
		};

		render(
			<FinalizationFrameHarness
				chatID={chatID}
				initialMessages={[older, userMsg]}
				frames={frames}
				paginationRef={paginationRef}
			/>,
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 31);
		});

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
			expect(screen.getByTestId("tail").textContent).toBe("response");
		});

		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			paginationRef.current?.();
		});
		await waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalled();
		});

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 32, "assistant", "response"),
			});
		});

		// Buffered, so the durable list cannot show it yet, and the overlay has
		// to carry the tail on its own.
		expect(screen.getByTestId("transcript").textContent).toBe("30,31");
		expect(screen.getByTestId("tail").textContent).toBe("response");

		await act(async () => {
			olderPage.resolve({
				messages: [buildMessage(chatID, 20, "user", "older")],
				queued_messages: [],
				has_more: false,
			});
			await olderPage.promise;
		});

		await waitFor(() => {
			expect(screen.getByTestId("transcript").textContent).toBe("20,30,31,32");
		});
		await waitFor(() => {
			expect(screen.getByTestId("tail").textContent).toBe("");
		});

		const handoffFrames = frames.slice(
			frames.findIndex((frame) => frame.overlayText === "response"),
		);
		const badFrames = handoffFrames.filter(
			(frame) =>
				(frame.overlayText === "response") === frame.durableIDs.includes(32),
		);
		expect(badFrames).toEqual([]);
		await waitFor(() => {
			expect(screen.getByTestId("finalizing").textContent).toBe("");
		});
	});

	// The real backend sequence: the durable `message` and the `preview_reset`
	// that retires its preview arrive in one frame, while a pagination fetch
	// holds the durable write in the serialization buffer. The overlay is the
	// only thing that can render the finalized turn until the fetch settles.
	it("shows the finalized tail exactly once for a [message, preview_reset] batch during a fetch", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-preview-reset-frames";
		const older = buildMessage(chatID, 30, "assistant", "older answer");
		const userMsg = buildMessage(chatID, 31, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [
				{ messages: [userMsg, older], queued_messages: [], has_more: true },
			],
			pageParams: [undefined],
		});

		const frames: OverlayFrame[] = [];
		const paginationRef: { current: (() => void) | null } = { current: null };

		render(
			<FinalizationFrameHarness
				chatID={chatID}
				initialMessages={[older, userMsg]}
				frames={frames}
				paginationRef={paginationRef}
			/>,
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 31);
		});

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
			expect(screen.getByTestId("tail").textContent).toBe("response");
		});

		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			paginationRef.current?.();
		});
		await waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalled();
		});

		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 32, "assistant", "response"),
				},
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		// The reset must not blank the tail: the durable write is buffered, so
		// the transcript cannot show the finalized turn yet.
		expect(screen.getByTestId("transcript").textContent).toBe("30,31");
		expect(screen.getByTestId("tail").textContent).toBe("response");

		await act(async () => {
			olderPage.resolve({
				messages: [buildMessage(chatID, 20, "user", "older")],
				queued_messages: [],
				has_more: false,
			});
			await olderPage.promise;
		});

		await waitFor(() => {
			expect(screen.getByTestId("transcript").textContent).toBe("20,30,31,32");
		});
		await waitFor(() => {
			expect(screen.getByTestId("tail").textContent).toBe("");
		});

		const handoffFrames = frames.slice(
			frames.findIndex((frame) => frame.overlayText === "response"),
		);
		const badFrames = handoffFrames.filter(
			(frame) =>
				(frame.overlayText === "response") === frame.durableIDs.includes(32),
		);
		expect(badFrames).toEqual([]);
		await waitFor(() => {
			expect(screen.getByTestId("finalizing").textContent).toBe("");
		});
	});

	// The finalizing snapshot is transient overlay state, so every path that
	// drops the overlay drops it too. These drive the real socket events rather
	// than the store methods, so a handler that forgets a path is caught.
	const renderFinalizationLifecycle = (
		chatID: string,
		queryClient: QueryClient,
		initialMessages: readonly TypesGen.ChatMessage[],
	) =>
		renderHook(
			({ activeChatID }: { activeChatID: string }) => {
				const { store } = useTestChatStore({
					chatID: activeChatID,
					chatMessages: [...initialMessages],
					chatRecord: buildChat(activeChatID),
					chatMessagesData: {
						messages: [...initialMessages],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					finalizingMessageID: useChatSelector(
						store,
						selectFinalizingMessageID,
					),
					finalizingStreamState: useChatSelector(
						store,
						selectFinalizingStreamState,
					),
				};
			},
			{
				wrapper: createWrapper(queryClient),
				initialProps: { activeChatID: chatID },
			},
		);

	it.each([
		[
			"history_reset",
			(chatID: string) => ({
				type: "history_reset" as const,
				chat_id: chatID,
			}),
		],
		[
			"retry",
			(chatID: string): TypesGen.ChatStreamEvent => ({
				type: "retry" as const,
				chat_id: chatID,
				retry: {
					attempt: 2,
					error: "upstream timeout",
					kind: "timeout",
					provider: "anthropic",
					delay_ms: 5000,
					retrying_at: "2025-01-01T00:01:00.000Z",
				},
			}),
		],
	])("clears the finalization handoff on %s", async (_label, buildEvent) => {
		immediateAnimationFrame();

		const chatID = `chat-finalize-clear-${_label}`;
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const { result } = renderFinalizationLifecycle(chatID, queryClient, [
			userMsg,
		]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

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
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "response"),
			});
		});
		expect(result.current.finalizingMessageID).toBe(2);
		expect(result.current.finalizingStreamState).not.toBeNull();

		act(() => {
			mockSocket.emitData(buildEvent(chatID));
		});

		expect(result.current.finalizingMessageID).toBeNull();
		expect(result.current.finalizingStreamState).toBeNull();
	});

	// The backend emits the durable `message` for a snapshot and then the
	// `preview_reset` that retires its preview. Clearing the overlay on that
	// reset would strand the tail that just finalized, so a pending handoff
	// survives it and is retired by identity instead.
	it("keeps the finalization handoff across a preview_reset in a later frame", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-preview-reset";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const { result } = renderFinalizationLifecycle(chatID, queryClient, [
			userMsg,
		]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

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
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "response"),
			});
		});
		expect(result.current.finalizingMessageID).toBe(2);

		// A separate frame, so the handler cannot see the durable message in
		// this batch and has to consult the store's pending handoff.
		act(() => {
			mockSocket.emitData({ type: "preview_reset", chat_id: chatID });
		});

		expect(result.current.finalizingMessageID).toBe(2);
		expect(result.current.finalizingStreamState).not.toBeNull();
	});

	it("clears the overlay on a preview_reset with no handoff pending", async () => {
		immediateAnimationFrame();

		const chatID = "chat-preview-reset-no-handoff";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const { result } = renderFinalizationLifecycle(chatID, queryClient, [
			userMsg,
		]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "discarded preview" },
				},
			});
		});
		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
		});

		act(() => {
			mockSocket.emitData({ type: "preview_reset", chat_id: chatID });
		});

		expect(result.current.streamState).toBeNull();
		expect(result.current.finalizingMessageID).toBeNull();
		expect(result.current.finalizingStreamState).toBeNull();
	});

	it("clears the finalization handoff when the chat changes", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-clear-switch";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const { result, rerender } = renderFinalizationLifecycle(
			chatID,
			queryClient,
			[userMsg],
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
					part: { type: "text", text: "response" },
				},
			});
		});
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "response"),
			});
		});
		expect(result.current.finalizingMessageID).toBe(2);

		rerender({ activeChatID: "chat-finalize-clear-switch-other" });

		expect(result.current.finalizingMessageID).toBeNull();
		expect(result.current.finalizingStreamState).toBeNull();
	});

	it("clears the finalization handoff when the socket reconnects", async () => {
		immediateAnimationFrame();

		const chatID = "chat-finalize-clear-reconnect";
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const firstSocket = createMockSocket();
		mockWatchChatReturnOnce(firstSocket);

		const queryClient = createTestQueryClient();
		const { result } = renderFinalizationLifecycle(chatID, queryClient, [
			userMsg,
		]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			firstSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "response" },
				},
			});
		});
		act(() => {
			firstSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "response"),
			});
		});
		expect(result.current.finalizingMessageID).toBe(2);

		act(() => {
			firstSocket.emitError();
		});
		const secondSocket = createMockSocket();
		mockWatchChatReturnOnce(secondSocket);
		await waitFor(
			() => {
				expect(watchChat).toHaveBeenCalledTimes(2);
			},
			{ timeout: 3_000 },
		);
		act(() => {
			secondSocket.emitOpen();
		});

		expect(result.current.finalizingMessageID).toBeNull();
		expect(result.current.finalizingStreamState).toBeNull();
	});
});

describe("partsBuf cleanup on reconnect (Bug 2)", () => {
	it("discards stale buffered parts when the socket reconnects", async () => {
		immediateAnimationFrame();

		const chatID = "chat-b2-reconnect";
		const userMsg = buildMessage(chatID, 1, "user", "test");
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: buildChat(chatID),
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

describe("durable edit reconciliation through the cache", () => {
	it("projects an optimistic edit and then the authoritative replacement", async () => {
		immediateAnimationFrame();

		const chatID = "chat-local-edit-sync";
		const msg1 = buildMessage(chatID, 1, "user", "first");
		const msg2 = buildMessage(chatID, 2, "assistant", "second");
		const msg3 = buildMessage(chatID, 3, "user", "third");
		const optimisticReplacement = {
			...msg3,
			content: [{ type: "text" as const, text: "edited draft" }],
		};
		// The backend soft-deletes the edited row and inserts a replacement with
		// a NEW ID, so reconciliation is by ID plus deleted_message_ids.
		const authoritativeReplacement = buildMessage(chatID, 9, "user", "edited");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [msg1, msg2, msg3],
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: [msg1, msg2, msg3],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
		});

		act(() => {
			mockSocket.emitOpen();
		});

		// Channel 1, first half: the optimistic projection truncates everything
		// at or above the edited ID and reinstates the edited message.
		act(() => {
			queryClient.setQueryData(
				chatKeys.messages(chatID),
				(current: MessagesCache | undefined) =>
					projectEditedConversationIntoCache({
						currentData: current,
						editedMessageId: msg3.id,
						replacementMessage: optimisticReplacement,
					}),
			);
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
			expect(
				result.current.messages.find((message) => message.id === 3)?.content,
			).toEqual(optimisticReplacement.content);
		});

		// Channel 1, second half: the response deletes the optimistic ID and
		// inserts the server's replacement.
		act(() => {
			queryClient.setQueryData(
				chatKeys.messages(chatID),
				(current: MessagesCache | undefined) =>
					reconcileEditedMessageInCache({
						currentData: current,
						optimisticMessageId: msg3.id,
						responseMessages: [authoritativeReplacement],
						deletedMessageIds: [msg3.id],
					}),
			);
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 9]);
			expect(result.current.messages.some((message) => message.id === 3)).toBe(
				false,
			);
			expect(
				result.current.messages.find((message) => message.id === 9)?.content,
			).toEqual(authoritativeReplacement.content);
		});

		// The socket echo of the same replacement is idempotent: the upsert is
		// keyed by ID, so the list does not grow a second copy.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: authoritativeReplacement,
			});
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 9]);
		});
	});

	it("stays idempotent when the socket echo arrives before the edit response", async () => {
		immediateAnimationFrame();

		const chatID = "chat-local-edit-echo-first";
		const msg1 = buildMessage(chatID, 1, "user", "first");
		const msg2 = buildMessage(chatID, 2, "assistant", "second");
		const msg3 = buildMessage(chatID, 3, "user", "third");
		const optimisticReplacement = {
			...msg3,
			content: [{ type: "text" as const, text: "edited draft" }],
		};
		const authoritativeReplacement = buildMessage(chatID, 9, "user", "edited");

		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [msg1, msg2, msg3],
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: [msg1, msg2, msg3],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					messages: useDurableMessageList({ store, chatId: chatID }),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3]);
		});

		act(() => {
			mockSocket.emitOpen();
		});

		act(() => {
			queryClient.setQueryData(
				chatKeys.messages(chatID),
				(current: MessagesCache | undefined) =>
					projectEditedConversationIntoCache({
						currentData: current,
						editedMessageId: msg3.id,
						replacementMessage: optimisticReplacement,
					}),
			);
		});

		// Channel 2 wins the race: the socket echoes the server's replacement
		// before the mutation response reconciles the optimistic ID away.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: authoritativeReplacement,
			});
		});

		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 3, 9]);
		});

		act(() => {
			queryClient.setQueryData(
				chatKeys.messages(chatID),
				(current: MessagesCache | undefined) =>
					reconcileEditedMessageInCache({
						currentData: current,
						optimisticMessageId: msg3.id,
						responseMessages: [authoritativeReplacement],
						deletedMessageIds: [msg3.id],
					}),
			);
		});

		// The late response removes the optimistic row and re-inserts the same
		// ID it already echoed, so the list neither duplicates nor loses it.
		await waitFor(() => {
			expect(result.current.messages.map((m) => m.id)).toEqual([1, 2, 9]);
			expect(
				result.current.messages.find((message) => message.id === 9)?.content,
			).toEqual(authoritativeReplacement.content);
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
					chatStatus: useDurableChatStatus({ store, chatId: chatID }),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useTestChatStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

// chatMessagesForInfiniteScroll uses staleTime: Infinity because the
// per-chat socket, not a background refetch, is what reconciles the cached
// pages. These cover the three resync paths that make the cached pages
// converge after reconnecting with a stale cache.
describe("message cache resync over the socket", () => {
	// The seeded cache entry has no observer (useChatStore writes to it but
	// never subscribes), so the shared helper's gcTime of 0 would collect it
	// before the assertions run.
	const createRetainedQueryClient = (): QueryClient =>
		new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});

	it("writes messages committed above the cached cursor into the cache", async () => {
		const chatID = "chat-1";
		const cachedMessage = buildMessage(chatID, 5, "user", "cached");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		const initialChatMessagesData: TypesGen.ChatMessagesResponse = {
			messages: [cachedMessage],
			queued_messages: [],
			has_more: false,
		};
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		renderHook(
			() =>
				useTestChatStore({
					chatID,
					chatMessages: [cachedMessage],
					chatRecord: buildChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		await waitFor(() => {
			// The socket subscribes with the cached cursor, so the server
			// replays only what was committed after it.
			expect(watchChat).toHaveBeenCalledWith(chatID, 5);
		});

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 6, "assistant", "replayed"),
			});
		});

		await waitFor(() => {
			const cachedData = queryClient.getQueryData<{
				pages: TypesGen.ChatMessagesResponse[];
				pageParams: unknown[];
			}>(chatKeys.messages(chatID));
			expect(cachedData?.pages[0]?.messages.map((m) => m.id)).toEqual([6, 5]);
		});
	});

	it("replaces cached messages edited or deleted below the cursor", async () => {
		const chatID = "chat-1";
		const cachedMessages = [
			buildMessage(chatID, 1, "user", "original"),
			buildMessage(chatID, 2, "assistant", "stale answer"),
		];
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		const initialChatMessagesData: TypesGen.ChatMessagesResponse = {
			messages: cachedMessages,
			queued_messages: [],
			has_more: false,
		};
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		renderHook(
			() =>
				useTestChatStore({
					chatID,
					chatMessages: cachedMessages,
					chatRecord: buildChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 2);
		});

		act(() => {
			mockSocket.emitDataBatch([
				{ type: "history_reset", chat_id: chatID },
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 1, "user", "edited"),
				},
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			const cachedData = queryClient.getQueryData<{
				pages: TypesGen.ChatMessagesResponse[];
				pageParams: unknown[];
			}>(chatKeys.messages(chatID));
			expect(cachedData?.pages[0]?.messages).toEqual([
				buildMessage(chatID, 1, "user", "edited"),
			]);
		});
	});

	it("applies the authoritative queue snapshot when the queue has drained", async () => {
		const chatID = "chat-1";
		const cachedMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		const initialChatMessagesData: TypesGen.ChatMessagesResponse = {
			messages: [cachedMessage],
			queued_messages: [queuedMessage],
			has_more: false,
		};
		queryClient.setQueryData(chatKeys.messages(chatID), {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		renderHook(
			() =>
				useTestChatStore({
					chatID,
					chatMessages: [cachedMessage],
					chatRecord: buildChat(chatID),
					chatMessagesData: initialChatMessagesData,
					chatQueuedMessages: [queuedMessage],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper: createWrapper(queryClient) },
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
			const cachedData = queryClient.getQueryData<{
				pages: TypesGen.ChatMessagesResponse[];
				pageParams: unknown[];
			}>(chatKeys.messages(chatID));
			expect(cachedData?.pages[0]?.queued_messages).toEqual([]);
		});
	});
});

type MessagesCache = {
	pages: TypesGen.ChatMessagesResponse[];
	pageParams: unknown[];
};

const readMessagesCache = (
	queryClient: QueryClient,
	chatID: string,
): MessagesCache | undefined =>
	queryClient.getQueryData<MessagesCache>(chatKeys.messages(chatID));

const messageIDsPerPage = (cache: MessagesCache | undefined): number[][] =>
	cache?.pages.map((page) => page.messages.map((message) => message.id)) ?? [];

const createDeferred = <T,>(): {
	promise: Promise<T>;
	resolve: (value: T) => void;
} => {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((resolvePromise) => {
		resolve = resolvePromise;
	});
	return { promise, resolve };
};

// The query cache, not the store, is canonical for durable messages: the
// socket is their single ordered writer, so its writes have to land in page 0
// without leaving stale cross-page copies, and they must not be clobbered by a
// paginating fetch that captured the pages before the write.
describe("durable messages in the query cache", () => {
	const createRetainedQueryClient = (): QueryClient =>
		new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
					gcTime: Number.POSITIVE_INFINITY,
					refetchOnWindowFocus: false,
					networkMode: "offlineFirst",
				},
			},
		});

	const seedMessagesCache = (
		queryClient: QueryClient,
		chatID: string,
		pages: MessagesCache["pages"],
		pageParams: MessagesCache["pageParams"],
	): void => {
		queryClient.setQueryData<MessagesCache>(chatKeys.messages(chatID), {
			pages,
			pageParams,
		});
	};

	const renderMessagesHarness = (
		queryClient: QueryClient,
		chatID: string,
		chatMessages: readonly TypesGen.ChatMessage[],
	) =>
		renderHook(
			() => {
				const messagesQuery = useInfiniteQuery(
					chatMessagesForInfiniteScroll(chatID),
				);
				useTestChatStore({
					chatID,
					chatMessages,
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: [...chatMessages],
						queued_messages: [],
						has_more: false,
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return messagesQuery;
			},
			{ wrapper: createWrapper(queryClient) },
		);

	// Exposes the hook's queue cache writer next to the pagination handle, so a
	// test can commit a queue write while a fetch is in flight.
	const renderQueueHarness = (
		queryClient: QueryClient,
		chatID: string,
		chatMessages: readonly TypesGen.ChatMessage[],
		chatQueuedMessages: readonly TypesGen.ChatQueuedMessage[],
	) =>
		renderHook(
			() => {
				const messagesQuery = useInfiniteQuery(
					chatMessagesForInfiniteScroll(chatID),
				);
				const { setCacheQueuedMessages } = useTestChatStore({
					chatID,
					chatMessages,
					chatRecord: buildChat(chatID),
					chatMessagesData: {
						messages: [...chatMessages],
						queued_messages: [...chatQueuedMessages],
						has_more: false,
					},
					chatQueuedMessages,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return { messagesQuery, setCacheQueuedMessages };
			},
			{ wrapper: createWrapper(queryClient) },
		);

	it("canonicalizes an updated message into page 0 and drops the older copy", async () => {
		const chatID = "chat-1";
		const newest = buildMessage(chatID, 30, "assistant", "newest");
		const staleOlder = buildMessage(chatID, 20, "user", "original");
		const oldest = buildMessage(chatID, 10, "user", "oldest");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[
				{ messages: [newest], queued_messages: [], has_more: true },
				{ messages: [staleOlder, oldest], queued_messages: [], has_more: true },
			],
			[undefined, 30],
		);

		renderMessagesHarness(queryClient, chatID, [oldest, staleOlder, newest]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 30);
		});

		const revised = buildMessage(chatID, 20, "user", "revised");
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: revised,
			});
		});

		await waitFor(() => {
			const cached = readMessagesCache(queryClient, chatID);
			// The authoritative values land in page 0 and the older page keeps
			// only the IDs it still owns, so the flatten cannot resurrect the
			// superseded copy.
			expect(messageIDsPerPage(cached)).toEqual([[30, 20], [10]]);
			expect(cached?.pages[0].messages[1]).toEqual(revised);
			expect(cached?.pageParams).toEqual([undefined, 30]);
			expect(
				cached && selectDurableMessages(cached).map((message) => message.id),
			).toEqual([10, 20, 30]);
		});
	});

	it("drops a page emptied by the upsert together with its page param", async () => {
		const chatID = "chat-1";
		const newest = buildMessage(chatID, 30, "assistant", "newest");
		const staleOlder = buildMessage(chatID, 20, "user", "original");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[
				{ messages: [newest], queued_messages: [], has_more: true },
				{ messages: [staleOlder], queued_messages: [], has_more: true },
			],
			[undefined, 30],
		);

		const { result } = renderMessagesHarness(queryClient, chatID, [
			staleOlder,
			newest,
		]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 30);
		});
		expect(result.current.hasNextPage).toBe(true);

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 20, "user", "revised"),
			});
		});

		await waitFor(() => {
			const cached = readMessagesCache(queryClient, chatID);
			expect(messageIDsPerPage(cached)).toEqual([[30, 20]]);
			expect(cached?.pageParams).toEqual([undefined]);
		});
		// An emptied last page would make getNextPageParam return undefined and
		// strand the rest of the history, so the page goes away with its cursor.
		expect(result.current.hasNextPage).toBe(true);
	});

	it("preserves page metadata and the queue when upserting a message", async () => {
		const chatID = "chat-1";
		const existing = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[
				{
					messages: [existing],
					queued_messages: [queuedMessage],
					has_more: true,
				},
			],
			[undefined],
		);

		renderMessagesHarness(queryClient, chatID, [existing]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: buildMessage(chatID, 2, "assistant", "hi"),
			});
		});

		await waitFor(() => {
			const cached = readMessagesCache(queryClient, chatID);
			expect(messageIDsPerPage(cached)).toEqual([[2, 1]]);
			expect(cached?.pages[0].queued_messages).toEqual([queuedMessage]);
			expect(cached?.pages[0].has_more).toBe(true);
		});
	});

	it("replays a socket append buffered during an in-flight fetchNextPage", async () => {
		const chatID = "chat-1";
		const newest = buildMessage(chatID, 30, "assistant", "newest");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[{ messages: [newest], queued_messages: [], has_more: true }],
			[undefined],
		);

		const { result } = renderMessagesHarness(queryClient, chatID, [newest]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 30);
		});

		// The fetch captures the pages as they are now and installs them
		// unconditionally when it resolves.
		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			void result.current.fetchNextPage();
		});
		await waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalled();
		});

		const committed = buildMessage(chatID, 31, "user", "committed mid-fetch");
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: committed,
			});
		});
		// Buffered rather than written, so the resolving fetch cannot discard it.
		expect(messageIDsPerPage(readMessagesCache(queryClient, chatID))).toEqual([
			[30],
		]);

		await act(async () => {
			olderPage.resolve({
				messages: [buildMessage(chatID, 20, "user", "older")],
				queued_messages: [],
				has_more: false,
			});
			await olderPage.promise;
		});

		await waitFor(() => {
			const cached = readMessagesCache(queryClient, chatID);
			// Both survive: the fetched page and the socket write replayed on top.
			expect(messageIDsPerPage(cached)).toEqual([[31, 30], [20]]);
		});
		expect(result.current.isFetchingNextPage).toBe(false);
		// And it stays: nothing re-runs the clobbering install afterwards.
		await waitFor(() => {
			expect(
				readMessagesCache(queryClient, chatID)?.pages[0].messages[0],
			).toEqual(committed);
		});
	});

	it("replays a history reset buffered during an in-flight fetchNextPage", async () => {
		const chatID = "chat-1";
		const newest = buildMessage(chatID, 30, "assistant", "stale answer");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[{ messages: [newest], queued_messages: [], has_more: true }],
			[undefined],
		);

		const { result } = renderMessagesHarness(queryClient, chatID, [newest]);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 30);
		});

		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			void result.current.fetchNextPage();
		});
		await waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalled();
		});

		const replacement = buildMessage(chatID, 1, "user", "edited");
		act(() => {
			mockSocket.emitDataBatch([
				{ type: "history_reset", chat_id: chatID },
				{ type: "message", chat_id: chatID, message: replacement },
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await act(async () => {
			olderPage.resolve({
				messages: [buildMessage(chatID, 20, "user", "older")],
				queued_messages: [],
				has_more: false,
			});
			await olderPage.promise;
		});

		await waitFor(() => {
			const cached = readMessagesCache(queryClient, chatID);
			// The replacement history is authoritative, so the page the fetch
			// installed is superseded rather than merged.
			expect(cached?.pages).toHaveLength(1);
			expect(cached?.pages[0].messages).toEqual([replacement]);
			expect(cached?.pageParams).toEqual([undefined]);
		});
	});

	// The queue snapshot lives on page 0 of the same cache entry, so it needs the
	// same serialization: a fetch that captured the pages before the delete
	// would reinstall the deleted entry, and the store would not correct it
	// because no queue_update arrived.
	it("keeps a queued-message delete committed during an in-flight fetchNextPage", async () => {
		const chatID = "chat-1";
		const newest = buildMessage(chatID, 30, "assistant", "newest");
		const keptQueued = buildQueuedMessage(chatID, 11, "kept");
		const deletedQueued = buildQueuedMessage(chatID, 10, "deleted");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[
				{
					messages: [newest],
					queued_messages: [deletedQueued, keptQueued],
					has_more: true,
				},
			],
			[undefined],
		);

		const { result } = renderQueueHarness(
			queryClient,
			chatID,
			[newest],
			[deletedQueued, keptQueued],
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 30);
		});

		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			void result.current.messagesQuery.fetchNextPage();
		});
		await waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalled();
		});

		// The optimistic delete the queued-message mutation performs.
		act(() => {
			result.current.setCacheQueuedMessages([keptQueued]);
		});
		expect(
			readMessagesCache(queryClient, chatID)?.pages[0].queued_messages,
		).toEqual([deletedQueued, keptQueued]);

		await act(async () => {
			olderPage.resolve({
				messages: [buildMessage(chatID, 20, "user", "older")],
				queued_messages: [],
				has_more: false,
			});
			await olderPage.promise;
		});

		await waitFor(() => {
			const cached = readMessagesCache(queryClient, chatID);
			// The delete survives the fetch instead of resurrecting.
			expect(cached?.pages[0].queued_messages).toEqual([keptQueued]);
			expect(messageIDsPerPage(cached)).toEqual([[30], [20]]);
		});
	});

	it("replays a queue_update buffered during an in-flight fetchNextPage", async () => {
		const chatID = "chat-1";
		const newest = buildMessage(chatID, 30, "assistant", "newest");
		const queued = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createRetainedQueryClient();
		seedMessagesCache(
			queryClient,
			chatID,
			[{ messages: [newest], queued_messages: [queued], has_more: true }],
			[undefined],
		);

		const { result } = renderQueueHarness(
			queryClient,
			chatID,
			[newest],
			[queued],
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 30);
		});

		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			void result.current.messagesQuery.fetchNextPage();
		});
		await waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalled();
		});

		act(() => {
			mockSocket.emitData({
				type: "queue_update",
				chat_id: chatID,
				queued_messages: [],
			});
		});
		expect(
			readMessagesCache(queryClient, chatID)?.pages[0].queued_messages,
		).toEqual([queued]);

		await act(async () => {
			olderPage.resolve({
				messages: [buildMessage(chatID, 20, "user", "older")],
				queued_messages: [],
				has_more: false,
			});
			await olderPage.promise;
		});

		await waitFor(() => {
			expect(
				readMessagesCache(queryClient, chatID)?.pages[0].queued_messages,
			).toEqual([]);
		});
	});
});
