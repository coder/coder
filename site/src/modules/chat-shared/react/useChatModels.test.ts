import { act, renderHook, waitFor } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { createElement, type FC, type PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
	ChatModelOption,
	ChatPreferenceStore,
	ChatRuntime,
} from "../core";
import { ChatRuntimeProvider } from "./ChatRuntimeProvider";
import { useChatModels } from "./useChatModels";

const createDeferred = <T>() => {
	let resolve!: (value: T | PromiseLike<T>) => void;
	let reject!: (reason?: unknown) => void;
	const promise = new Promise<T>((nextResolve, nextReject) => {
		resolve = nextResolve;
		reject = nextReject;
	});
	return { promise, resolve, reject };
};

const createModel = (
	overrides: Partial<ChatModelOption> & { id?: string } = {},
): ChatModelOption => ({
	id: overrides.id ?? "model-1",
	provider: overrides.provider ?? "openai",
	model: overrides.model ?? "gpt-4o",
	displayName: overrides.displayName ?? "GPT-4o",
});

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
		listModels: vi.fn(async () => [] as readonly ChatModelOption[]),
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

describe("useChatModels", () => {
	it("loads models on mount and reports loading until the promise resolves", async () => {
		const pendingModels = createDeferred<readonly ChatModelOption[]>();
		const runtime = createFakeRuntime({
			listModels: vi.fn(() => pendingModels.promise),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(() => useChatModels(), { wrapper });

		await waitFor(() => {
			expect(result.current.isLoading).toBe(true);
		});
		expect(runtime.listModels).toHaveBeenCalledTimes(1);
		expect(result.current.models).toEqual([]);
		expect(result.current.error).toBeNull();

		const models = [
			createModel({ id: "model-1" }),
			createModel({ id: "model-2" }),
		];
		act(() => {
			pendingModels.resolve(models);
		});

		await waitFor(() => {
			expect(result.current.isLoading).toBe(false);
		});
		expect(result.current.models).toEqual(models);
		expect(result.current.error).toBeNull();
	});

	it("stores rejected model loads as an error", async () => {
		const error = new Error("models unavailable");
		const runtime = createFakeRuntime({
			listModels: vi.fn(async () => {
				throw error;
			}),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(() => useChatModels(), { wrapper });

		await waitFor(() => {
			expect(result.current.isLoading).toBe(false);
			expect(result.current.error).toBe(error);
		});
		expect(result.current.models).toEqual([]);
	});

	it("deduplicates concurrent consumers within the same provider", async () => {
		const pendingModels = createDeferred<readonly ChatModelOption[]>();
		const runtime = createFakeRuntime({
			listModels: vi.fn(() => pendingModels.promise),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(
			() => [useChatModels(), useChatModels()] as const,
			{ wrapper },
		);

		await waitFor(() => {
			expect(result.current[0].isLoading).toBe(true);
			expect(result.current[1].isLoading).toBe(true);
		});
		expect(runtime.listModels).toHaveBeenCalledTimes(1);

		const sharedModels = [createModel({ id: "shared-model" })];
		act(() => {
			pendingModels.resolve(sharedModels);
		});

		await waitFor(() => {
			expect(result.current[0].isLoading).toBe(false);
			expect(result.current[1].isLoading).toBe(false);
		});
		expect(result.current[0].models).toEqual(sharedModels);
		expect(result.current[1].models).toEqual(sharedModels);
	});

	it("refetches models and ignores stale older responses", async () => {
		const initialLoad = createDeferred<readonly ChatModelOption[]>();
		const staleRefetch = createDeferred<readonly ChatModelOption[]>();
		const freshRefetch = createDeferred<readonly ChatModelOption[]>();
		const runtime = createFakeRuntime({
			listModels: vi
				.fn()
				.mockImplementationOnce(() => initialLoad.promise)
				.mockImplementationOnce(() => staleRefetch.promise)
				.mockImplementationOnce(() => freshRefetch.promise),
		});
		const wrapper = createWrapper(runtime);
		const { result } = renderHook(() => useChatModels(), { wrapper });

		await waitFor(() => {
			expect(result.current.isLoading).toBe(true);
		});

		const initialModels = [createModel({ id: "initial-model" })];
		act(() => {
			initialLoad.resolve(initialModels);
		});
		await waitFor(() => {
			expect(result.current.models).toEqual(initialModels);
		});

		let firstRefetch!: Promise<void>;
		let secondRefetch!: Promise<void>;
		act(() => {
			firstRefetch = result.current.refetch();
			secondRefetch = result.current.refetch();
		});

		await waitFor(() => {
			expect(runtime.listModels).toHaveBeenCalledTimes(3);
			expect(result.current.isLoading).toBe(true);
		});
		expect(result.current.models).toEqual([]);

		const latestModels = [createModel({ id: "latest-model" })];
		act(() => {
			freshRefetch.resolve(latestModels);
		});
		await waitFor(() => {
			expect(result.current.isLoading).toBe(false);
			expect(result.current.models).toEqual(latestModels);
		});

		act(() => {
			staleRefetch.resolve([createModel({ id: "stale-model" })]);
		});
		await act(async () => {
			await Promise.all([firstRefetch, secondRefetch]);
		});

		expect(result.current.models).toEqual(latestModels);
		expect(result.current.error).toBeNull();
	});
});
