import { act, renderHook, waitFor } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { createElement, type FC, type PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatPreferenceStore, ChatRuntime } from "../core";
import { ChatRuntimeProvider } from "./ChatRuntimeProvider";
import { useChatPreferences } from "./useChatPreferences";

const createFakeRuntime = (overrides: Partial<ChatRuntime> = {}) => {
	const runtime = {
		listChats: vi.fn(async () => [] as readonly TypesGen.Chat[]),
		getChat: vi.fn(async (chatId: string) => ({
			chat: {
				id: chatId,
				owner_id: "owner-1",
				last_model_config_id: "model-config-1",
				title: "Chat",
				status: "completed" as TypesGen.ChatStatus,
				last_error: null,
				created_at: new Date(Date.UTC(2025, 0, 1)).toISOString(),
				updated_at: new Date(Date.UTC(2025, 0, 1, 0, 0, 1)).toISOString(),
				archived: false,
			},
			messages: [],
			queued_messages: [],
		})),
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
	preferenceStore: ChatPreferenceStore,
): FC<PropsWithChildren> => {
	return ({ children }) =>
		createElement(ChatRuntimeProvider, { runtime, preferenceStore, children });
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useChatPreferences", () => {
	it("defaults selectedModel to undefined", () => {
		const wrapper = createWrapper(createFakeRuntime(), createPreferenceStore());
		const { result } = renderHook(() => useChatPreferences(), { wrapper });

		expect(result.current.selectedModel).toBeUndefined();
	});

	it("updates selectedModel through the convenience setter", async () => {
		const preferenceStore = createPreferenceStore();
		const wrapper = createWrapper(createFakeRuntime(), preferenceStore);
		const { result } = renderHook(() => useChatPreferences(), { wrapper });

		act(() => {
			result.current.setSelectedModel("gpt-4o-mini");
		});

		await waitFor(() => {
			expect(result.current.selectedModel).toBe("gpt-4o-mini");
		});
		expect(preferenceStore.get("selectedModel", undefined)).toBe("gpt-4o-mini");
	});

	it("supports generic get and set operations for arbitrary keys", () => {
		const preferenceStore = createPreferenceStore({ theme: "dark" });
		const wrapper = createWrapper(createFakeRuntime(), preferenceStore);
		const { result } = renderHook(() => useChatPreferences(), { wrapper });

		expect(result.current.get("theme", "light")).toBe("dark");
		expect(result.current.get("missing", 7)).toBe(7);

		act(() => {
			result.current.set("pageSize", 50);
		});

		expect(preferenceStore.get("pageSize", 0)).toBe(50);
	});

	it("reacts to external updates from a natively subscribing store", async () => {
		const preferenceStore = createPreferenceStore();
		const wrapper = createWrapper(createFakeRuntime(), preferenceStore);
		const { result } = renderHook(() => useChatPreferences(), { wrapper });

		act(() => {
			preferenceStore.set("selectedModel", "claude-3.7");
		});

		await waitFor(() => {
			expect(result.current.selectedModel).toBe("claude-3.7");
		});
	});

	it("propagates provider writes for a non-subscribing backing store", async () => {
		const preferenceStore = createPreferenceStore({}, { withSubscribe: false });
		const wrapper = createWrapper(createFakeRuntime(), preferenceStore);
		const { result } = renderHook(() => useChatPreferences(), { wrapper });

		act(() => {
			result.current.set("selectedModel", "llama-3.3");
		});

		await waitFor(() => {
			expect(result.current.selectedModel).toBe("llama-3.3");
		});
	});
});
