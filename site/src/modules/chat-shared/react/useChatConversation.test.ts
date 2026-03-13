import { act, renderHook, waitFor } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { createElement, type FC, type PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatPreferenceStore, ChatRuntime } from "../core";
import {
	ChatRuntimeProvider,
	useChatStoreSnapshot,
} from "./ChatRuntimeProvider";
import { useChatConversation } from "./useChatConversation";

const createTimestamp = (seconds: number): string =>
	new Date(Date.UTC(2025, 0, 1, 0, 0, seconds)).toISOString();

const createDeferred = <T>() => {
	let resolve!: (value: T | PromiseLike<T>) => void;
	let reject!: (reason?: unknown) => void;
	const promise = new Promise<T>((nextResolve, nextReject) => {
		resolve = nextResolve;
		reject = nextReject;
	});
	return { promise, resolve, reject };
};

const createChat = (
	overrides: Partial<TypesGen.Chat> & { id?: string } = {},
): TypesGen.Chat => ({
	id: overrides.id ?? "chat-1",
	owner_id: overrides.owner_id ?? "owner-1",
	workspace_id: overrides.workspace_id,
	parent_chat_id: overrides.parent_chat_id,
	root_chat_id: overrides.root_chat_id,
	last_model_config_id: overrides.last_model_config_id ?? "model-config-1",
	title: overrides.title ?? "Test chat",
	status: overrides.status ?? "completed",
	last_error: overrides.last_error ?? null,
	diff_status: overrides.diff_status,
	created_at: overrides.created_at ?? createTimestamp(0),
	updated_at: overrides.updated_at ?? createTimestamp(1),
	archived: overrides.archived ?? false,
});

const createMessage = (
	overrides: Partial<TypesGen.ChatMessage> & {
		id?: number;
		chat_id?: string;
		content?: readonly TypesGen.ChatMessagePart[];
	} = {},
): TypesGen.ChatMessage => {
	const id = overrides.id ?? 1;
	return {
		id,
		chat_id: overrides.chat_id ?? "chat-1",
		model_config_id: overrides.model_config_id,
		created_at: overrides.created_at ?? createTimestamp(id),
		role: overrides.role ?? "assistant",
		content: overrides.content ?? [{ type: "text", text: `message-${id}` }],
		usage: overrides.usage,
	};
};

const createQueuedMessage = (
	overrides: Partial<TypesGen.ChatQueuedMessage> & {
		id?: number;
		chat_id?: string;
	} = {},
): TypesGen.ChatQueuedMessage => {
	const id = overrides.id ?? 1;
	return {
		id,
		chat_id: overrides.chat_id ?? "chat-1",
		created_at: overrides.created_at ?? createTimestamp(id),
		content: overrides.content ?? [{ type: "text", text: `queued-${id}` }],
	};
};

const createChatDetail = (
	overrides: Partial<TypesGen.ChatWithMessages> = {},
): TypesGen.ChatWithMessages => ({
	chat: overrides.chat ?? createChat(),
	messages: overrides.messages ?? [],
	queued_messages: overrides.queued_messages ?? [],
});

const createFakeRuntime = (overrides: Partial<ChatRuntime> = {}) => {
	const runtime = {
		listChats: vi.fn(async () => [] as readonly TypesGen.Chat[]),
		getChat: vi.fn(async (chatId: string) =>
			createChatDetail({ chat: createChat({ id: chatId }) }),
		),
		sendMessage: vi.fn(async () => ({ queued: false })),
		listModels: vi.fn(async () => []),
		subscribeToChat: vi.fn(() => ({ dispose: vi.fn() })),
	} satisfies ChatRuntime;

	return { ...runtime, ...overrides } as typeof runtime;
};

const createPreferenceStore = (): ChatPreferenceStore => ({
	get<T>(_key: string, fallback: T): T {
		return fallback;
	},
	set<T>(_key: string, _value: T): void {},
	subscribe(_key: string, _cb: () => void) {
		return () => {};
	},
});

