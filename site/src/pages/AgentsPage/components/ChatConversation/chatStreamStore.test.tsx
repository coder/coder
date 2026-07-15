import { act, render, renderHook, waitFor } from "@testing-library/react";
import { watchChat } from "#/api/api";
import {
	chatMessagesForInfiniteScroll,
	chatQueryKeys,
	infiniteChats,
	selectChatMessagesProjection,
} from "#/api/queries/chats";

const infiniteChatsTestKey = infiniteChats().queryKey;

type ChatMessagesInfiniteData = {
	pages: TypesGen.ChatMessagesResponse[];
	pageParams: unknown[];
};

const seedChatMessages = (
	queryClient: QueryClient,
	chatID: string,
	messages: readonly TypesGen.ChatMessage[],
) => {
	queryClient.setQueryDefaults(chatMessagesForInfiniteScroll(chatID).queryKey, {
		gcTime: Number.POSITIVE_INFINITY,
	});
	queryClient.setQueryData<ChatMessagesInfiniteData>(
		chatMessagesForInfiniteScroll(chatID).queryKey,
		{
			pages: [
				{
					messages: [...messages].reverse(),
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		},
	);
};

const readChatMessages = (
	queryClient: QueryClient,
	chatID: string,
): readonly TypesGen.ChatMessage[] => {
	const data = queryClient.getQueryData<ChatMessagesInfiniteData>(
		chatMessagesForInfiniteScroll(chatID).queryKey,
	);
	return data ? selectChatMessagesProjection(data).messages : [];
};

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
import {
	QueryClient,
	QueryClientProvider,
	useInfiniteQuery,
	useQuery,
} from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	isAwaitingFirstStreamChunk,
	selectReconnectState,
	selectRetryState,
	selectStreamState,
	selectTransientError,
	useChatSelector,
	useChatStreamStore,
} from "./chatStreamStore";

const useCachedChatStatus = (chatID: string): TypesGen.ChatStatus | null =>
	useQuery({
		queryKey: chatQueryKeys.detail(chatID),
		queryFn: async () => buildChat(chatID),
		enabled: false,
	}).data?.status ?? null;

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

const createWrapper =
	(queryClient: QueryClient): FC<PropsWithChildren> =>
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

describe("useChatStreamStore", () => {
	it("does not clear in-progress stream parts for duplicate snapshot messages", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		seedChatMessages(queryClient, chatID, [existingMessage]);
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2]);
			expect(
				readChatMessages(queryClient, chatID).find(
					(message) => message.id === 2,
				)?.content,
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
		seedChatMessages(queryClient, chatID, [existingMessage]);
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2, 3]);
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		seedChatMessages(queryClient, chatID, [existingMessage]);
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2]);
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
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatID).queryKey, {
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: initialMessages,
					chatRecord: buildChat(chatID),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2, 3]);
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
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1]);
			expect(
				readChatMessages(queryClient, chatID).find(
					(message) => message.id === 1,
				)?.content,
			).toEqual(replacementMessage.content);
			expect(result.current.streamState).toBeNull();
		});

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll(chatID).queryKey);
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
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatID).queryKey, {
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

		renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: initialMessages,
					chatRecord: buildChat(chatID),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2, 3]);
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
		expect(
			readChatMessages(queryClient, chatID).map((message) => message.id),
		).toEqual([1, 2, 3]);

		// Frame 2: the rest of the replacement, terminated by the
		// preview_reset the server emits in the same sync.
		act(() => {
			mockSocket.emitDataBatch([
				{ type: "message", chat_id: chatID, message: replacementTwo },
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2]);
			expect(
				readChatMessages(queryClient, chatID).find(
					(message) => message.id === 1,
				)?.content,
			).toEqual(replacementOne.content);
			expect(
				readChatMessages(queryClient, chatID).find(
					(message) => message.id === 2,
				)?.content,
			).toEqual(replacementTwo.content);
		});

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll(chatID).queryKey);
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			2, 1,
		]);
	});

	it("replaces more than one stream batch and accepts IDs older than after_id", async () => {
		const chatID = "chat-history-reset-large";
		const initialMessage = buildMessage(
			chatID,
			500,
			"assistant",
			"old history",
		);
		const replacement = Array.from({ length: 300 }, (_, index) =>
			buildMessage(
				chatID,
				index + 1,
				index % 2 === 0 ? "user" : "assistant",
				`replacement ${index + 1}`,
			),
		);
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		seedChatMessages(queryClient, chatID, [initialMessage]);
		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [initialMessage],
					chatRecord: buildChat(chatID),
				}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, 500);
		});

		act(() => {
			mockSocket.emitDataBatch([
				{ type: "history_reset", chat_id: chatID },
				...replacement.slice(0, 255).map(
					(message): TypesGen.ChatStreamEvent => ({
						type: "message",
						chat_id: chatID,
						message,
					}),
				),
			]);
		});
		expect(
			readChatMessages(queryClient, chatID).map((message) => message.id),
		).toEqual([500]);

		act(() => {
			mockSocket.emitDataBatch([
				...replacement.slice(255).map(
					(message): TypesGen.ChatStreamEvent => ({
						type: "message",
						chat_id: chatID,
						message,
					}),
				),
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(readChatMessages(queryClient, chatID)).toHaveLength(300);
			expect(readChatMessages(queryClient, chatID).at(0)?.id).toBe(1);
			expect(readChatMessages(queryClient, chatID).at(-1)?.id).toBe(300);
		});
		const cached = queryClient.getQueryData<ChatMessagesInfiniteData>(
			chatMessagesForInfiniteScroll(chatID).queryKey,
		);
		expect(cached?.pages).toHaveLength(1);
		expect(cached?.pageParams).toEqual([undefined]);
		expect(cached?.pages[0]?.has_more).toBe(false);
	});

	it("discards an interrupted history replacement before reconnect", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const chatID = "chat-history-reset-interrupted";
		const initialMessage = buildMessage(chatID, 10, "assistant", "old history");
		const staleReplacement = buildMessage(
			chatID,
			1,
			"user",
			"stale replacement",
		);
		const freshReplacement = buildMessage(
			chatID,
			2,
			"user",
			"fresh replacement",
		);
		const sockets = mockWatchChatWithFreshSockets();
		const queryClient = createTestQueryClient();
		seedChatMessages(queryClient, chatID, [initialMessage]);
		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [initialMessage],
					chatRecord: buildChat(chatID),
				}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(sockets).toHaveLength(1);
		});
		const firstSocket = sockets[0]!;
		act(() => {
			firstSocket.emitDataBatch([
				{ type: "history_reset", chat_id: chatID },
				{ type: "message", chat_id: chatID, message: staleReplacement },
			]);
			firstSocket.emitClose();
			firstSocket.emitData({ type: "preview_reset", chat_id: chatID });
		});

		expect(readChatMessages(queryClient, chatID)).toEqual([initialMessage]);

		await act(async () => {
			vi.advanceTimersByTime(1_000);
		});
		await waitFor(() => {
			expect(sockets).toHaveLength(2);
		});
		const secondSocket = sockets[1]!;
		act(() => {
			secondSocket.emitOpen();
			secondSocket.emitDataBatch([
				{ type: "history_reset", chat_id: chatID },
				{ type: "message", chat_id: chatID, message: freshReplacement },
				{ type: "preview_reset", chat_id: chatID },
			]);
		});

		await waitFor(() => {
			expect(readChatMessages(queryClient, chatID)).toEqual([freshReplacement]);
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		let streamRenderCount = 0;
		let orderedIDsRenderCount = 0;

		type ChatStreamStoreHandle = ReturnType<typeof useChatStreamStore>["store"];

		const StreamProbe: FC<{ store: ChatStreamStoreHandle }> = ({ store }) => {
			useChatSelector(store, selectStreamState);
			streamRenderCount += 1;
			return null;
		};

		const OrderedIDsProbe: FC = () => {
			orderedIDsRenderCount += 1;
			return null;
		};

		const TestHarness: FC = () => {
			const { store } = useChatStreamStore({
				chatID,
				chatMessages: [existingMessage],
				chatRecord: buildChat(chatID),
			});
			return (
				<>
					<StreamProbe store={store} />
					<OrderedIDsProbe />
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatID).queryKey, {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
				}),
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

		const cachedData = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll(chatID).queryKey);
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
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatID).queryKey, {
			pages: [initialChatMessagesData],
			pageParams: [undefined],
		});

		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
				}),
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
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toContain(2);
		});

		// The React Query cache should also contain the new message.
		const cachedData = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll(chatID).queryKey);
		const cachedMessages = cachedData?.pages[0]?.messages ?? [];
		// Verifies insertion, preservation, and DESC order.
		expect(cachedMessages.map((m) => m.id)).toEqual([2, 1]);
		// Emitting the same message again should not change the
		// cache reference (reference stability).
		const refBefore = queryClient.getQueryData(
			chatMessagesForInfiniteScroll(chatID).queryKey,
		);
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: newMessage,
			});
		});
		const refAfter = queryClient.getQueryData(
			chatMessagesForInfiniteScroll(chatID).queryKey,
		);
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
		}>(chatMessagesForInfiniteScroll(chatID).queryKey);
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

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: buildChat(chatID1),
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStreamStore>[0]) => {
				const { store } = useChatStreamStore(options);
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
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 10);
		});
		const chat2Socket = vi.mocked(watchChat).mock.results.at(-1)?.value;
		expect(chat2Socket).toBeDefined();

		// The old WebSocket was closed during effect cleanup.
		expect(mockSocket1.close).toHaveBeenCalled();
		// Stream state was reset, with no stale stream data from chat-1.
		expect(result.current.streamState).toBeNull();

		act(() => {
			(chat2Socket as ReturnType<typeof createMockSocket>).emitData({
				type: "message_part",
				chat_id: chatID2,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "chat2-stream" },
				},
			});
		});
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "chat2-stream" },
			]);
		});
	});

	it("ignores queue_update events for other chats", async () => {
		const chatID = "chat-1";
		const otherChatID = "chat-2";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const queuedMessage = buildQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		seedChatMessages(queryClient, chatID, [existingMessage]);
		queryClient.setQueryData<ChatMessagesInfiniteData>(
			chatMessagesForInfiniteScroll(chatID).queryKey,
			(current) =>
				current
					? {
							...current,
							pages: [
								{ ...current.pages[0], queued_messages: [queuedMessage] },
								...current.pages.slice(1),
							],
						}
					: current,
		);
		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
				}),
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

		const cachedData = queryClient.getQueryData<ChatMessagesInfiniteData>(
			chatMessagesForInfiniteScroll(chatID).queryKey,
		);
		expect(cachedData?.pages[0]?.queued_messages).toEqual([queuedMessage]);
	});

	it("filters message events with mismatched chat_id", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

		const initialOptions = {
			chatID: chatID1,
			chatMessages: [msg1] as TypesGen.ChatMessage[],
			chatRecord: buildChat(chatID1),
		};

		const { result, rerender } = renderHook(
			(options: Parameters<typeof useChatStreamStore>[0]) => {
				const { store } = useChatStreamStore(options);
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
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2, 10);
		});

		expect(result.current.streamState).toBeNull();
	});

	it("does not apply message parts after status changes to waiting", async () => {
		immediateAnimationFrame();

		const chatID = "chat-status-guard";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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

	it("writes durable errors to exact detail without copying them into transient state", async () => {
		immediateAnimationFrame();
		const chatID = "chat-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);
		const queryClient = createTestQueryClient();
		const chatRecord = buildChat(chatID);
		queryClient.setQueryData(chatQueryKeys.detail(chatID), chatRecord);
		const wrapper = createWrapper(queryClient);
		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord,
				});
				return {
					chatStatus: useCachedChatStatus(chatID),
					transientError: useChatSelector(store, selectTransientError),
				};
			},
			{ wrapper },
		);
		await waitFor(() =>
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined),
		);
		const error: TypesGen.ChatError = {
			message: "Rate limit exceeded",
			detail: "Image exceeds 5 MB maximum.",
			kind: "rate_limit",
			provider: "anthropic",
			retryable: true,
			status_code: 429,
		};
		act(() => mockSocket.emitData({ type: "error", chat_id: chatID, error }));
		await waitFor(() => {
			expect(
				queryClient.getQueryData<TypesGen.Chat>(chatQueryKeys.detail(chatID)),
			).toMatchObject({
				status: "error",
				last_error: error,
			});
		});
		expect(result.current.chatStatus).toBe("error");
		expect(result.current.transientError).toBeNull();
	});

	it("populates retryState and clears it on preview_reset", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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

		act(() => {
			mockSocket.emitData({ type: "preview_reset", chat_id: chatID });
		});
		await waitFor(() => expect(result.current.retryState).toBeNull());

		act(() => {
			mockSocket.emitDataBatch([
				{
					type: "retry",
					chat_id: chatID,
					retry: {
						attempt: 3,
						error: "fresh snapshot retry",
						kind: "timeout",
						delay_ms: 1000,
						retrying_at: "2025-01-01T00:02:00.000Z",
					},
				},
				{ type: "preview_reset", chat_id: chatID },
			]);
		});
		await waitFor(() => {
			expect(result.current.retryState?.attempt).toBe(3);
		});

		const episodePart = {
			type: "message_part" as const,
			chat_id: chatID,
			message_part: {
				history_version: 5,
				generation_attempt: 1,
				seq: 1,
				part: { type: "text" as const, text: "episode" },
			},
		};
		act(() => mockSocket.emitData(episodePart));
		await waitFor(() => expect(result.current.retryState).toBeNull());

		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 4,
					error: "retry remains",
					kind: "timeout",
					delay_ms: 1000,
					retrying_at: "2025-01-01T00:03:00.000Z",
				},
			});
		});
		await waitFor(() => expect(result.current.retryState?.attempt).toBe(4));
		act(() => mockSocket.emitData(episodePart));
		await waitFor(() => expect(result.current.retryState?.attempt).toBe(4));
	});

	it("clears retryState when status transitions to running", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry-clear";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
				});
				return {
					retryState: useChatSelector(store, selectRetryState),
					chatStatus: useCachedChatStatus(chatID),
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
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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

	it("sets reconnectState on WebSocket disconnect and clears it after reconnect", async () => {
		immediateAnimationFrame();
		vi.spyOn(Math, "random").mockReturnValue(0.5);

		const chatID = "chat-disconnect";
		const mockSocket1 = createMockSocket();
		mockWatchChatReturnOnce(mockSocket1);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
				});
				return {
					chatStatus: useCachedChatStatus(chatID),
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
				});
				return {
					store,
					streamError: useChatSelector(store, selectTransientError),
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
			result.current.store.setTransientError({
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

	it("does not surface reconnect state after a durable stream error", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect-existing";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const chatRecord = buildChat(chatID);
		queryClient.setQueryData(chatQueryKeys.detail(chatID), chatRecord);
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord,
				});
				return {
					chatStatus: useCachedChatStatus(chatID),
					transientError: useChatSelector(store, selectTransientError),
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
			expect(result.current.chatStatus).toBe("error");
			expect(result.current.transientError).toBeNull();
			expect(result.current.reconnectState).toBeNull();
		});

		// WebSocket disconnect should not overwrite the terminal error
		// or surface reconnect state once the turn has already failed.
		act(() => {
			mockSocket.emitError();
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
			expect(result.current.transientError).toBeNull();
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
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: { ...buildChat(chatID), status: "waiting" },
				});
				return {
					chatStatus: useCachedChatStatus(chatID),
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
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
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
		const msg = buildMessage(chatID, 42, "assistant", "hello");
		const watchMock = vi.mocked(watchChat);
		const sockets = mockWatchChatWithFreshSockets(watchMock);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [msg],
					chatRecord: buildChat(chatID),
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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

	it("projects non-error status into exact detail", async () => {
		immediateAnimationFrame();

		const chatID = "chat-clear-error";
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
				});
				return {
					chatStatus: useCachedChatStatus(chatID),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});

		// A non-error status clears the persisted exact-detail error.
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

		const queuedMsg = buildQueuedMessage(chatID, 10, "follow-up");
		const initialMessages = [msg1, msg2];
		seedChatMessages(queryClient, chatID, initialMessages);

		queryClient.setQueryData<ChatMessagesInfiniteData>(
			chatMessagesForInfiniteScroll(chatID).queryKey,
			(current) =>
				current
					? {
							...current,
							pages: [
								{ ...current.pages[0], queued_messages: [queuedMsg] },
								...current.pages.slice(1),
							],
						}
					: current,
		);

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: initialMessages,
					chatRecord: buildChat(chatID),
				}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2]);
			expect(
				queryClient.getQueryData<ChatMessagesInfiniteData>(
					chatMessagesForInfiniteScroll(chatID).queryKey,
				)?.pages[0]?.queued_messages,
			).toEqual([queuedMsg]);
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

		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2, 3]);
			expect(
				queryClient.getQueryData<ChatMessagesInfiniteData>(
					chatMessagesForInfiniteScroll(chatID).queryKey,
				)?.pages[0]?.queued_messages,
			).toEqual([]);
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

		const initialMessages = [msg1, msg2];
		seedChatMessages(queryClient, chatID, initialMessages);

		const initialOptions = {
			chatID,
			chatMessages: initialMessages,
			chatRecord: buildChat(chatID),
		};

		const { result } = renderHook(
			(options: Parameters<typeof useChatStreamStore>[0]) => {
				const { store } = useChatStreamStore(options);
				return {
					store,
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useCachedChatStatus(chatID),
				};
			},
			{ initialProps: initialOptions, wrapper },
		);

		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toEqual([1, 2]);
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
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toContain(3);
			expect(result.current.streamState).not.toBeNull();
			const blocks = result.current.streamState?.blocks ?? [];
			const textBlock = blocks.find((b) => b.type === "response");
			expect(textBlock).toBeDefined();
		});
	});

	it("does not let a stale REST chatRecord.status override WS-delivered status", async () => {
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
				useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: props.chatRecord,
				});
				return {
					chatStatus: useCachedChatStatus(chatID),
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

		// Deliver a status event so the stream takes execution ownership.
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

		// Simulate a stale REST refetch returning "waiting".
		rerender({
			chatRecord: { ...buildChat(chatID), status: "waiting" },
		});

		// The store must ignore the stale REST value because the
		// WS already delivered a status event for this chat.
		await waitFor(() => {
			expect(result.current.chatStatus).toBe("running");
		});
	});

	it("hydrates the next chat after the previous stream owned execution status", async () => {
		immediateAnimationFrame();

		const chatA = "chat-status-owner-a";
		const chatB = "chat-status-owner-b";
		const socketA = createMockSocket();
		const socketB = createMockSocket();
		mockWatchChatReturnOnce(socketA);
		mockWatchChatReturnOnce(socketB);
		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result, rerender } = renderHook(
			(props: { chatID: string; chatRecord: TypesGen.Chat }) => {
				useChatStreamStore({
					chatID: props.chatID,
					chatMessages: [],
					chatRecord: props.chatRecord,
				});
				return useCachedChatStatus(props.chatID);
			},
			{
				wrapper,
				initialProps: {
					chatID: chatA,
					chatRecord: buildChat(chatA),
				},
			},
		);

		act(() => {
			socketA.emitData({
				type: "status",
				chat_id: chatA,
				status: { status: "waiting" },
			});
		});
		await waitFor(() => expect(result.current).toBe("waiting"));

		rerender({
			chatID: chatB,
			chatRecord: { ...buildChat(chatB), status: "waiting" },
		});
		await waitFor(() => expect(result.current).toBe("waiting"));
	});

	it("preserves stream state when status transitions to waiting", async () => {
		immediateAnimationFrame();

		const chatID = "chat-preserve-stream";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
		seedChatMessages(queryClient, chatID, [existingMessage]);
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
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
			});
		});

		// Stream state should now be null and the durable message
		// should be in the message store.
		await waitFor(() => {
			expect(result.current.streamState).toBeNull();
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toContain(2);
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...buildChat(chatID), status: "running" },
				});
				const streamState = useChatSelector(store, selectStreamState);
				const chatStatus = useCachedChatStatus(chatID);
				return {
					streamState,
					chatStatus,
					isAwaiting: isAwaitingFirstStreamChunk(
						chatStatus,
						streamState,
						userMsg,
					),
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...buildChat(chatID), status: "running" },
				});
				const streamState = useChatSelector(store, selectStreamState);
				const chatStatus = useCachedChatStatus(chatID);
				return {
					streamState,
					chatStatus,
					isAwaiting: isAwaitingFirstStreamChunk(
						chatStatus,
						streamState,
						userMsg,
					),
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
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: { ...buildChat(chatID), status: "running" },
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					chatStatus: useCachedChatStatus(chatID),
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

		renderHook(
			() => {
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
				});
				return { chatStatus: useCachedChatStatus(chatID) };
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

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
				}),
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

		renderHook(
			() => {
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
				});
				return { chatStatus: useCachedChatStatus(chatID) };
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
		const activeChat = buildChat(chatID);
		const otherChat = buildChat(otherChatID);
		seedInfiniteChats(queryClient, [activeChat, otherChat]);

		const wrapper = createWrapper(queryClient);

		renderHook(
			() => {
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: activeChat,
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

		renderHook(
			() =>
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
				}),
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

		renderHook(
			() => {
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
				});
				return { chatStatus: useCachedChatStatus(chatID) };
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

		renderHook(
			() => {
				useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: initialChat,
				});
				return { chatStatus: useCachedChatStatus(chatID) };
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
		const userMsg = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		seedChatMessages(queryClient, chatID, [userMsg]);
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: buildChat(chatID),
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
			});
		});

		// The durable message must be present AND streamState
		// must be null in the same snapshot.
		await waitFor(() => {
			expect(
				readChatMessages(queryClient, chatID).map((message) => message.id),
			).toContain(2);
			expect(
				readChatMessages(queryClient, chatID).find(
					(message) => message.id === 2,
				)?.role,
			).toBe("assistant");
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
		seedChatMessages(queryClient, chatID, [userMsg]);
		const wrapper = createWrapper(queryClient);

		// Track every snapshot emitted to subscribers.
		const snapshots: Array<{
			hasStream: boolean;
			hasDurableAssistant: boolean;
		}> = [];

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: buildChat(chatID),
				});
				const streamState = useChatSelector(store, selectStreamState);
				const { data } = useInfiniteQuery({
					...chatMessagesForInfiniteScroll(chatID),
					enabled: false,
				});
				const messages = data?.messages ?? [];
				const hasDurableAssistant = messages.some(
					(message) => message.role === "assistant",
				);

				snapshots.push({
					hasStream: streamState !== null,
					hasDurableAssistant,
				});

				return { streamState, messages };
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
			});
		});

		await waitFor(() => {
			expect(result.current.messages.some((message) => message.id === 2)).toBe(
				true,
			);
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

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [userMsg],
					chatRecord: buildChat(chatID),
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
		const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [],
					chatRecord: buildChat(chatID),
				});
				return {
					streamError: useChatSelector(store, selectTransientError),
					chatStatus: useCachedChatStatus(chatID),
				};
			},
			{ wrapper },
		);

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID, undefined);
		});
		invalidateQueries.mockClear();

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
		expect(invalidateQueries).toHaveBeenCalledWith({
			queryKey: chatQueryKeys.detail(chatID),
		});
		expect(invalidateQueries).toHaveBeenCalledWith({
			queryKey: chatQueryKeys.messages(chatID),
			exact: true,
		});
		expect(invalidateQueries).toHaveBeenCalledWith({
			queryKey: chatQueryKeys.prompts(chatID),
			exact: true,
		});
		expect(invalidateQueries).toHaveBeenCalledWith({
			queryKey: chatQueryKeys.lists(),
		});
		expect(invalidateQueries).toHaveBeenCalledWith({
			queryKey: chatQueryKeys.searches(),
		});
		expect(invalidateQueries).toHaveBeenCalledWith({
			queryKey: chatQueryKeys.byWorkspace(),
		});
	});

	it("does not corrupt in-progress stream state", async () => {
		immediateAnimationFrame();

		const chatID = "chat-parse-no-corrupt";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					streamError: useChatSelector(store, selectTransientError),
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

	it("ignores later events from a socket after a parse error", async () => {
		immediateAnimationFrame();

		const chatID = "chat-parse-recover";
		const existingMessage = buildMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		mockWatchChatReturn(mockSocket);

		const queryClient = createTestQueryClient();
		const wrapper = createWrapper(queryClient);

		const { result } = renderHook(
			() => {
				const { store } = useChatStreamStore({
					chatID,
					chatMessages: [existingMessage],
					chatRecord: buildChat(chatID),
				});
				return {
					streamState: useChatSelector(store, selectStreamState),
					streamError: useChatSelector(store, selectTransientError),
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

		expect(mockSocket.close).toHaveBeenCalled();

		// Events from the damaged connection are fenced until reconnect.
		act(() => {
			mockSocket.emitData({
				type: "message_part",
				chat_id: chatID,
				message_part: {
					role: "assistant",
					part: { type: "text", text: "must be ignored" },
				},
			});
		});
		await act(async () => {});
		expect(result.current.streamState).toBeNull();
		expect(result.current.streamError).toEqual({
			kind: "generic",
			message: "Failed to parse chat stream update.",
		});
	});
});
