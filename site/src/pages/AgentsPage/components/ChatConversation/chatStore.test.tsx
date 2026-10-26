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
import { createDeferred } from "#/testHelpers/deferred";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	type PendingAction,
	selectChatStatus,
	selectHasStreamState,
	selectIsAwaitingFirstStreamChunk,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectReconnectState,
	selectRetryState,
	selectShownError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	useChatStore,
} from "./chatStore";
import { deriveLiveStatus } from "./liveStatusModel";
import { buildStreamTools } from "./streamState";

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
	newestPages.set(chatID, { snapshot_version: 1, ...page });
};

beforeEach(() => {
	vi.mocked(API.experimental.getChatMessages).mockImplementation(
		async (chatID) =>
			newestPages.get(chatID) ?? {
				messages: [],
				queued_messages: [],
				has_more: false,
				snapshot_version: 1,
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

// The server sends the chat's status on connect and at every change.
const statusEvent = (
	chatID: string,
	status: TypesGen.ChatStatus = "running",
): TypesGen.ChatStreamEvent => ({
	type: "status",
	chat_id: chatID,
	status: { status },
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
				snapshot_version: 1,
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
				{
					type: "message",
					chat_id: chatID,
					message: assistantMessage,
					snapshot_version: 1,
				},
				{ type: "preview_reset", chat_id: chatID },
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
				{
					type: "message",
					chat_id: chatID,
					message: assistantMessage,
					snapshot_version: 1,
				},
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
				{
					type: "message",
					chat_id: chatID,
					message: toolMessage,
					snapshot_version: 1,
				},
				{ type: "preview_reset", chat_id: chatID },
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
				{
					type: "message",
					chat_id: chatID,
					message: durableMessage,
					snapshot_version: 1,
				},
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
				{ type: "history_reset", chat_id: chatID, snapshot_version: 1 },
				{
					type: "message",
					chat_id: chatID,
					message: replacementMessage,
					snapshot_version: 1,
				},
				{ type: "preview_reset", chat_id: chatID },
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

		// The second prompt was edited, so 5 and 6 replace 3 and 4. Each frame
		// applies when it arrives.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "history_reset",
					snapshot_version: 3,
					chat_id: chatID,
					history_reset: { from_message_id: 3 },
				},
				{
					type: "message",
					snapshot_version: 3,
					chat_id: chatID,
					message: buildMessage(chatID, 5, "user", "edited prompt"),
				},
			]);
		});
		expect(result.current).toEqual([1, 2, 5]);
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					snapshot_version: 4,
					chat_id: chatID,
					message: buildMessage(chatID, 6, "assistant", "edited answer"),
				},
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(result.current).toEqual([1, 2, 5, 6]);
		});
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
				snapshot_version: 1,
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
				snapshot_version: 1,
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

	it("drops message_part updates after the stream reports waiting", async () => {
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
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "first" },
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "first" },
			]);
		});

		// The stream reports waiting: the turn is over server-side.
		// A part arriving now comes from the closed episode draining
		// (the server keeps closed episodes subscribed for replay for
		// up to 15s) and must not repopulate the stream.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("waiting");
		});

		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "late drain" },
				},
			});
		});

		// Wait past the coalesced flush window so the drop is
		// observable rather than "not yet flushed".
		await act(async () => {
			await new Promise((resolve) => setTimeout(resolve, 10));
		});

		expect(result.current.streamState?.blocks).toEqual([
			{ type: "response", text: "first" },
		]);

		// The stream reporting running reopens the window: the next
		// turn's parts must flow again.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
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
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "firstnext turn" },
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
			expect(watchChat).toHaveBeenCalledWith(chatID2, 1);
		});

		// The old WebSocket was closed during effect cleanup.
		expect(mockSocket1.close).toHaveBeenCalled();
		// No stream state from chat-1 remains.
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "queue_update",
				snapshot_version: 1,
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

		// A message event for another chat must not clear the stream state.
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
				snapshot_version: 1,
			});
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "streaming" },
			]);
		});

		// An assistant message for this chat clears the stream state.
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
				snapshot_version: 1,
			});
		});

		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
		});
	});

	it("keeps a message_part that follows an assistant message in the same frame", async () => {
		immediateAnimationFrame();

		const chatID = "chat-raf";
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

		// The handler clears the stream state at the end of the frame for
		// the assistant message, then applies the part that followed it.
		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 2, "assistant", "done"),
					snapshot_version: 1,
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

		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
		});
	});

	it("shows only the new chat's state after a chat switch", async () => {
		immediateAnimationFrame();

		const chatID1 = "chat-1";
		const chatID2 = "chat-2";
		const sockets = mockWatchChatWithFreshSockets();
		const wrapper = createWrapper(createTestQueryClient());
		mockNewestPage(chatID1, {
			messages: [buildMessage(chatID1, 1, "user", "hello")],
			queued_messages: [buildQueuedMessage(chatID1, 10, "queued")],
			has_more: false,
		});
		mockNewestPage(chatID2, {
			messages: [buildMessage(chatID2, 10, "user", "world")],
			queued_messages: [],
			has_more: false,
		});
		const options = {
			chatID: chatID1,
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
		};
		const { result, rerender } = renderHook(
			(props: Parameters<typeof useChatStore>[0]) => {
				const { store } = useChatStore(props);
				return {
					messageIDs: useChatSelector(store, selectOrderedMessageIDs),
					queuedMessages: useChatSelector(store, selectQueuedMessages),
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ initialProps: options, wrapper },
		);
		await waitFor(() => expect(sockets).toHaveLength(1));
		act(() => {
			sockets[0].emitData({
				type: "message_part",
				chat_id: chatID1,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "chat-1 output" },
				},
			});
		});
		await waitFor(() => {
			expect(result.current.streamState).not.toBeNull();
		});

		rerender({ ...options, chatID: chatID2 });

		await waitFor(() => expect(result.current.messageIDs).toEqual([10]));
		expect(result.current.queuedMessages).toEqual([]);
		expect(result.current.streamState).toBeNull();
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// The waiting status discards the part buffered before it in the
		// same frame.
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					chatStatus: useChatSelector(store, selectChatStatus),
					streamError: useChatSelector(store, selectShownError),
					retryState: useChatSelector(store, selectRetryState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
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

	// Regression coverage for a chat that stopped with an error but kept the
	// streamed tool spinning. The error event must clear the partial stream.
	it("clears the in-flight streamed tool after a terminal error event", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-error-clears-running-tool";
		const existingMessage = buildMessage(
			chatID,
			1,
			"user",
			"create a workspace",
		);
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
					store,
					hasStreamState: useChatSelector(store, selectHasStreamState),
					chatStatus: useChatSelector(store, selectChatStatus),
					streamState: useChatSelector(store, selectStreamState),
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// A create_workspace tool call begins but never gets a result.
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

		// A tool call with no result renders as running.
		await waitFor(() => {
			expect(
				result.current.streamState?.toolCalls["create-workspace-1"]?.name,
			).toBe("create_workspace");
		});
		expect(
			buildStreamTools(
				result.current.streamState?.toolCalls,
				result.current.streamState?.toolResults,
			),
		).toMatchObject([{ id: "create-workspace-1", status: "running" }]);

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: {
					message: "The chat session ended unexpectedly.",
					kind: "generic",
					retryable: false,
				},
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
			expect(result.current.streamError).toMatchObject({
				kind: "generic",
				message: "The chat session ended unexpectedly.",
			});
		});

		expect(result.current.streamState).toBeNull();
		expect(result.current.hasStreamState).toBe(false);
		expect(
			buildStreamTools(
				result.current.streamState?.toolCalls,
				result.current.streamState?.toolResults,
			),
		).toEqual([]);

		const liveStatus = deriveLiveStatus({
			streamState: result.current.streamState,
			retryState: null,
			reconnectState: null,
			streamError: result.current.streamError,
			persistedError: null,
			isAwaitingFirstStreamChunk: false,
			chatStatus: result.current.chatStatus,
		});
		expect(liveStatus.phase).toBe("failed");
		expect(liveStatus.hasAccumulatedOutput).toBe(false);
	});

	// A part already queued from the errored turn can still arrive after the
	// error and repopulate the stream. This is accepted because the gate that
	// would drop it cannot tell it apart from a part belonging to the next
	// turn. The error status and callout are preserved either way.
	it("keeps the error status when a late part arrives after a terminal error", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-error-late-parts";
		const existingMessage = buildMessage(
			chatID,
			1,
			"user",
			"create a workspace",
		);
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
					chatStatus: useChatSelector(store, selectChatStatus),
					streamState: useChatSelector(store, selectStreamState),
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: {
					message: "The chat session ended unexpectedly.",
					kind: "generic",
					retryable: false,
				},
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
			expect(result.current.streamState).toBeNull();
		});

		// A late part from the errored turn arrives in a later frame.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					part: { type: "text", text: "late partial output" },
				},
			});
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		// The part is kept and the error status and callout are preserved.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "late partial output" },
			]);
		});
		expect(result.current.chatStatus).toBe("error");
		expect(result.current.streamError).toMatchObject({
			kind: "generic",
			message: "The chat session ended unexpectedly.",
		});
	});

	// The next turn must stream again after an error. Its first part can
	// arrive before its running status because the server sends parts and
	// status on separate paths, so the part must be kept either way.
	it("re-enables streaming parts on the next turn after a terminal error", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-error-next-turn";
		const existingMessage = buildMessage(
			chatID,
			1,
			"user",
			"create a workspace",
		);
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
					chatStatus: useChatSelector(store, selectChatStatus),
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
				type: "error",
				chat_id: chatID,
				error: {
					message: "The chat session ended unexpectedly.",
					kind: "generic",
					retryable: false,
				},
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
			expect(result.current.streamState).toBeNull();
		});

		// The next turn's first part arrives before its running status.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					part: { type: "text", text: "retry output" },
				},
			});
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await act(async () => {
			vi.advanceTimersByTime(1);
		});

		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "retry output" },
			]);
		});
	});

	// A non-error status means a new turn began, so the prior error must be
	// cleared for the new stream to render.
	it("clears streamError when a non-error status arrives after an error", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-error-clears-on-running";
		const existingMessage = buildMessage(
			chatID,
			1,
			"user",
			"create a workspace",
		);
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
					chatStatus: useChatSelector(store, selectChatStatus),
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: {
					message: "The chat session ended unexpectedly.",
					kind: "generic",
					retryable: false,
				},
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).not.toBeNull();
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "running" },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toBeNull();
		});
		expect(result.current.chatStatus).toBe("running");
	});

	// A status from a turn that was already running must not clear a stored
	// request failure, because the failed prompt was never retried.
	it("keeps streamError when a running turn emits a non-error status", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-keeps-request-error";
		const existingMessage = buildMessage(
			chatID,
			1,
			"user",
			"create a workspace",
		);
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
					store,
					chatStatus: useChatSelector(store, selectChatStatus),
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// A queued send fails while the prior turn is still running.
		act(() => {
			result.current.store.setClientError({
				kind: "generic",
				message: "Prompt could not be sent.",
			});
		});
		expect(result.current.streamError).not.toBeNull();

		// The still-running turn completes and emits a non-error status.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "waiting" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("waiting");
		});
		expect(result.current.streamError).not.toBeNull();
	});

	// Another viewer's action can take an errored chat out of error, so the
	// new turn supersedes this tab's request failure too.
	it("clears a request failure when an errored chat starts a new turn", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });

		const chatID = "chat-request-error-clears-on-new-turn";
		const existingMessage = buildMessage(
			chatID,
			1,
			"user",
			"create a workspace",
		);
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		mockNewestPage(chatID, {
			messages: [existingMessage],
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
					store,
					chatStatus: useChatSelector(store, selectChatStatus),
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// The chat is in error, and this tab's send fails.
		act(() => {
			mockSocket.emitData({
				type: "status",
				chat_id: chatID,
				status: { status: "error" },
			});
		});
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
		});
		act(() => {
			result.current.store.setClientError({
				kind: "generic",
				message: "Prompt could not be sent.",
			});
		});
		expect(result.current.streamError).not.toBeNull();

		// Another viewer sends a message, and the chat starts a new turn.
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
		expect(result.current.streamError).toBeNull();
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
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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
		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

		const { result } = renderHook(
			() => {
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitDataBatch([
				statusEvent(chatID),
				{
					type: "status",
					chat_id: subagentChatID,
					status: { status: "waiting" },
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.subagentStatusOverrides.get(subagentChatID)).toBe(
				"waiting",
			);
		});
		// The subagent status event must not change the main chat's status.
		expect(result.current.chatStatus).toBe("running");
	});

	it("opens the stream at the page's snapshot version and reconnects at the highest event version", async () => {
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
			mockSocket2.emitDataBatch([
				{
					type: "message",
					chat_id: chatID,
					message: buildMessage(chatID, 2, "assistant", "hi"),
					snapshot_version: 7,
				},
				// A resent change below the held version does not lower it.
				{
					type: "queue_update",
					chat_id: chatID,
					queued_messages: [],
					snapshot_version: 6,
				},
				statusEvent(chatID),
			]);
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

	it("reopens a remounted chat at its last snapshot version without fetching its page again", async () => {
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
				type: "queue_update",
				chat_id: chatID,
				queued_messages: [],
				snapshot_version: 7,
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
		const newestPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			newestPage.promise,
		);

		const { result } = renderHook(
			() => {
				const { loadState } = useChatStore({
					chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return loadState;
			},
			{ wrapper },
		);

		expect(result.current.status).toBe("loading");
		expect(watchChat).not.toHaveBeenCalled();

		act(() => {
			newestPage.resolve({
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
			expect(result.current.loadState).toEqual({
				status: "failed",
				error: failure,
			});
		});
		expect(watchChat).not.toHaveBeenCalled();

		act(() => {
			result.current.retryLoad();
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 3);
		});
		expect(result.current.loadState).toEqual({ status: "loaded" });
	});

	it("does not use one chat's snapshot version for another chat", async () => {
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});
		act(() => {
			mockSocket1.emitData(statusEvent(chatID));
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
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					store,
					streamError: useChatSelector(store, selectShownError),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchMock).toHaveBeenCalledWith(chatID, 1);
		});

		const socket1 = sockets[0]!;
		act(() => {
			socket1.emitOpen();
			result.current.store.setClientError({
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
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamError: useChatSelector(store, selectShownError),
					reconnectState: useChatSelector(store, selectReconnectState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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

	it("does not surface reconnectState for settled chats", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect-settled";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});
		act(() => {
			mockSocket.emitData(statusEvent(chatID, "waiting"));
		});
		expect(result.current.chatStatus).toBe("waiting");

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

		// Second disconnect — reconnect after 2s.
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
		// updates from the reconnected socket
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
				const { store } = useChatStore({
					chatID,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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

		mockNewestPage(chatID, {
			messages: initialMessages,
			queued_messages: [queuedMsg],
			has_more: false,
		});
		const initialOptions = {
			chatID,
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

		// Trailing parts of the current response, then the promoted user
		// message, in one frame. Only an assistant message clears the
		// stream state.
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
					snapshot_version: 1,
				},
			]);
		});

		await waitFor(() => {
			expect(result.current.orderedMessageIDs).toContain(3);
			expect(result.current.streamState).not.toBeNull();
			const blocks = result.current.streamState?.blocks ?? [];
			const textBlock = blocks.find((b) => b.type === "response");
			expect(textBlock).toBeDefined();
		});
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
					orderedIDs: useChatSelector(store, selectOrderedMessageIDs),
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
				snapshot_version: 1,
			});
		});

		// Stream state should now be null and the durable message
		// should be in the message store.
		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
			expect(result.current.orderedIDs).toContain(2);
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

	it("discards buffered parts when status transitions to waiting", async () => {
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
		const initialChat = buildChat(chatID);
		// Seed the chats list so updateSidebarChat can find it.
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		const messageTimestamp = "2025-06-15T12:00:00.000Z";
		act(() => {
			mockSocket.emitData({
				type: "message",
				snapshot_version: 1,
				chat_id: chatID,
				message: {
					...buildMessage(chatID, 42, "assistant", "hello"),
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
		const initialChat = buildChat(chatID);
		seedInfiniteChats(queryClient, [initialChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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
		const activeChat = buildChat(chatID);
		const otherChat = buildChat(otherChatID);
		seedInfiniteChats(queryClient, [activeChat, otherChat]);

		const wrapper = createWrapper(queryClient);
		const setChatErrorReason = vi.fn();
		const clearChatErrorReason = vi.fn();

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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// Emit a status event for the *active* chat.
		act(() => {
			mockSocket.emitData({
				type: "status",
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		// The per-chat WS no longer writes updated_at, so any
		// message event should leave it untouched.
		act(() => {
			mockSocket.emitData({
				type: "message",
				snapshot_version: 1,
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitData({
				type: "status",
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
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
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
				message: buildMessage(chatID, 2, "assistant", "response"),
				snapshot_version: 1,
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
				message: buildMessage(chatID, 2, "assistant", "hello"),
				snapshot_version: 1,
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
				const { store } = useChatStore({
					chatID,
					setChatErrorReason,
					clearChatErrorReason,
				});
				return {
					streamError: useChatSelector(store, selectShownError),
					chatStatus: useChatSelector(store, selectChatStatus),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 1);
		});

		act(() => {
			mockSocket.emitParseError();
		});

		await waitFor(() => {
			expect(result.current.streamError).toEqual({
				kind: "generic",
				message: "Failed to parse a chat stream update. Reconnecting.",
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
					streamError: useChatSelector(store, selectShownError),
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
				message: "Failed to parse a chat stream update. Reconnecting.",
			});
		});
		expect(result.current.streamState?.blocks).toEqual([
			{ type: "response", text: "partial response" },
		]);
	});
});

describe("chat stream contract", () => {
	const chatID = "chat-contract";
	const message = (
		id: number,
		role: TypesGen.ChatMessageRole,
		snapshotVersion: number,
	): TypesGen.ChatStreamEvent => ({
		type: "message",
		chat_id: chatID,
		message: buildMessage(chatID, id, role, `message ${id}`),
		snapshot_version: snapshotVersion,
	});
	const part = (text: string): TypesGen.ChatStreamEvent => ({
		type: "message_part",
		chat_id: chatID,
		message_part: { role: "assistant", part: { type: "text", text } },
	});
	const retry: TypesGen.ChatStreamEvent = {
		type: "retry",
		chat_id: chatID,
		retry: {
			attempt: 2,
			delay_ms: 1000,
			error: "rate limited",
			retrying_at: "2025-01-01T00:00:01.000Z",
		},
	};
	const renderChat = (
		page: TypesGen.ChatMessagesResponse,
		errorReasons = { set: vi.fn(), clear: vi.fn() },
	) => {
		mockNewestPage(chatID, page);
		return renderHook(
			() => {
				const chat = useChatStore({
					chatID,
					setChatErrorReason: errorReasons.set,
					clearChatErrorReason: errorReasons.clear,
				});
				const { store } = chat;
				return {
					store,
					pendingCount: useChatSelector(
						store,
						(state) => state.pendingActions.length,
					),
					fetchMoreMessages: chat.fetchMoreMessages,
					isFetchingMoreMessages: chat.isFetchingMoreMessages,
					messageIDs: useChatSelector(store, selectOrderedMessageIDs),
					queuedIDs: useChatSelector(store, selectQueuedMessages).map(
						(message) => message.id,
					),
					status: useChatSelector(store, selectChatStatus),
					error: useChatSelector(store, selectShownError),
					retry: useChatSelector(store, selectRetryState),
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper: createWrapper(createTestQueryClient()) },
		);
	};
	const page = (
		messages: readonly TypesGen.ChatMessage[],
		snapshotVersion = 4,
	): TypesGen.ChatMessagesResponse => ({
		messages,
		queued_messages: [],
		has_more: false,
		snapshot_version: snapshotVersion,
	});

	// The reconnect resends the events at the synced version 4.
	it.each<{
		name: string;
		action: PendingAction;
		status: TypesGen.ChatStatus;
		replayed: TypesGen.ChatStreamEvent[];
		result: TypesGen.ChatStreamEvent[];
	}>([
		{
			name: "send",
			action: {
				kind: "send",
				message: buildMessage(chatID, 0, "user", "second"),
			},
			status: "waiting",
			replayed: [message(1, "user", 4)],
			result: [message(3, "user", 5)],
		},
		{
			name: "queued send",
			action: {
				kind: "send",
				message: buildMessage(chatID, 0, "user", "second"),
			},
			status: "running",
			replayed: [
				{
					type: "queue_update",
					chat_id: chatID,
					queued_messages: [],
					snapshot_version: 4,
				},
			],
			result: [
				{
					type: "queue_update",
					chat_id: chatID,
					queued_messages: [buildQueuedMessage(chatID, 10, "second")],
					snapshot_version: 5,
				},
			],
		},
		{
			name: "edit",
			action: {
				kind: "edit",
				editedMessageID: 1,
				message: buildMessage(chatID, 1, "user", "edited"),
			},
			status: "waiting",
			replayed: [
				{
					type: "history_reset",
					chat_id: chatID,
					history_reset: { from_message_id: 1 },
					snapshot_version: 4,
				},
				message(1, "user", 4),
			],
			result: [
				{
					type: "history_reset",
					chat_id: chatID,
					history_reset: { from_message_id: 1 },
					snapshot_version: 5,
				},
				message(3, "user", 5),
			],
		},
	])("keeps a pending $name through the events a reconnect resends", async ({
		action,
		status,
		replayed,
		result: committed,
	}) => {
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));
		act(() => {
			result.current.store.addPendingAction(action, status);
		});
		expect(result.current.pendingCount).toBe(1);

		act(() => {
			sockets[0].emitOpen();
			sockets[0].emitDataBatch(replayed);
		});
		expect(result.current.pendingCount).toBe(1);

		act(() => {
			sockets[0].emitDataBatch(committed);
		});
		expect(result.current.pendingCount).toBe(0);
	});

	it("shows a chat's pending send in no other chat", async () => {
		mockWatchChatWithFreshSockets();
		const otherChatID = "chat-contract-other";
		mockNewestPage(chatID, page([buildMessage(chatID, 1, "user", "prompt")]));
		mockNewestPage(
			otherChatID,
			page([buildMessage(otherChatID, 2, "user", "other prompt")]),
		);
		const { result, rerender } = renderHook(
			({ id }) => {
				const { store, loadState } = useChatStore({
					chatID: id,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				});
				return {
					store,
					loadState,
					messageIDs: useChatSelector(store, selectOrderedMessageIDs),
				};
			},
			{
				initialProps: { id: chatID },
				wrapper: createWrapper(createTestQueryClient()),
			},
		);
		await waitFor(() => expect(result.current.loadState.status).toBe("loaded"));
		const sentFrom = result.current.store;
		act(() => {
			sentFrom.addPendingAction(
				{ kind: "send", message: buildMessage(chatID, 0, "user", "sent") },
				"waiting",
			);
		});

		rerender({ id: otherChatID });
		await waitFor(() => expect(result.current.messageIDs).toEqual([2]));
	});

	it("applies each event when it arrives", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat({
			...page([
				buildMessage(chatID, 1, "user", "prompt"),
				buildMessage(chatID, 2, "assistant", "answer"),
			]),
			queued_messages: [buildQueuedMessage(chatID, 10, "next")],
		});
		await waitFor(() => expect(sockets).toHaveLength(1));

		// A promote of the queued message, split across two frames.
		act(() => {
			sockets[0].emitData({
				type: "message",
				chat_id: chatID,
				message: {
					...buildMessage(chatID, 3, "user", "next"),
					queued_message_id: 10,
				},
				snapshot_version: 5,
			});
		});
		expect(result.current).toMatchObject({
			messageIDs: [1, 2, 3],
			queuedIDs: [10],
		});

		act(() => {
			sockets[0].emitData({
				type: "queue_update",
				chat_id: chatID,
				queued_messages: [],
				snapshot_version: 5,
			});
		});
		expect(result.current.queuedIDs).toEqual([]);
	});

	it("reconnects at the last applied version without the retry", async () => {
		vi.spyOn(Math, "random").mockReturnValue(0.5);
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));
		expect(watchChat).toHaveBeenLastCalledWith(chatID, 4);

		act(() => {
			sockets[0].emitOpen();
			sockets[0].emitDataBatch([
				message(2, "assistant", 5),
				statusEvent(chatID),
				retry,
			]);
		});
		expect(result.current.retry).not.toBeNull();

		act(() => {
			sockets[0].emitError();
		});
		await waitFor(() => expect(sockets).toHaveLength(2), { timeout: 3_000 });
		expect(watchChat).toHaveBeenLastCalledWith(chatID, 5);
		act(() => {
			sockets[1].emitOpen();
		});
		expect(result.current).toMatchObject({
			messageIDs: [1, 2],
			status: "running",
			retry: null,
		});
	});

	it("drops the retry on return to the chat", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const queryClient = createTestQueryClient();
		mockNewestPage(chatID, page([buildMessage(chatID, 1, "user", "prompt")]));
		const renderOnce = () =>
			renderHook(
				() => {
					const { store } = useChatStore({
						chatID,
						setChatErrorReason: vi.fn(),
						clearChatErrorReason: vi.fn(),
					});
					return useChatSelector(store, selectRetryState);
				},
				{ wrapper: createWrapper(queryClient) },
			);
		const first = renderOnce();
		await waitFor(() => expect(sockets).toHaveLength(1));
		act(() => {
			sockets[0].emitDataBatch([statusEvent(chatID), retry]);
		});
		expect(first.result.current).not.toBeNull();
		first.unmount();

		const second = renderOnce();
		expect(second.result.current).toBeNull();
	});

	it("resyncs from the last applied version when a frame fails to parse", async () => {
		vi.spyOn(Math, "random").mockReturnValue(0.5);
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));

		act(() => {
			sockets[0].emitOpen();
			sockets[0].emitData(message(2, "assistant", 5));
			sockets[0].emitParseError();
		});
		expect(sockets[0].close).toHaveBeenCalled();
		expect(result.current.error).toMatchObject({
			message: "Failed to parse a chat stream update. Reconnecting.",
		});
		act(() => {
			sockets[0].emitClose();
		});

		await waitFor(() => expect(sockets).toHaveLength(2), { timeout: 3_000 });
		expect(watchChat).toHaveBeenLastCalledWith(chatID, 5);
		act(() => {
			sockets[1].emitOpen();
		});
		expect(result.current.error).toBeNull();
	});

	it.each<TypesGen.ChatStreamEvent>([
		{
			type: "message",
			chat_id: chatID,
			message: buildMessage(chatID, 3, "user", "unversioned"),
		},
		{
			type: "queue_update",
			chat_id: chatID,
			queued_messages: [buildQueuedMessage(chatID, 10, "unversioned")],
		},
		{
			type: "history_reset",
			chat_id: chatID,
			history_reset: { from_message_id: 1 },
		},
	])("drops a frame with an unversioned $type event and reconnects without an error", async (unversioned) => {
		vi.spyOn(Math, "random").mockReturnValue(0.5);
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));

		act(() => {
			sockets[0].emitOpen();
			sockets[0].emitDataBatch([message(2, "assistant", 5), unversioned]);
		});
		expect(sockets[0].close).toHaveBeenCalled();
		expect(result.current.messageIDs).toEqual([1]);
		expect(result.current.queuedIDs).toEqual([]);
		expect(result.current.error).toBeNull();
		act(() => {
			sockets[0].emitClose();
		});

		await waitFor(() => expect(sockets).toHaveLength(2), { timeout: 3_000 });
		expect(watchChat).toHaveBeenLastCalledWith(chatID, 4);
		expect(result.current.error).toBeNull();
	});

	it("fails the load of a newest page without a version", async () => {
		mockWatchChatWithFreshSockets();
		mockNewestPage(chatID, {
			...page([buildMessage(chatID, 1, "user", "prompt")]),
			snapshot_version: undefined,
		});
		const { result } = renderHook(
			() =>
				useChatStore({
					chatID,
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
			{ wrapper: createWrapper(createTestQueryClient()) },
		);

		await waitFor(() => expect(result.current.loadState.status).toBe("failed"));
		expect(watchChat).not.toHaveBeenCalled();
	});

	it("keeps an error through an error status and a queue update", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));

		act(() => {
			sockets[0].emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "provider down", kind: "generic", retryable: false },
			});
			sockets[0].emitDataBatch([
				statusEvent(chatID, "error"),
				{
					type: "queue_update",
					chat_id: chatID,
					queued_messages: [],
					snapshot_version: 5,
				},
			]);
		});
		expect(result.current.error).toMatchObject({ message: "provider down" });
	});

	it("keeps a retry that its preview_reset follows", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));

		act(() => {
			sockets[0].emitDataBatch([
				statusEvent(chatID),
				retry,
				{ type: "preview_reset", chat_id: chatID },
			]);
		});
		expect(result.current.retry).toMatchObject({ attempt: 2 });
	});

	it("drops the partial reply at an error and at a retry", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat(
			page([buildMessage(chatID, 1, "user", "prompt")]),
		);
		await waitFor(() => expect(sockets).toHaveLength(1));
		const showPart = async () => {
			act(() => {
				sockets[0].emitDataBatch([statusEvent(chatID), part("partial")]);
			});
			await act(async () => {
				vi.advanceTimersByTime(1);
			});
			expect(result.current.streamState).not.toBeNull();
		};

		await showPart();
		act(() => {
			sockets[0].emitData(retry);
		});
		expect(result.current.streamState).toBeNull();

		await showPart();
		act(() => {
			sockets[0].emitDataBatch([
				statusEvent(chatID, "error"),
				{ type: "error", chat_id: chatID },
			]);
		});
		expect(result.current.streamState).toBeNull();
	});

	it("discards an older page requested before a history reset", async () => {
		const sockets = mockWatchChatWithFreshSockets();
		const { result } = renderChat({
			...page([buildMessage(chatID, 10, "user", "prompt")]),
			has_more: true,
		});
		await waitFor(() => expect(sockets).toHaveLength(1));

		const olderPage = createDeferred<TypesGen.ChatMessagesResponse>();
		vi.mocked(API.experimental.getChatMessages).mockReturnValueOnce(
			olderPage.promise,
		);
		act(() => {
			result.current.fetchMoreMessages();
		});
		await waitFor(() =>
			expect(result.current.isFetchingMoreMessages).toBe(true),
		);

		act(() => {
			sockets[0].emitDataBatch([
				{
					type: "history_reset",
					chat_id: chatID,
					history_reset: { from_message_id: 10 },
					snapshot_version: 5,
				},
				message(11, "user", 5),
			]);
		});
		await act(async () => {
			olderPage.resolve(page([buildMessage(chatID, 9, "assistant", "old")]));
		});

		await waitFor(() =>
			expect(result.current.isFetchingMoreMessages).toBe(false),
		);
		expect(result.current.messageIDs).toEqual([11]);
	});
});
