import { act, render, renderHook, waitFor } from "@testing-library/react";
import { API, watchChat } from "#/api/api";
import { chatListKey, toChatListParams } from "#/api/queries/chats";

const infiniteChatsTestKey = chatListKey(toChatListParams());

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

import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
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
	API: { experimental: { getChatMessages: vi.fn() } },
}));

// The newest page the server returns for each chat; a chat without one is
// empty.
const newestPages = new Map<string, TypesGen.ChatMessagesResponse>();
const mockNewestPage = (
	chatID: string,
	page: TypesGen.ChatMessagesResponse,
): void => {
	newestPages.set(chatID, page);
};

beforeEach(() => {
	vi.mocked(API.experimental.getChatMessages).mockImplementation(
		async (chatID) =>
			newestPages.get(chatID) ?? {
				messages: [],
				queued_messages: [],
				has_more: false,
			},
	);
});

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
		url: "ws://example.test/api/v2/chats/mock-stream",
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

const createWrapper =
	(queryClient: QueryClient): React.FC<React.PropsWithChildren> =>
	({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);

const buildChat = (chatID: string): TypesGen.Chat => ({
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
});

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

// Every sync the server sends ends with a checkpoint that carries the turn.
const checkpointEvent = (
	chatID: string,
	turn: TypesGen.ChatStreamTurn = { status: "running" },
	snapshotVersion = 1,
): TypesGen.ChatStreamEvent => ({
	type: "checkpoint",
	chat_id: chatID,
	checkpoint: { snapshot_version: snapshotVersion, turn },
});

const immediateAnimationFrame = (): void => {
	vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
		callback(0);
		return 1;
	});
	vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
};

afterEach(() => {
	newestPages.clear();
	vi.clearAllTimers();
	vi.useRealTimers();
	vi.restoreAllMocks();
	vi.mocked(watchChat).mockReset();
});

