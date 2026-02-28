import { act, render, renderHook, waitFor } from "@testing-library/react";
import { watchChat } from "api/api";
import { chatKey } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	selectChatStatus,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	useChatStore,
} from "./ChatContext";

vi.mock("api/api", () => ({
	watchChat: vi.fn(),
}));

type MessageListener = (
	payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
) => void;
type ErrorListener = (payload: Event) => void;

interface MockSocket {
	addEventListener(event: "message", callback: MessageListener): void;
	addEventListener(event: "error", callback: ErrorListener): void;
	removeEventListener(event: "message", callback: MessageListener): void;
	removeEventListener(event: "error", callback: ErrorListener): void;
	close: () => void;
	emitData: (event: TypesGen.ChatStreamEvent) => void;
	emitDataBatch: (events: readonly TypesGen.ChatStreamEvent[]) => void;
	emitError: () => void;
}

const createMockSocket = (): MockSocket => {
	const messageListeners = new Set<MessageListener>();
	const errorListeners = new Set<ErrorListener>();

	const addEventListener = (
		event: "message" | "error",
		callback: MessageListener | ErrorListener,
	): void => {
		if (event === "message") {
			messageListeners.add(callback as MessageListener);
			return;
		}
		errorListeners.add(callback as ErrorListener);
	};

	const removeEventListener = (
		event: "message" | "error",
		callback: MessageListener | ErrorListener,
	): void => {
		if (event === "message") {
			messageListeners.delete(callback as MessageListener);
			return;
		}
		errorListeners.delete(callback as ErrorListener);
	};

	return {
		addEventListener,
		removeEventListener,
		close: vi.fn(),
		emitData: (event) => {
			const payload: OneWayMessageEvent<TypesGen.ServerSentEvent> = {
				sourceEvent: {} as MessageEvent<string>,
				parseError: undefined,
				parsedMessage: {
					type: "data",
					data: event,
				},
			};
			for (const listener of messageListeners) {
				listener(payload);
			}
		},
		emitDataBatch: (events) => {
			const payload: OneWayMessageEvent<TypesGen.ServerSentEvent> = {
				sourceEvent: {} as MessageEvent<string>,
				parseError: undefined,
				parsedMessage: {
					type: "data",
					data: events,
				},
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
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	title: "test",
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	last_error: null,
});

const makeMessage = (
	chatID: string,
	id: number,
	role: string,
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
	vi.restoreAllMocks();
	vi.mocked(watchChat).mockReset();
});

describe("useChatStore", () => {
	it("does not clear in-progress stream parts for duplicate snapshot messages", async () => {
		immediateAnimationFrame();

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
				chatData: {
					chat: makeChat(chatID),
					messages: [existingMessage],
					queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
			chatData: {
				chat: makeChat(chatID),
				messages: [existingMessage],
				queued_messages: [queuedMessage],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
			chatData: {
				chat: {
					...makeChat(chatID),
					updated_at: "2025-01-01T00:00:01.000Z",
				},
				messages: [existingMessage],
				queued_messages: [queuedMessage],
			},
			chatQueuedMessages: [queuedMessage],
		});

		await waitFor(() => {
			expect(result.current.queuedMessages).toEqual([]);
		});
	});

	it("writes queue_update snapshots into the chat query cache", async () => {
		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const queuedMessage = makeQueuedMessage(chatID, 10, "queued");
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
		const initialChatData: TypesGen.ChatWithMessages = {
			chat: makeChat(chatID),
			messages: [existingMessage],
			queued_messages: [queuedMessage],
		};
		queryClient.setQueryData(chatKey(chatID), initialChatData);

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
					chatData: initialChatData,
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		expect(
			queryClient.getQueryData<TypesGen.ChatWithMessages | undefined>(
				chatKey(chatID),
			)?.queued_messages,
		).toEqual([]);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket2 as never);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1 as never)
			.mockReturnValueOnce(mockSocket1 as never)
			.mockReturnValueOnce(mockSocket1 as never);

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
			chatData: {
				chat: makeChat(chatID1),
				messages: [msg1],
				queued_messages: [] as TypesGen.ChatQueuedMessage[],
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
			expect(watchChat).toHaveBeenCalledWith(chatID1);
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
			chatData: {
				chat: makeChat(chatID2),
				messages: [msg2],
				queued_messages: [],
			},
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [queuedMessage],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket2 as never);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1 as never)
			.mockReturnValueOnce(mockSocket1 as never)
			.mockReturnValueOnce(mockSocket1 as never);

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
			chatData: {
				chat: makeChat(chatID1),
				messages: [msg1],
				queued_messages: [] as TypesGen.ChatQueuedMessage[],
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
			expect(watchChat).toHaveBeenCalledWith(chatID1);
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
			chatData: {
				chat: makeChat(chatID2),
				messages: [msg2],
				queued_messages: [],
			},
		});

		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket2 as never);
		vi.mocked(watchChat)
			.mockReturnValueOnce(mockSocket1 as never)
			.mockReturnValueOnce(mockSocket1 as never)
			.mockReturnValueOnce(mockSocket1 as never);

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
			chatData: {
				chat: makeChat(chatID1),
				messages: [msg1],
				queued_messages: [queuedMsg],
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
			expect(watchChat).toHaveBeenCalledWith(chatID1);
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
			chatData: {
				chat: makeChat(chatID2),
				messages: [],
				queued_messages: [],
			},
			chatQueuedMessages: [],
		});

		// After the switch, queued messages from chat-1 should NOT be
		// visible — the store resets them on chatID change.
		await waitFor(() => {
			expect(watchChat).toHaveBeenCalledWith(chatID2);
		});
		expect(result.current.queuedMessages).toEqual([]);
	});

	it("does not apply message parts after status changes to waiting", async () => {
		immediateAnimationFrame();

		const chatID = "chat-status-guard";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "Rate limit exceeded" },
			});
		});

		await waitFor(() => {
			expect(result.current.chatStatus).toBe("error");
		});
		expect(result.current.streamError).toBe("Rate limit exceeded");
		expect(result.current.retryState).toBeNull();
		expect(setChatErrorReason).toHaveBeenCalledWith(
			chatID,
			"Rate limit exceeded",
		);
	});

	it("uses fallback message when error event has no message", async () => {
		immediateAnimationFrame();

		const chatID = "chat-error-empty";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
		});

		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "  " },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toBe("Chat processing failed.");
		});
	});

	it("populates retryState on retry event", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
		});

		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 2,
					error: "upstream timeout",
					delay_ms: 5000,
					retrying_at: "2025-01-01T00:01:00.000Z",
				},
			});
		});

		await waitFor(() => {
			expect(result.current.retryState).toEqual({
				attempt: 2,
				error: "upstream timeout",
			});
		});
	});

	it("clears retryState when status transitions to running", async () => {
		immediateAnimationFrame();

		const chatID = "chat-retry-clear";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
		});

		// Set retry state first.
		act(() => {
			mockSocket.emitData({
				type: "retry",
				chat_id: chatID,
				retry: {
					attempt: 1,
					error: "rate limited",
					delay_ms: 3000,
					retrying_at: "2025-01-01T00:00:30.000Z",
				},
			});
		});

		await waitFor(() => {
			expect(result.current.retryState).not.toBeNull();
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

	it("routes status events for other chatIDs to subagent overrides", async () => {
		immediateAnimationFrame();

		const chatID = "chat-main";
		const subagentChatID = "chat-subagent-1";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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

	it("sets streamError on WebSocket disconnect", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
		});

		act(() => {
			mockSocket.emitError();
		});

		await waitFor(() => {
			expect(result.current.streamError).toBe("Chat stream disconnected.");
		});
	});

	it("does not overwrite existing streamError on WebSocket disconnect", async () => {
		immediateAnimationFrame();

		const chatID = "chat-disconnect-existing";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
		});

		// Set an error via an error stream event first.
		act(() => {
			mockSocket.emitData({
				type: "error",
				chat_id: chatID,
				error: { message: "Rate limit exceeded" },
			});
		});

		await waitFor(() => {
			expect(result.current.streamError).toBe("Rate limit exceeded");
		});

		// WebSocket disconnect should NOT overwrite the existing error.
		act(() => {
			mockSocket.emitError();
		});

		// The original error should be preserved.
		await waitFor(() => {
			expect(result.current.streamError).toBe("Rate limit exceeded");
		});
	});

	it("clears chatErrorReason when status transitions to non-error", async () => {
		immediateAnimationFrame();

		const chatID = "chat-clear-error";
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

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
					chatData: {
						chat: makeChat(chatID),
						messages: [],
						queued_messages: [],
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
			expect(watchChat).toHaveBeenCalledWith(chatID);
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
});
