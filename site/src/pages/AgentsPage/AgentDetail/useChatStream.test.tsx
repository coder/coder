import { act, renderHook, waitFor } from "@testing-library/react";
import { watchChat } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { describe, expect, it, vi } from "vitest";
import { useChatStream } from "./useChatStream";

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

describe("useChatStream", () => {
	it("does not clear in-progress stream parts for duplicate snapshot messages", async () => {
		vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
			cb(0);
			return 1;
		});
		vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const { result } = renderHook(
			() =>
				useChatStream({
					chatId: chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
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

		// Snapshot replay sends already-known durable messages.
		act(() => {
			mockSocket.emitData({
				type: "message",
				chat_id: chatID,
				message: existingMessage,
			});
		});

		// Streamed parts should remain visible after duplicate message replay.
		await waitFor(() => {
			expect(result.current.streamState?.blocks).toEqual([
				{ type: "response", text: "reconnect-part-one" },
			]);
		});
	});

	it("clears stream state when a new durable message arrives", async () => {
		vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
			cb(0);
			return 1;
		});
		vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});

		const chatID = "chat-1";
		const existingMessage = makeMessage(chatID, 1, "user", "hello");
		const newMessage = makeMessage(chatID, 2, "assistant", "done");
		const mockSocket = createMockSocket();
		vi.mocked(watchChat).mockReturnValue(mockSocket as never);

		const queryClient = createTestQueryClient();
		const wrapper = ({ children }: PropsWithChildren) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		);

		const { result } = renderHook(
			() =>
				useChatStream({
					chatId: chatID,
					chatMessages: [existingMessage],
					chatRecord: makeChat(chatID),
					chatData: {
						chat: makeChat(chatID),
						messages: [existingMessage],
						queued_messages: [],
					},
					chatQueuedMessages: [],
					setChatErrorReason: vi.fn(),
					clearChatErrorReason: vi.fn(),
				}),
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
});
