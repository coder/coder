import { act, renderHook, waitFor } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import type { FC, PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
	ChatPreferenceStore,
	ChatRuntime,
	ChatStreamEvent,
} from "../core";
import {
	ChatRuntimeProvider,
	useChatRuntimeContext,
	useChatStoreSnapshot,
} from "./ChatRuntimeProvider";
import { useChatConversation } from "./useChatConversation";
import { useChatPreferences } from "./useChatPreferences";

const createTimestamp = (seconds: number): string =>
	new Date(Date.UTC(2025, 0, 1, 0, 0, seconds)).toISOString();

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
		content?: readonly TypesGen.ChatMessagePart[];
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

const createPreferenceStore = (
	initialValues: Record<string, unknown> = {},
	options: { withSubscribe?: boolean } = {},
) => {
	const values = new Map<string, unknown>(Object.entries(initialValues));
	const listeners = new Map<string, Set<() => void>>();
	const withSubscribe = options.withSubscribe ?? true;

	const notify = (key: string): void => {
		const keyListeners = listeners.get(key);
		if (!keyListeners) {
			return;
		}
		for (const listener of keyListeners) {
			listener();
		}
	};

	const store = {
		get<T>(key: string, fallback: T): T {
			return values.has(key) ? (values.get(key) as T) : fallback;
		},
		set<T>(key: string, value: T): void {
			values.set(key, value);
			if (withSubscribe) {
				notify(key);
			}
		},
		...(withSubscribe
			? {
					subscribe: (key: string, cb: () => void) => {
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
				}
			: {}),
	} satisfies ChatPreferenceStore;

	return { ...store, values };
};

const createWrapper = (
	runtime: ChatRuntime,
	preferenceStore: ChatPreferenceStore = createPreferenceStore(),
): FC<PropsWithChildren> => {
	return ({ children }) => (
		<ChatRuntimeProvider runtime={runtime} preferenceStore={preferenceStore}>
			{children}
		</ChatRuntimeProvider>
	);
};

type SubscriptionRecord = {
	input: { chatId: string; afterMessageId?: number };
	onEvent: (event: ChatStreamEvent) => void;
	dispose: ReturnType<typeof vi.fn>;
};

const createRuntimeHarness = (overrides: Partial<ChatRuntime> = {}) => {
	const subscriptions: SubscriptionRecord[] = [];
	const runtime = createFakeRuntime({
		subscribeToChat: vi.fn((input, onEvent) => {
			const subscription = {
				input,
				onEvent,
				dispose: vi.fn(),
			};
			subscriptions.push(subscription);
			return { dispose: subscription.dispose };
		}),
		...overrides,
	});

	return { runtime, subscriptions };
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("ChatRuntimeProvider", () => {
	it("throws when the runtime context hook is used outside the provider", () => {
		expect(() => renderHook(() => useChatRuntimeContext())).toThrow(
			"useChatRuntimeContext must be used within a ChatRuntimeProvider.",
		);
	});

	it("exposes the runtime context and initial store state inside the provider", () => {
		const runtime = createFakeRuntime();
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => ({
				context: useChatRuntimeContext(),
				snapshot: useChatStoreSnapshot(),
			}),
			{ wrapper },
		);

		expect(result.current.context.runtime).toBe(runtime);
		expect(result.current.context.activeChatId).toBeNull();
		expect(result.current.snapshot.orderedMessageIDs).toEqual([]);
		expect(result.current.snapshot.messagesByID.size).toBe(0);
		expect(result.current.snapshot.queuedMessages).toEqual([]);
	});

	it("subscribes to the active chat and disposes subscriptions on switch and unmount", async () => {
		const { runtime, subscriptions } = createRuntimeHarness({
			getChat: vi.fn(async (chatId: string) => {
				const messageID = chatId === "chat-1" ? 1 : 10;
				return createChatDetail({
					chat: createChat({ id: chatId }),
					messages: [createMessage({ id: messageID, chat_id: chatId })],
				});
			}),
		});
		const wrapper = createWrapper(runtime);
		const { result, rerender, unmount } = renderHook(
			({ chatId }: { chatId: string | null }) => useChatConversation(chatId),
			{
				initialProps: { chatId: "chat-1" },
				wrapper,
			},
		);

		await waitFor(() => {
			expect(result.current.chat?.id).toBe("chat-1");
			expect(runtime.subscribeToChat).toHaveBeenCalledTimes(1);
		});
		expect(subscriptions[0]?.input).toEqual({
			chatId: "chat-1",
			afterMessageId: 1,
		});

		rerender({ chatId: "chat-2" });
		await waitFor(() => {
			expect(result.current.chat?.id).toBe("chat-2");
			expect(runtime.subscribeToChat).toHaveBeenCalledTimes(2);
		});
		expect(subscriptions[0]?.dispose).toHaveBeenCalledTimes(1);
		expect(subscriptions[1]?.input).toEqual({
			chatId: "chat-2",
			afterMessageId: 10,
		});

		unmount();
		expect(subscriptions[1]?.dispose).toHaveBeenCalledTimes(1);
	});

	it("applies streamed events to the shared store", async () => {
		const { runtime, subscriptions } = createRuntimeHarness({
			getChat: vi.fn(async (chatId: string) =>
				createChatDetail({
					chat: createChat({ id: chatId }),
					messages: [createMessage({ id: 1, chat_id: chatId, role: "user" })],
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
			expect(subscriptions).toHaveLength(1);
		});

		const emitEvent = subscriptions[0]?.onEvent;
		expect(emitEvent).toBeTruthy();
		if (!emitEvent) {
			throw new Error("Expected a chat subscription callback.");
		}

		act(() => {
			emitEvent({
				type: "message",
				chat_id: "chat-1",
				message: createMessage({
					id: 2,
					chat_id: "chat-1",
					created_at: createTimestamp(2),
				}),
			});
		});
		expect(result.current.snapshot.orderedMessageIDs).toEqual([1, 2]);
		expect(result.current.snapshot.messagesByID.get(2)?.chat_id).toBe("chat-1");

		act(() => {
			emitEvent({
				type: "message_part",
				chat_id: "chat-1",
				message_part: {
					part: { type: "text", text: "stream text" },
				},
			});
			emitEvent({
				type: "message_part",
				chat_id: "chat-1",
				message_part: {
					part: {
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "search",
						args: { q: "repo" },
					},
				},
			});
			emitEvent({
				type: "message_part",
				chat_id: "chat-1",
				message_part: {
					part: {
						type: "tool-result",
						tool_call_id: "tool-1",
						tool_name: "search",
						result: { status: "ok" },
					},
				},
			});
		});
		expect(result.current.snapshot.streamState?.blocks).toEqual([
			{ type: "response", text: "stream text" },
			{ type: "tool", id: "tool-1" },
		]);
		expect(
			result.current.snapshot.streamState?.toolCalls.tool_1,
		).toBeUndefined();
		expect(
			result.current.snapshot.streamState?.toolCalls["tool-1"],
		).toMatchObject({
			id: "tool-1",
			name: "search",
			args: { q: "repo" },
		});
		expect(
			result.current.snapshot.streamState?.toolResults["tool-1"],
		).toMatchObject({
			id: "tool-1",
			name: "search",
			result: { status: "ok" },
			isError: false,
		});

		const queuedMessage = createQueuedMessage({ id: 3, chat_id: "chat-1" });
		act(() => {
			emitEvent({
				type: "queue_update",
				chat_id: "chat-1",
				queued_messages: [queuedMessage],
			});
		});
		expect(result.current.snapshot.queuedMessages).toEqual([queuedMessage]);

		act(() => {
			emitEvent({
				type: "status",
				chat_id: "chat-1",
				status: { status: "running" },
			});
			emitEvent({
				type: "status",
				chat_id: "subagent-1",
				status: { status: "paused" },
			});
		});
		expect(result.current.snapshot.chatStatus).toBe("running");
		expect(
			result.current.snapshot.subagentStatusOverrides.get("subagent-1"),
		).toBe("paused");

		act(() => {
			emitEvent({
				type: "error",
				chat_id: "chat-1",
				error: { message: "  rate limited  " },
			});
		});
		expect(result.current.snapshot.chatStatus).toBe("error");
		expect(result.current.snapshot.streamError).toBe("rate limited");
		expect(result.current.snapshot.retryState).toBeNull();

		act(() => {
			emitEvent({
				type: "retry",
				chat_id: "chat-1",
				retry: { attempt: 2, error: "retrying" },
			});
		});
		expect(result.current.snapshot.retryState).toEqual({
			attempt: 2,
			error: "retrying",
		});
		expect(result.current.snapshot.streamState).toBeNull();
	});

	it("ignores mismatched message, message_part, and queue_update events", async () => {
		const { runtime, subscriptions } = createRuntimeHarness({
			getChat: vi.fn(async (chatId: string) =>
				createChatDetail({
					chat: createChat({ id: chatId }),
					messages: [createMessage({ id: 1, chat_id: chatId })],
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
			expect(subscriptions).toHaveLength(1);
		});

		const emitEvent = subscriptions[0]?.onEvent;
		if (!emitEvent) {
			throw new Error("Expected a chat subscription callback.");
		}

		act(() => {
			emitEvent({
				type: "message",
				chat_id: "chat-2",
				message: createMessage({ id: 2, chat_id: "chat-2" }),
			});
			emitEvent({
				type: "message_part",
				chat_id: "chat-2",
				message_part: {
					part: { type: "text", text: "ignore me" },
				},
			});
			emitEvent({
				type: "queue_update",
				chat_id: "chat-2",
				queued_messages: [createQueuedMessage({ id: 5, chat_id: "chat-2" })],
			});
		});

		expect(result.current.snapshot.orderedMessageIDs).toEqual([1]);
		expect(result.current.snapshot.messagesByID.get(2)).toBeUndefined();
		expect(result.current.snapshot.streamState).toBeNull();
		expect(result.current.snapshot.queuedMessages).toEqual([]);
	});

	it("notifies selectedModel subscribers when adapting a non-subscribing preference store", async () => {
		const runtime = createFakeRuntime();
		const preferenceStore = createPreferenceStore({}, { withSubscribe: false });
		const wrapper = createWrapper(runtime, preferenceStore);
		const { result } = renderHook(() => useChatPreferences(), { wrapper });

		expect(result.current.selectedModel).toBeUndefined();

		act(() => {
			result.current.setSelectedModel("model-2");
		});

		await waitFor(() => {
			expect(result.current.selectedModel).toBe("model-2");
		});
		expect(preferenceStore.get("selectedModel", undefined)).toBe("model-2");
	});
});
