import { act, renderHook } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { createElement, type FC, type PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatPreferenceStore, ChatRuntime } from "../core";
import {
	buildParsedMessageSections,
	parseMessagesWithMergedTools,
} from "../core";
import {
	ChatRuntimeProvider,
	useChatRuntimeContext,
} from "./ChatRuntimeProvider";
import { useChatMessages } from "./useChatMessages";

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

const createFakeRuntime = (overrides: Partial<ChatRuntime> = {}) => {
	const runtime = {
		listChats: vi.fn(async () => [] as readonly TypesGen.Chat[]),
		getChat: vi.fn(async (chatId: string) => ({
			chat: createChat({ id: chatId }),
			messages: [],
			queued_messages: [],
		})),
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

describe("useChatMessages", () => {
	it("returns empty message state for a fresh provider", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(() => useChatMessages(), { wrapper });

		expect(result.current.sections).toEqual([]);
		expect(result.current.queuedMessages).toEqual([]);
		expect(result.current.streamBlocks).toEqual([]);
		expect(result.current.streamTools).toEqual([]);
		expect(result.current.isStreaming).toBe(false);
	});

	it("parses durable messages into sections and passes through queued messages", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const queuedMessage = createQueuedMessage({ id: 7, chat_id: "chat-1" });
		const messages = [
			createMessage({
				id: 1,
				chat_id: "chat-1",
				role: "user",
				content: [{ type: "text", text: "hello" }],
			}),
			createMessage({
				id: 2,
				chat_id: "chat-1",
				role: "assistant",
				content: [
					{ type: "reasoning", text: "thinking" },
					{ type: "text", text: "reply" },
					{
						type: "tool-call",
						tool_call_id: "tool-1",
						tool_name: "search",
						args: { q: "repo" },
					},
				],
			}),
			createMessage({
				id: 3,
				chat_id: "chat-1",
				role: "assistant",
				content: [
					{
						type: "tool-result",
						tool_call_id: "tool-1",
						tool_name: "search",
						result: { matches: "1" },
					},
				],
			}),
			createMessage({
				id: 4,
				chat_id: "chat-1",
				role: "user",
				content: [{ type: "text", text: "follow-up" }],
			}),
		];
		const expectedSections = buildParsedMessageSections(
			parseMessagesWithMergedTools(messages),
		);
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const chatMessages = useChatMessages();
				return { context, chatMessages };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.replaceMessages(messages);
			result.current.context.store.setQueuedMessages([queuedMessage]);
		});

		expect(result.current.chatMessages.sections).toEqual(expectedSections);
		expect(result.current.chatMessages.queuedMessages).toEqual([queuedMessage]);
	});

	it("derives stream blocks and tools from streaming parts", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const chatMessages = useChatMessages();
				return { context, chatMessages };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.applyMessageParts([
				{ type: "text", text: "draft response" },
				{
					type: "tool-call",
					tool_call_id: "tool-1",
					tool_name: "search",
					args: { q: "repo" },
				},
				{
					type: "tool-result",
					tool_call_id: "tool-1",
					tool_name: "search",
					result: { status: "ok" },
				},
			]);
		});

		expect(result.current.chatMessages.streamBlocks).toEqual([
			{ type: "response", text: "draft response" },
			{ type: "tool", id: "tool-1" },
		]);
		expect(result.current.chatMessages.streamTools).toEqual([
			{
				id: "tool-1",
				name: "search",
				args: { q: "repo" },
				result: { status: "ok" },
				isError: false,
				status: "completed",
			},
		]);
		expect(result.current.chatMessages.isStreaming).toBe(true);
	});

	it("reports streaming when the chat status is running without stream parts", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const chatMessages = useChatMessages();
				return { context, chatMessages };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.setChatStatus("running");
		});

		expect(result.current.chatMessages.streamBlocks).toEqual([]);
		expect(result.current.chatMessages.streamTools).toEqual([]);
		expect(result.current.chatMessages.isStreaming).toBe(true);
	});
});