describe("useChatStore", () => {
	it("does not open the WebSocket when the AI Gateway is disabled", async () => {
		const chatID = "chat-gateway-disabled";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() =>
				useChatStore({
					chatID,
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					messagesByID: useChatSelector(store, selectMessagesByID),
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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
				checkpointEvent(chatID),
			]);
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
			expect(result.current.orderedMessageIDs).toEqual([1, 2]);
			expect(result.current.messagesByID.get(2)?.content).toEqual(
				assistantMessage.content,
			);
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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
				checkpointEvent(chatID),
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
				checkpointEvent(chatID),
			]);
		});

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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
				checkpointEvent(chatID),
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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
				checkpointEvent(chatID),
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
			expect(result.current.orderedMessageIDs).toEqual([1, 2]);
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		mockNewestPage(chatID, {
			messages: initialMessages,
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					messagesByID: useChatSelector(store, selectMessagesByID),
					orderedMessageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1, 2, 3]);
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
				{ type: "message", chat_id: chatID, message: replacementMessage }, // The server always emits preview_reset after a history
				// change in the same sync; it terminates the replacement run.
				{ type: "preview_reset", chat_id: chatID },
				checkpointEvent(chatID),
			]);
		});

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toEqual([1]);
			expect(result.current.messagesByID.get(1)?.content).toEqual(
				replacementMessage.content,
			);
			expect(result.current.streamState).toBeNull();
		});
	});

	it("keeps the messages below a history_reset's from_message_id", async () => {
		const chatID = "chat-history-reset-from";
		const initialMessages = [
			buildMessage(chatID, 1, "user", "first prompt"),
			buildMessage(chatID, 2, "assistant", "first answer"),
			buildMessage(chatID, 3, "user", "second prompt"),
			buildMessage(chatID, 4, "assistant", "second answer"),
		];
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = new QueryClient({
			defaultOptions: {
				queries: { retry: false, gcTime: Number.POSITIVE_INFINITY },
			},
		});
		const wrapper = createWrapper(queryClient);

		mockNewestPage(chatID, {
			messages: initialMessages,
			queued_messages: [],
			has_more: false,
			snapshot_version: 2,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return useChatSelector(store, selectOrderedMessageIDs);
			},
			{ wrapper },
		);
		await waitFor(() => {
			expect(result.current).toEqual([1, 2, 3, 4]);
		});

		// The second prompt was edited, so 5 and 6 replace 3 and 4. The reset
		// arrives split across two frames.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "history_reset",
					chat_id: chatID,
					history_reset: { from_message_id: 3 },
				},
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 5, "user", "edited prompt"),
				},
			]);
		});
		expect(result.current).toEqual([1, 2, 3, 4]);
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 6, "assistant", "edited answer"),
				},
				{ type: "preview_reset", chat_id: chatID },
				checkpointEvent(chatID),
			]);
		});

		await waitFor(() => {
			expect(result.current).toEqual([1, 2, 5, 6]);
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
		let orderedIDsRenderCount = 0;

		type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

		const StreamProbe: React.FC<{ store: ChatStoreHandle }> = ({ store }) => {
			useChatSelector(store, selectStreamState);
			streamRenderCount += 1;
			return null;
		};

		const QueueProbe: React.FC<{ store: ChatStoreHandle }> = ({ store }) => {
			useChatSelector(store, selectQueuedMessages);
			queueRenderCount += 1;
			return null;
		};

		const OrderedIDsProbe: React.FC<{ store: ChatStoreHandle }> = ({
			store,
		}) => {
			useChatSelector(store, selectOrderedMessageIDs);
			orderedIDsRenderCount += 1;
			return null;
		};

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const TestHarness: React.FC = () => {
			const { store } = useChatStore({
				chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

	it("applies message parts whatever the turn status", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		// The server decides which parts to send; the client shows each one.
		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
			turn: { status: "waiting" },
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: {
						type: "text",
						text: "live output",
					},
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "live output" },
			]);
		});
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

		mockNewestPage(chatID1, {
			messages: [msg1],
			queued_messages: [] as TypesGen.ChatQueuedMessage[],
			has_more: false,
		});
		const initialOptions = {
			chatID: chatID1,
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
			expect(watchChat).toHaveBeenCalledWith(chatID1, 0);
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

		mockNewestPage(chatID2, {
			messages: [msg2],
			queued_messages: [],
			has_more: false,
		});
		rerender({
			...initialOptions,
			chatID: chatID2,
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 0);
		});

		// The old WebSocket was closed during effect cleanup.
		expect(mockSocket1.close).toHaveBeenCalled();
		// No stream state from chat-1 carries over.
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [queuedMessage],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "queue_update",
					chat_id: otherChatID,
					queued_messages: [],
				},
				checkpointEvent(otherChatID),
			]);
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		// A message event with a mismatched chat_id is ignored.
		const mismatchedMessage = buildMessage(
			"chat-2",
			99,
			"assistant",
			"wrong chat",
		);
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: "chat-2",
					message: mismatchedMessage,
				},
				checkpointEvent("chat-2"),
			]);
		});

		// The mismatched event did not touch the stream state.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "streaming" },
			]);
		});

		// The chat's own sync applies, including the preview_reset the server
		// sends after a commit.
		const matchingMessage = buildMessage(
			chatID,
			2,
			"assistant",
			"correct chat",
		);
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: matchingMessage,
				},
				{ type: "preview_reset", chat_id: chatID },
				checkpointEvent(chatID),
			]);
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
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

		mockNewestPage(chatID1, {
			messages: [msg1],
			queued_messages: [] as TypesGen.ChatQueuedMessage[],
			has_more: false,
		});
		const initialOptions = {
			chatID: chatID1,
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
			expect(watchChat).toHaveBeenCalledWith(chatID1, 0);
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

		mockNewestPage(chatID2, {
			messages: [msg2],
			queued_messages: [],
			has_more: false,
		});
		rerender({
			...initialOptions,
			chatID: chatID2,
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 0);
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

		mockNewestPage(chatID1, {
			messages: [msg1],
			queued_messages: [queuedMsg],
			has_more: false,
		});
		const initialOptions = {
			chatID: chatID1,
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
			expect(watchChat).toHaveBeenCalledWith(chatID1, 0);
		});

		// Verify queued messages from chat-1 are present.
		expect(result.current.queuedMessages.map((m) => m.id)).toEqual([
			queuedMsg.id,
		]);

		// Switch to chat-2 with no messages and no queued messages
		// (simulating query not yet resolved for the new chat).
		mockNewestPage(chatID2, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		rerender({
			...initialOptions,
			chatID: chatID2,
		});

		// Chat-2 has its own store, so chat-1's queued messages are gone.
		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 0);
		});
		expect(result.current.queuedMessages).toEqual([]);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
			turn: { status: "running" },
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: subagentChatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			expect(result.current.subagentStatusOverrides.get(subagentChatID)).toBe(
				"waiting",
			);
		});
		// The subagent status event must not change the main chat's status.
		expect(result.current.chatStatus).toBe("running");
	});

	it("opens the stream at the page's snapshot version and reconnects at the last checkpoint version", async () => {
		immediateAnimationFrame();
		vi.spyOn(Math, "random").mockReturnValue(0.5);

		const chatID = "chat-snapshot-version";
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);
		const existingMessage = buildMessage(chatID, 1, "user", "hello");

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
			snapshot_version: 5,
		});
		renderHook(
			() => {
				useChatStore({
					chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 5);
		});

		const mockSocket2 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket2);
		act(() => {
			mockSocket1.emitError();
		});

		await waitFor(
			() => {
				expect(watchChat).toHaveBeenNthCalledWith(2, chatID, 5);
			},
			{ timeout: 3_000 },
		);

		act(() => {
			mockSocket2.emitOpen();
			mockSocket2.emitData({
				type: "checkpoint",
				chat_id: chatID,
				checkpoint: {
					snapshot_version: 7,
					turn: { status: "running" },
				},
			});
		});

		const mockSocket3 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket3);
		act(() => {
			mockSocket2.emitError();
		});

		await waitFor(
			() => {
				expect(watchChat).toHaveBeenNthCalledWith(3, chatID, 7);
			},
			{ timeout: 3_000 },
		);
	});

	it("reopens a remounted chat at the last checkpoint version without fetching its page again", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const chatID = "chat-remount-snapshot-version";
		const queryClient = new QueryClient({
			defaultOptions: {
				queries: { retry: false, gcTime: Number.POSITIVE_INFINITY },
			},
		});
		const wrapper = createWrapper(queryClient);
		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
			snapshot_version: 5,
		});
		const renderChat = () =>
			renderHook(
				() => {
					useChatStore({
						chatID,
						setChatErrorReason: vi.fn(),
						clearChatErrorReason: vi.fn(),
					});
				},
				{ wrapper },
			);

		const first = renderChat();
		await waitFor(() => {
			expect(watchChat).toHaveBeenLastCalledWith(chatID, 5);
		});
		act(() => {
			sockets[0].emitOpen();
			sockets[0].emitData({
				type: "checkpoint",
				chat_id: chatID,
				checkpoint: {
					snapshot_version: 7,
					turn: { status: "running" },
				},
			});
		});
		first.unmount();

		renderChat();
		await waitFor(() => {
			expect(watchChat).toHaveBeenLastCalledWith(chatID, 7);
		});
		expect(API.experimental.getChatMessages).toHaveBeenCalledTimes(1);
	});

	it("opens the stream only after the newest page has loaded", async () => {
		mockWatchChatWithFreshSockets();
		const chatID = "chat-loading-page";
		const wrapper = createWrapper(createTestQueryClient());
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		let resolvePage: (page: TypesGen.ChatMessagesResponse) => void = () => {};
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			new Promise((resolve) => {
				resolvePage = resolve;
			}),
		);

		const { result } = renderHook(
			() => {
				const { load } = useChatStore({
					chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return load;
			},
			{ wrapper },
		);

		expect(result.current.status).toBe("loading");
		expect(watchChat).not.toHaveBeenCalled();

		act(() => {
			resolvePage({
				messages: [existingMessage],
				queued_messages: [],
				has_more: false,
				snapshot_version: 5,
			});
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 5);
		});
		expect(result.current.status).toBe("loaded");
	});

	it("reports a failed load and loads again on retry", async () => {
		mockWatchChatWithFreshSockets();
		const chatID = "chat-load-fails";
		const wrapper = createWrapper(createTestQueryClient());
		const failure = new Error("unavailable");
		vi.mocked(API.experimental.getChatMessages).mockRejectedValueOnce(failure);
		mockNewestPage(chatID, {
			messages: [buildMessage(chatID, 1, "user", "hello")],
			queued_messages: [],
			has_more: false,
			snapshot_version: 3,
		});

		const { result } = renderHook(
			() =>
				useChatStore({
					chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.load).toEqual({ status: "failed", error: failure });
		});
		expect(watchChat).not.toHaveBeenCalled();

		act(() => {
			result.current.retryLoad();
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 3);
		});
		expect(result.current.load).toEqual({ status: "loaded" });
	});

	it("does not carry one chat's snapshot version into another chat", async () => {
		mockWatchChatWithFreshSockets();
		const wrapper = createWrapper(createTestQueryClient());
		mockNewestPage("chat-a", {
			messages: [buildMessage("chat-a", 1, "user", "hello")],
			queued_messages: [],
			has_more: false,
			snapshot_version: 100,
		});
		mockNewestPage("chat-b", {
			messages: [buildMessage("chat-b", 1, "user", "hello")],
			queued_messages: [],
			has_more: false,
			snapshot_version: 3,
		});

		const { rerender } = renderHook(
			(props: { chatID: string }) => {
				useChatStore({
					chatID: props.chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
			},
			{ wrapper, initialProps: { chatID: "chat-a" } },
		);
		await waitFor(() => {
			expect(watchChat).toHaveBeenLastCalledWith("chat-a", 100);
		});

		rerender({ chatID: "chat-b" });
		await waitFor(() => {
			expect(watchChat).toHaveBeenLastCalledWith("chat-b", 3);
		});
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
			turn: { status: "running" },
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchMock).toHaveBeenCalledWith(chatID, 0);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		// Set an error via an error stream event first.
		act(() => {
			mockSocket.emitData(
				checkpointEvent(chatID, {
					status: "error",
					error: { message: "Rate limit exceeded", retryable: false },
				}),
			);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
			turn: { status: "waiting" },
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(result.current.chatStatus).toBe("waiting");
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() =>
				useChatStore({
					chatID,
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

		// Second disconnect: reconnect after 2s.
		const socket2 = sockets[1]!;
		act(() => socket2.emitClose());

		await waitFor(() => expect(watchMock).toHaveBeenCalledTimes(3), {
			timeout: 5_000,
		});
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchMock).toHaveBeenCalledWith(chatID, 0);
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
});

describe("thinking indicator event ordering", () => {
	it("shows starting phase when a running checkpoint arrives before message_part in same batch", async () => {
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

		mockNewestPage(chatID, {
			messages: [userMsg],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		// The checkpoint reports running before the first message_part.
		act(() => {
			mockSocket.emitDataBatch([
				checkpointEvent(chatID, { status: "running" }),
				{
					type: "message_part",
					chat_id: chatID,
					message_part: {
						part: { type: "text", text: "Hello" },
					},
				},
			]);
		});

		// The status applies at once; the parts flush later.
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
});

describe("updateSidebarChat via stream events", () => {
	it("updates sidebar chat status at a checkpoint", async () => {
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
		// Seed the chats list so updateSidebarChat can find it.
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitData(checkpointEvent(chatID, { status: "waiting" }));
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		const messageTimestamp = "2025-06-15T12:00:00.000Z";
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: {
						...buildMessage(chatID, 42, "assistant", "hello"),
						created_at: messageTimestamp,
					},
				},
				checkpointEvent(chatID),
			]);
		});

		// The per-chat WebSocket does not write updated_at — only the
		// global chat-list WebSocket delivers the authoritative server
		// timestamp. Verify it stays at the original value.
		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].updated_at).toBe(initialChat.updated_at);
		});
	});

	it("updates sidebar chat status to error at an errored checkpoint", async () => {
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitData(
				checkpointEvent(chatID, {
					status: "error",
					error: { message: "something went wrong", retryable: false },
				}),
			);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		// A checkpoint for the *active* chat.
		act(() => {
			mockSocket.emitData(checkpointEvent(chatID, { status: "waiting" }));
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		// The per-chat WS no longer writes updated_at, so any
		// message event should leave it untouched.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: {
						...buildMessage(chatID, 99, "assistant", "old message"),
						created_at: "2020-01-01T00:00:00.000Z",
					},
				},
				checkpointEvent(chatID),
			]);
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			expect(sidebarChats?.[0].updated_at).toBe(futureTimestamp);
		});
	});

	it("does not change updated_at at a checkpoint", async () => {
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitData(checkpointEvent(chatID, { status: "waiting" }));
		});

		await waitFor(() => {
			const sidebarChats = readInfiniteChats(queryClient);
			// Status should update, but updated_at must stay untouched.
			expect(sidebarChats?.[0].status).toBe("waiting");
			expect(sidebarChats?.[0].updated_at).toBe(initialChat.updated_at);
		});
	});

	it("does not change updated_at at an errored checkpoint", async () => {
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return { chatStatus: useChatSelector(store, selectChatStatus) };
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
		});

		act(() => {
			mockSocket.emitData(
				checkpointEvent(chatID, {
					status: "error",
					error: { message: "something broke", retryable: false },
				}),
			);
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
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		mockNewestPage(chatID, {
			messages: [userMsg],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		// Commit the assistant message as durable. The server ends the
		// attempt's live output with a preview_reset in the same sync.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 2, "assistant", "response"),
				},
				{ type: "preview_reset", chat_id: chatID },
				checkpointEvent(chatID),
			]);
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
		const userMsg = buildMessage(chatID, 1, "user", "hi");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		// Track every snapshot emitted to subscribers.
		const snapshots: Array<{
			hasStream: boolean;
			hasDurableAssistant: boolean;
		}> = [];

		mockNewestPage(chatID, {
			messages: [userMsg],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 2, "assistant", "hello"),
				},
				{ type: "preview_reset", chat_id: chatID },
				checkpointEvent(chatID),
			]);
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
		const userMsg = buildMessage(chatID, 1, "user", "test");
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		mockNewestPage(chatID, {
			messages: [userMsg],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		mockNewestPage(chatID, {
			messages: [],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

		mockNewestPage(chatID, {
			messages: [existingMessage],
			queued_messages: [],
			has_more: false,
		});
		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 0);
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

describe("chat stream syncs", () => {
	const chatID = "chat-sync";
	const checkpoint = (
		snapshotVersion: number,
		turn: TypesGen.ChatStreamTurn = { status: "running" },
	): TypesGen.ChatStreamEvent => ({
		type: "checkpoint",
		chat_id: chatID,
		checkpoint: { snapshot_version: snapshotVersion, turn },
	});
	const renderChat = (
		page: TypesGen.ChatMessagesResponse,
		errorReasons = { set: vi.fn(), clear: vi.fn() },
	) => {
		mockNewestPage(chatID, page);
		return renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason: errorReasons.set,
					clearChatErrorReason: errorReasons.clear,
				});
				return {
					messageIDs: useChatSelector(store, selectOrderedMessageIDs),
					queuedIDs: useChatSelector(store, selectQueuedMessages).map(
						(message) => message.id,
					),
					status: useChatSelector(store, selectChatStatus),
					error: useChatSelector(store, selectStreamError),
					retry: useChatSelector(store, selectRetryState),
				};
			},
			{ wrapper: createWrapper(createTestQueryClient()) },
		);
	};

	it("shows a sync only at its checkpoint", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat({
			messages: [
				buildMessage(chatID, 1, "user", "prompt"),
				buildMessage(chatID, 2, "assistant", "answer"),
			],
			queued_messages: [buildQueuedMessage(chatID, 10, "next")],
			has_more: false,
			snapshot_version: 4,
		});
		await waitFor(() => expect(sockets).toHaveLength(1));

		// The turn ends and the queued message is promoted. The sync arrives
		// in two frames.
		act(() => {
			sockets[0].emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: {
						...buildMessage(chatID, 3, "user", "next"),
						queued_message_id: 10,
					},
				},
			]);
		});
		expect(result.current).toMatchObject({
			messageIDs: [1, 2],
			queuedIDs: [10],
		});

		act(() => {
			sockets[0].emitDataBatch([
				{ type: "queue_update", chat_id: chatID, queued_messages: [] },
				{ type: "preview_reset", chat_id: chatID },
				checkpoint(5),
			]);
		});
		expect(result.current).toMatchObject({
			messageIDs: [1, 2, 3],
			queuedIDs: [],
		});
	});

	it("drops a sync that the connection lost before its checkpoint", async () => {
		vi.spyOn(Math, "random").mockReturnValue(0.5);
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat({
			messages: [buildMessage(chatID, 1, "user", "prompt")],
			queued_messages: [],
			has_more: false,
			snapshot_version: 4,
		});
		await waitFor(() => expect(sockets).toHaveLength(1));

		act(() => {
			sockets[0].emitOpen();
			sockets[0].emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 2, "assistant", "partial sync"),
				},
			]);
			sockets[0].emitError();
		});
		expect(result.current.messageIDs).toEqual([1]);

		// The reconnect resumes from the last checkpoint and sends the sync
		// again.
		await waitFor(() => expect(sockets).toHaveLength(2), { timeout: 3_000 });
		expect(watchChat).toHaveBeenLastCalledWith(chatID, 4);
		act(() => {
			sockets[1].emitOpen();
			sockets[1].emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 2, "assistant", "answer"),
				},
				{ type: "preview_reset", chat_id: chatID },
				checkpoint(5),
			]);
		});
		expect(result.current.messageIDs).toEqual([1, 2]);
	});

	it("takes the turn from each checkpoint", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const errorReasons = { set: vi.fn(), clear: vi.fn() };
		const { result } = renderChat(
			{
				messages: [buildMessage(chatID, 1, "user", "prompt")],
				queued_messages: [],
				has_more: false,
				snapshot_version: 4,
			},
			errorReasons,
		);
		await waitFor(() => expect(sockets).toHaveLength(1));

		act(() => {
			sockets[0].emitData(
				checkpoint(5, {
					status: "running",
					retry: {
						attempt: 2,
						delay_ms: 1000,
						error: "rate limited",
						retrying_at: "2025-01-01T00:00:01.000Z",
					},
				}),
			);
		});
		expect(result.current).toMatchObject({
			status: "running",
			error: null,
			retry: { attempt: 2, error: "rate limited" },
		});

		act(() => {
			sockets[0].emitData(
				checkpoint(6, {
					status: "error",
					error: {
						message: "provider down",
						kind: "generic",
						retryable: false,
					},
				}),
			);
		});
		expect(result.current).toMatchObject({
			status: "error",
			error: { message: "provider down" },
			retry: null,
		});
		expect(errorReasons.set).toHaveBeenLastCalledWith(
			chatID,
			expect.objectContaining({ message: "provider down" }),
		);

		act(() => {
			sockets[0].emitData(checkpoint(7, { status: "waiting" }));
		});
		expect(result.current).toMatchObject({
			status: "waiting",
			error: null,
			retry: null,
		});
		expect(errorReasons.clear).toHaveBeenLastCalledWith(chatID);
	});

	it("takes the turn from the newest page before the stream connects", async () => {
		mockWatchChatWithFreshSockets();
		const { result } = renderChat({
			messages: [buildMessage(chatID, 1, "user", "prompt")],
			queued_messages: [],
			has_more: false,
			snapshot_version: 4,
			turn: {
				status: "error",
				error: { message: "provider down", kind: "generic", retryable: false },
			},
		});

		await waitFor(() => expect(result.current.messageIDs).toEqual([1]));
		expect(result.current).toMatchObject({
			status: "error",
			error: { message: "provider down" },
		});
	});
});