const createWrapper = (
	runtime: ChatRuntime,
	preferenceStore: ChatPreferenceStore = createPreferenceStore(),
): FC<PropsWithChildren> => {
	return ({ children }) =>
		createElement(ChatRuntimeProvider, { runtime, preferenceStore, children });
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useChatConversation", () => {
	it("reports loading state while the initial fetch is pending", async () => {
		const pendingChat = createDeferred<TypesGen.ChatWithMessages>();
		const runtime = createFakeRuntime({
			getChat: vi.fn(() => pendingChat.promise),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.isLoading).toBe(true);
		});
		expect(result.current.conversation.chat).toBeNull();
		expect(result.current.conversation.error).toBeNull();
		expect(result.current.snapshot.orderedMessageIDs).toEqual([]);

		act(() => {
			pendingChat.resolve(
				createChatDetail({
					chat: createChat({ id: "chat-1" }),
				}),
			);
		});
		await waitFor(() => {
			expect(result.current.conversation.isLoading).toBe(false);
		});
	});

	it("hydrates the shared store from runtime.getChat", async () => {
		const messages = [
			createMessage({ id: 1, chat_id: "chat-1", role: "user" }),
			createMessage({ id: 2, chat_id: "chat-1", role: "assistant" }),
		];
		const queuedMessage = createQueuedMessage({ id: 3, chat_id: "chat-1" });
		const runtime = createFakeRuntime({
			getChat: vi.fn(async () =>
				createChatDetail({
					chat: createChat({ id: "chat-1", status: "running" }),
					messages,
					queued_messages: [queuedMessage],
				}),
			),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});
		expect(result.current.snapshot.orderedMessageIDs).toEqual([1, 2]);
		expect(result.current.snapshot.messagesByID.get(2)).toEqual(messages[1]);
		expect(result.current.snapshot.queuedMessages).toEqual([queuedMessage]);
		expect(result.current.snapshot.chatStatus).toBe("running");
	});

	it("ignores stale fetch results after the selected chat changes", async () => {
		const chat1Deferred = createDeferred<TypesGen.ChatWithMessages>();
		const chat2Deferred = createDeferred<TypesGen.ChatWithMessages>();
		const runtime = createFakeRuntime({
			getChat: vi.fn((chatId: string) =>
				chatId === "chat-1" ? chat1Deferred.promise : chat2Deferred.promise,
			),
		});
		const wrapper = createWrapper(runtime);
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | null }) => ({
				conversation: useChatConversation(chatId),
				snapshot: useChatStoreSnapshot(),
			}),
			{
				initialProps: { chatId: "chat-1" as string | null },
				wrapper,
			},
		);

		await waitFor(() => {
			expect(result.current.conversation.isLoading).toBe(true);
		});

		rerender({ chatId: "chat-2" });
		act(() => {
			chat2Deferred.resolve(
				createChatDetail({
					chat: createChat({ id: "chat-2" }),
					messages: [createMessage({ id: 20, chat_id: "chat-2" })],
				}),
			);
		});

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-2");
		});
		expect(result.current.snapshot.orderedMessageIDs).toEqual([20]);

		act(() => {
			chat1Deferred.resolve(
				createChatDetail({
					chat: createChat({ id: "chat-1" }),
					messages: [createMessage({ id: 10, chat_id: "chat-1" })],
				}),
			);
		});

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-2");
		});
		expect(result.current.snapshot.orderedMessageIDs).toEqual([20]);
	});

	it("clears the active conversation when chatId becomes null", async () => {
		const runtime = createFakeRuntime({
			getChat: vi.fn(async () =>
				createChatDetail({
					chat: createChat({ id: "chat-1", status: "running" }),
					messages: [createMessage({ id: 1, chat_id: "chat-1" })],
					queued_messages: [createQueuedMessage({ id: 2, chat_id: "chat-1" })],
				}),
			),
		});
		const wrapper = createWrapper(runtime);
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | null }) => ({
				conversation: useChatConversation(chatId),
				snapshot: useChatStoreSnapshot(),
			}),
			{
				initialProps: { chatId: "chat-1" as string | null },
				wrapper,
			},
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		rerender({ chatId: null });
		await waitFor(() => {
			expect(result.current.conversation.chat).toBeNull();
			expect(result.current.conversation.isLoading).toBe(false);
		});
		expect(result.current.conversation.error).toBeNull();
		expect(result.current.snapshot.orderedMessageIDs).toEqual([]);
		expect(result.current.snapshot.queuedMessages).toEqual([]);
		expect(result.current.snapshot.chatStatus).toBeNull();
	});

	it("captures fetch errors and clears the loading state", async () => {
		const runtime = createFakeRuntime({
			getChat: vi.fn(async () => {
				throw new Error("network down");
			}),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.isLoading).toBe(false);
			expect(result.current.conversation.error).toBeInstanceOf(Error);
		});
		expect((result.current.conversation.error as Error).message).toBe(
			"network down",
		);
		expect(result.current.snapshot.orderedMessageIDs).toEqual([]);
	});

	it("refetches and applies newer chat data", async () => {
		const runtime = createFakeRuntime({
			getChat: vi
				.fn()
				.mockResolvedValueOnce(
					createChatDetail({
						chat: createChat({ id: "chat-1", updated_at: createTimestamp(1) }),
						messages: [createMessage({ id: 1, chat_id: "chat-1" })],
					}),
				)
				.mockResolvedValueOnce(
					createChatDetail({
						chat: createChat({ id: "chat-1", updated_at: createTimestamp(5) }),
						messages: [createMessage({ id: 9, chat_id: "chat-1" })],
						queued_messages: [
							createQueuedMessage({ id: 12, chat_id: "chat-1" }),
						],
					}),
				),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.snapshot.orderedMessageIDs).toEqual([1]);
		});

		await act(async () => {
			await result.current.conversation.refetch();
		});

		await waitFor(() => {
			expect(result.current.snapshot.orderedMessageIDs).toEqual([9]);
		});
		expect(runtime.getChat).toHaveBeenCalledTimes(2);
		expect(
			result.current.snapshot.queuedMessages.map((message) => message.id),
		).toEqual([12]);
	});

	it("records a mismatch error when the runtime returns a different chat", async () => {
		const runtime = createFakeRuntime({
			getChat: vi.fn(async () =>
				createChatDetail({
					chat: createChat({ id: "chat-2" }),
					messages: [createMessage({ id: 2, chat_id: "chat-2" })],
				}),
			),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.error).toBeInstanceOf(Error);
		});
		expect((result.current.conversation.error as Error).message).toBe(
			"Expected chat chat-1 but received chat-2.",
		);
		expect(result.current.snapshot.orderedMessageIDs).toEqual([]);
		expect(result.current.conversation.chat).toBeNull();
	});

	it("clears the previous chat state before a replacement chat finishes loading", async () => {
		const nextChat = createDeferred<TypesGen.ChatWithMessages>();
		const runtime = createFakeRuntime({
			getChat: vi.fn((chatId: string) => {
				if (chatId === "chat-1") {
					return Promise.resolve(
						createChatDetail({
							chat: createChat({ id: "chat-1", status: "running" }),
							messages: [createMessage({ id: 1, chat_id: "chat-1" })],
							queued_messages: [
								createQueuedMessage({ id: 2, chat_id: "chat-1" }),
							],
						}),
					);
				}
				return nextChat.promise;
			}),
		});
		const wrapper = createWrapper(runtime);
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | null }) => ({
				conversation: useChatConversation(chatId),
				snapshot: useChatStoreSnapshot(),
			}),
			{
				initialProps: { chatId: "chat-1" as string | null },
				wrapper,
			},
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});
		expect(
			result.current.snapshot.queuedMessages.map((message) => message.id),
		).toEqual([2]);

		rerender({ chatId: "chat-2" });
		await waitFor(() => {
			expect(result.current.conversation.isLoading).toBe(true);
			expect(result.current.conversation.chat).toBeNull();
			expect(result.current.snapshot.orderedMessageIDs).toEqual([]);
			expect(result.current.snapshot.queuedMessages).toEqual([]);
			expect(result.current.snapshot.chatStatus).toBeNull();
		});

		act(() => {
			nextChat.resolve(
				createChatDetail({
					chat: createChat({ id: "chat-2" }),
					messages: [createMessage({ id: 10, chat_id: "chat-2" })],
				}),
			);
		});
		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-2");
		});
	});
});
