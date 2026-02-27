import { act, render, renderHook, waitFor } from "@testing-library/react";
import { watchChat } from "api/api";
import { chatKey } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	selectOrderedMessageIDs,
	selectQueuedMessages,
	selectStreamState,
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
});
