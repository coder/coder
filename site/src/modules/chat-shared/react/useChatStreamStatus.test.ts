import { act, renderHook } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { createElement, type FC, type PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatPreferenceStore, ChatRuntime } from "../core";
import {
	ChatRuntimeProvider,
	useChatRuntimeContext,
} from "./ChatRuntimeProvider";
import { useChatStreamStatus } from "./useChatStreamStatus";

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

describe("useChatStreamStatus", () => {
	it("reports an idle baseline for a fresh provider", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(() => useChatStreamStatus(), { wrapper });

		expect(result.current.chatStatus).toBeNull();
		expect(result.current.streamState).toBeNull();
		expect(result.current.streamError).toBeNull();
		expect(result.current.retryState).toBeNull();
		expect(result.current.subagentStatusOverrides.size).toBe(0);
		expect(result.current.isStreaming).toBe(false);
		expect(result.current.isRetrying).toBe(false);
		expect(result.current.hasError).toBe(false);
		expect(result.current.isIdle).toBe(true);
	});

	it("reports streaming when the chat status is running", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const status = useChatStreamStatus();
				return { context, status };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.setChatStatus("running");
		});

		expect(result.current.status.chatStatus).toBe("running");
		expect(result.current.status.isStreaming).toBe(true);
		expect(result.current.status.isIdle).toBe(false);
	});

	it("reports streaming when stream state exists without a running status", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const status = useChatStreamStatus();
				return { context, status };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.setChatStatus("completed");
			result.current.context.store.applyMessagePart({
				type: "text",
				text: "streaming",
			});
		});

		expect(result.current.status.chatStatus).toBe("completed");
		expect(result.current.status.streamState?.blocks).toEqual([
			{ type: "response", text: "streaming" },
		]);
		expect(result.current.status.isStreaming).toBe(true);
		expect(result.current.status.isIdle).toBe(false);
	});

	it("reports retrying when retry state exists", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const status = useChatStreamStatus();
				return { context, status };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.setRetryState({
				attempt: 3,
				error: "retry later",
			});
		});

		expect(result.current.status.retryState).toEqual({
			attempt: 3,
			error: "retry later",
		});
		expect(result.current.status.isRetrying).toBe(true);
		expect(result.current.status.isIdle).toBe(false);
	});

	it("reports errors from the chat status and keeps raw fields aligned", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const status = useChatStreamStatus();
				return { context, status };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.setChatStatus("error");
			result.current.context.store.setSubagentStatusOverride(
				"subagent-1",
				"paused",
			);
		});

		expect(result.current.status.chatStatus).toBe("error");
		expect(result.current.status.hasError).toBe(true);
		expect(
			result.current.status.subagentStatusOverrides.get("subagent-1"),
		).toBe("paused");
		expect(result.current.status.isIdle).toBe(false);
	});

	it("reports errors from streamError even when the chat status is not error", () => {
		const wrapper = createWrapper(createFakeRuntime());
		const { result } = renderHook(
			() => {
				const context = useChatRuntimeContext();
				const status = useChatStreamStatus();
				return { context, status };
			},
			{ wrapper },
		);

		act(() => {
			result.current.context.store.setChatStatus("completed");
			result.current.context.store.setStreamError("broken stream");
		});

		expect(result.current.status.chatStatus).toBe("completed");
		expect(result.current.status.streamError).toBe("broken stream");
		expect(result.current.status.hasError).toBe(true);
		expect(result.current.status.isIdle).toBe(false);
	});
});
