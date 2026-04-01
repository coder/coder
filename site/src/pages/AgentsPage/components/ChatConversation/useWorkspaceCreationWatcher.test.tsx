import { renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { act } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StreamState } from "./types";
import { useWorkspaceCreationWatcher } from "./useWorkspaceCreationWatcher";

type ChatStoreHandle = Parameters<
	typeof useWorkspaceCreationWatcher
>[0]["store"];

const createStreamState = (
	toolCalls: StreamState["toolCalls"],
	toolResults: StreamState["toolResults"] = {},
): StreamState => ({
	blocks: [],
	toolCalls,
	toolResults,
	sources: [],
});

type MinimalChatStoreState = Pick<
	ReturnType<ChatStoreHandle["getSnapshot"]>,
	"streamState"
>;

const createTestStore = (initial: StreamState | null = null) => {
	let state: MinimalChatStoreState = { streamState: initial };
	const listeners = new Set<() => void>();

	const store: Pick<ChatStoreHandle, "getSnapshot" | "subscribe"> = {
		getSnapshot: () => state as ReturnType<ChatStoreHandle["getSnapshot"]>,
		subscribe: (listener: () => void) => {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		},
	};

	return {
		store: store as ChatStoreHandle,
		setStreamState: (streamState: StreamState | null) => {
			state = { streamState };
			for (const listener of listeners) {
				listener();
			}
		},
	};
};

const createWrapper = (queryClient: QueryClient): FC<PropsWithChildren> => {
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("useWorkspaceCreationWatcher", () => {
	let queryClient: QueryClient;

	beforeEach(() => {
		queryClient = new QueryClient({
			defaultOptions: {
				queries: {
					retry: false,
				},
			},
		});
	});

	it("invalidates chatKey on create_workspace tool result", async () => {
		const { store, setStreamState } = createTestStore();

		renderHook(
			() =>
				useWorkspaceCreationWatcher({
					store,
					chatID: "chat-1",
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			setStreamState(
				createStreamState(
					{
						"tool-1": {
							id: "tool-1",
							name: "create_workspace",
							args: { template: "some-template" },
						},
					},
					{
						"tool-1": {
							id: "tool-1",
							name: "create_workspace",
							isError: false,
						},
					},
				),
			);
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: ["chats", "chat-1"],
			});
		});

		invalidateSpy.mockRestore();
	});

	it("does not invalidate chat for non-workspace tools", async () => {
		const { store, setStreamState } = createTestStore();

		renderHook(
			() =>
				useWorkspaceCreationWatcher({
					store,
					chatID: "chat-1",
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			setStreamState(
				createStreamState(
					{
						"tool-1": {
							id: "tool-1",
							name: "read_file",
							args: { path: "/workspace/src/main.ts" },
						},
					},
					{
						"tool-1": {
							id: "tool-1",
							name: "read_file",
							isError: false,
						},
					},
				),
			);
		});

		await waitFor(() => {
			expect(invalidateSpy).not.toHaveBeenCalled();
		});

		invalidateSpy.mockRestore();
	});

	it("does not process the same tool call ID twice", async () => {
		const { store, setStreamState } = createTestStore();

		renderHook(
			() =>
				useWorkspaceCreationWatcher({
					store,
					chatID: "chat-1",
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const toolCalls = {
			"tool-1": {
				id: "tool-1",
				name: "create_workspace",
				args: { template: "some-template" },
			},
		};
		const toolResults = {
			"tool-1": {
				id: "tool-1",
				name: "create_workspace",
				isError: false,
			},
		};

		await act(async () => {
			setStreamState(createStreamState(toolCalls, toolResults));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(1);
		});

		// Re-emit the same stream state (simulates a re-render).
		await act(async () => {
			setStreamState(createStreamState(toolCalls, toolResults));
		});

		// invalidateQueries should still have been called only once.
		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(1);
		});

		invalidateSpy.mockRestore();
	});

	it("resets processed tool call IDs when chatID changes", async () => {
		const { store, setStreamState } = createTestStore();

		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const { rerender } = renderHook(
			({ chatID }: { chatID: string }) =>
				useWorkspaceCreationWatcher({
					store,
					chatID,
				}),
			{
				initialProps: { chatID: "chat-1" },
				wrapper: createWrapper(queryClient),
			},
		);

		await act(async () => {
			setStreamState(
				createStreamState(
					{
						"tool-1": {
							id: "tool-1",
							name: "create_workspace",
							args: { template: "some-template" },
						},
					},
					{
						"tool-1": {
							id: "tool-1",
							name: "create_workspace",
							isError: false,
						},
					},
				),
			);
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(1);
		});

		// Switch to a new chat and emit the same tool call ID.
		rerender({ chatID: "chat-2" });

		await act(async () => {
			setStreamState(
				createStreamState(
					{
						"tool-1": {
							id: "tool-1",
							name: "create_workspace",
							args: { template: "some-template" },
						},
					},
					{
						"tool-1": {
							id: "tool-1",
							name: "create_workspace",
							isError: false,
						},
					},
				),
			);
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(2);
		});

		invalidateSpy.mockRestore();
	});

	it("does nothing when streamState is null", async () => {
		const { store } = createTestStore(null);

		renderHook(
			() =>
				useWorkspaceCreationWatcher({
					store,
					chatID: "chat-1",
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await waitFor(() => {
			expect(invalidateSpy).not.toHaveBeenCalled();
		});

		invalidateSpy.mockRestore();
	});
});
