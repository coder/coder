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
import { useChatPreferences } from "./useChatPreferences";
import { useSendChatMessage } from "./useSendChatMessage";

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
			createChatDetail({
				chat: createChat({ id: chatId }),
				messages: [createMessage({ id: 1, chat_id: chatId, role: "user" })],
			}),
		),
		sendMessage: vi.fn(async () => ({ queued: false })),
		listModels: vi.fn(async () => []),
		subscribeToChat: vi.fn(() => ({ dispose: vi.fn() })),
	} satisfies ChatRuntime;

	return { ...runtime, ...overrides } as typeof runtime;
};

const createPreferenceStore = (
	initialValues: Record<string, unknown> = {},
): ChatPreferenceStore => {
	const values = new Map<string, unknown>(Object.entries(initialValues));
	const listeners = new Map<string, Set<() => void>>();

	const notify = (key: string): void => {
		const keyListeners = listeners.get(key);
		if (!keyListeners) {
			return;
		}
		for (const listener of keyListeners) {
			listener();
		}
	};

	return {
		get<T>(key: string, fallback: T): T {
			return values.has(key) ? (values.get(key) as T) : fallback;
		},
		set<T>(key: string, value: T): void {
			values.set(key, value);
			notify(key);
		},
		subscribe(key: string, cb: () => void) {
			const keyListeners = listeners.get(key) ?? new Set<() => void>();
			keyListeners.add(cb);
			listeners.set(key, keyListeners);
			return () => {
				const existing = listeners.get(key);
				if (!existing) {
					return;
				}
				existing.delete(cb);
				if (existing.size === 0) {
					listeners.delete(key);
				}
			};
		},
	};
};

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

describe("useSendChatMessage", () => {
	it("rejects when there is no active chat", async () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(() => useSendChatMessage(), { wrapper });

		await expect(
			result.current.sendMessage({ message: "hello" }),
		).rejects.toThrow("Cannot send a chat message without an active chat.");
	});

	it("passes the active chat, selected model, and parent message id to runtime.sendMessage and stores durable responses", async () => {
		const durableMessage = createMessage({
			id: 9,
			chat_id: "chat-1",
			role: "assistant",
			content: [{ type: "text", text: "done" }],
		});
		const runtime = createFakeRuntime({
			sendMessage: vi.fn(async () => ({
				queued: false,
				message: durableMessage,
			})),
		});
		const wrapper = createWrapper(runtime, createPreferenceStore());
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				preferences: useChatPreferences(),
				sender: useSendChatMessage(),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		act(() => {
			result.current.preferences.setSelectedModel("gpt-4o-mini");
		});
		await waitFor(() => {
			expect(result.current.preferences.selectedModel).toBe("gpt-4o-mini");
		});

		await act(async () => {
			await result.current.sender.sendMessage({
				message: "hello",
				parentMessageId: 1,
			});
		});

		expect(runtime.sendMessage).toHaveBeenCalledWith({
			chatId: "chat-1",
			message: "hello",
			model: "gpt-4o-mini",
			parentMessageId: 1,
		});
		expect(result.current.snapshot.messagesByID.get(9)).toEqual(durableMessage);
		expect(result.current.snapshot.orderedMessageIDs).toEqual([1, 9]);
	});

	it("stores queued responses in the shared queue", async () => {
		const queuedMessage = createQueuedMessage({ id: 7, chat_id: "chat-1" });
		const runtime = createFakeRuntime({
			sendMessage: vi.fn(async () => ({
				queued: true,
				queued_message: queuedMessage,
			})),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				sender: useSendChatMessage(),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		await act(async () => {
			await result.current.sender.sendMessage({ message: "queue this" });
		});

		expect(result.current.snapshot.queuedMessages).toEqual([queuedMessage]);
	});

	it("sets isSending while a request is in flight", async () => {
		const pendingResponse =
			createDeferred<TypesGen.CreateChatMessageResponse>();
		const runtime = createFakeRuntime({
			sendMessage: vi.fn(() => pendingResponse.promise),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				sender: useSendChatMessage(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		let sendPromise!: Promise<void>;
		act(() => {
			sendPromise = result.current.sender.sendMessage({ message: "pending" });
		});
		expect(result.current.sender.isSending).toBe(true);

		act(() => {
			pendingResponse.resolve({
				queued: false,
				message: createMessage({ id: 11, chat_id: "chat-1" }),
			});
		});
		await act(async () => {
			await sendPromise;
		});

		expect(result.current.sender.isSending).toBe(false);
	});

	it("surfaces send failures to callers and records lastError", async () => {
		const runtime = createFakeRuntime({
			sendMessage: vi.fn(async () => {
				throw new Error("send failed");
			}),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				sender: useSendChatMessage(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		await expect(
			result.current.sender.sendMessage({ message: "boom" }),
		).rejects.toThrow("send failed");
		await waitFor(() => {
			expect(result.current.sender.lastError).toBeInstanceOf(Error);
			expect(result.current.sender.isSending).toBe(false);
		});
		expect((result.current.sender.lastError as Error).message).toBe(
			"send failed",
		);
	});

	it("asserts when a queued response omits queued_message", async () => {
		const runtime = createFakeRuntime({
			sendMessage: vi.fn(async () => ({ queued: true })),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				sender: useSendChatMessage(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		await expect(
			result.current.sender.sendMessage({ message: "bad queued" }),
		).rejects.toThrow("Queued chat responses must include a queued message.");
		await waitFor(() => {
			expect(result.current.sender.isSending).toBe(false);
		});
	});

	it("asserts when a durable response omits message", async () => {
		const runtime = createFakeRuntime({
			sendMessage: vi.fn(async () => ({ queued: false })),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				conversation: useChatConversation("chat-1"),
				sender: useSendChatMessage(),
			}),
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current.conversation.chat?.id).toBe("chat-1");
		});

		await expect(
			result.current.sender.sendMessage({ message: "bad durable" }),
		).rejects.toThrow("Durable chat responses must include a message.");
		await waitFor(() => {
			expect(result.current.sender.isSending).toBe(false);
		});
	});
});
