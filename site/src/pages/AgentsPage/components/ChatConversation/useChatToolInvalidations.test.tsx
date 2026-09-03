import { renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { act } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	chatEntityKey,
	chatMessagesKey,
	chatPromptsKey,
	chatsByWorkspace,
} from "#/api/queries/chats";
import { getWorkspaceQuotaQueryKey } from "#/api/queries/workspaceQuota";
import { workspacesQueryKeyPrefix } from "#/api/queries/workspaces";
import { createChatStore } from "./chatStore";
import type { StreamState } from "./types";
import { useChatToolInvalidations } from "./useChatToolInvalidations";

const ORGANIZATION_NAME = "coder";
const USERNAME = "alice";

type ToolResultOverrides = Partial<StreamState["toolResults"][string]>;

const createStreamState = (
	name: string,
	id = "tool-1",
	resultOverrides: ToolResultOverrides = {},
): StreamState => ({
	blocks: [],
	toolCalls: {
		[id]: {
			id,
			name,
			args: {},
		},
	},
	toolResults: {
		[id]: {
			id,
			name,
			isError: false,
			...resultOverrides,
		},
	},
	sources: [],
});

const createTestStore = (initial: StreamState | null = null) => {
	const store = createChatStore();
	store.setStreamState(initial);

	return {
		store,
		setStreamState: store.setStreamState,
	};
};

const createWrapper = (queryClient: QueryClient): FC<PropsWithChildren> => {
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("useChatToolInvalidations", () => {
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

	const renderInvalidations = ({
		chatID = "chat-1",
		organizationName = ORGANIZATION_NAME,
		username = USERNAME,
	}: {
		chatID?: string;
		organizationName?: string;
		username?: string;
	} = {}) => {
		const { store, setStreamState } = createTestStore();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const result = renderHook(
			(props: { chatID: string; organizationName: string; username: string }) =>
				useChatToolInvalidations({
					store,
					...props,
				}),
			{
				initialProps: { chatID, organizationName, username },
				wrapper: createWrapper(queryClient),
			},
		);

		return {
			...result,
			invalidateSpy,
			setStreamState,
		};
	};

	it("dispatches chat binding and workspace mutation invalidations on create_workspace completion", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(createStreamState("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: chatEntityKey("chat-1"),
				exact: true,
			});
			expect(invalidateSpy).toHaveBeenCalledWith(
				expect.objectContaining({
					queryKey: workspacesQueryKeyPrefix,
					predicate: expect.any(Function),
				}),
			);
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: getWorkspaceQuotaQueryKey(ORGANIZATION_NAME, USERNAME),
				exact: true,
			});
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});
	});

	it("does not invalidate chat sub-resources on create_workspace completion", async () => {
		queryClient.setQueryData(chatEntityKey("chat-1"), { id: "chat-1" });
		queryClient.setQueryData(chatMessagesKey("chat-1"), {
			pages: [],
			pageParams: [],
		});
		queryClient.setQueryData(chatPromptsKey("chat-1"), { prompts: [] });
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-1",
		});
		const { setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(createStreamState("create_workspace"));
		});

		await waitFor(() => {
			expect(
				queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
				"detail entry should be invalidated",
			).toBe(true);
		});
		expect(
			queryClient.getQueryState(chatsByWorkspace(["ws-1"]).queryKey)
				?.isInvalidated,
			"by-workspace entry should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey("chat-1"))?.isInvalidated,
			"messages entry should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatPromptsKey("chat-1"))?.isInvalidated,
			"prompts entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("dispatches workspace mutation invalidations on start_workspace completion", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(createStreamState("start_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledWith(
				expect.objectContaining({
					queryKey: workspacesQueryKeyPrefix,
					predicate: expect.any(Function),
				}),
			);
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: getWorkspaceQuotaQueryKey(ORGANIZATION_NAME, USERNAME),
				exact: true,
			});
			expect(invalidateSpy).toHaveBeenCalledTimes(2);
		});

		expect(invalidateSpy).not.toHaveBeenCalledWith({
			queryKey: chatEntityKey("chat-1"),
			exact: true,
		});
	});

	it("dispatches workspace mutation invalidations for errored workspace tools", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(
				createStreamState("stop_workspace", "tool-1", { isError: true }),
			);
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledWith(
				expect.objectContaining({
					queryKey: workspacesQueryKeyPrefix,
					predicate: expect.any(Function),
				}),
			);
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: getWorkspaceQuotaQueryKey(ORGANIZATION_NAME, USERNAME),
				exact: true,
			});
			expect(invalidateSpy).toHaveBeenCalledTimes(2);
		});
	});

	it("invalidates workspace queries without quota when the quota key is incomplete", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations({
			organizationName: "",
		});

		await act(async () => {
			setStreamState(createStreamState("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: chatEntityKey("chat-1"),
				exact: true,
			});
			expect(invalidateSpy).toHaveBeenCalledWith(
				expect.objectContaining({
					queryKey: workspacesQueryKeyPrefix,
					predicate: expect.any(Function),
				}),
			);
			expect(invalidateSpy).toHaveBeenCalledTimes(3);
		});
	});

	it("does not invalidate queries for tools without invalidation signals", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(createStreamState("read_file"));
		});

		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("does not process the same tool call ID twice", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations();
		const streamState = createStreamState("create_workspace");

		await act(async () => {
			setStreamState(streamState);
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});

		await act(async () => {
			setStreamState(streamState);
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});
	});

	it("resets processed tool call IDs when chatID changes", async () => {
		const { invalidateSpy, rerender, setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(createStreamState("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});

		rerender({
			chatID: "chat-2",
			organizationName: ORGANIZATION_NAME,
			username: USERNAME,
		});

		await act(async () => {
			setStreamState(createStreamState("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(8);
			expect(invalidateSpy).toHaveBeenCalledWith({
				queryKey: chatEntityKey("chat-2"),
				exact: true,
			});
		});
	});

	it("waits for completed tool results before invalidating", async () => {
		const { invalidateSpy, setStreamState } = renderInvalidations();

		await act(async () => {
			setStreamState(
				createStreamState("create_workspace", "tool-1", {
					isStreaming: true,
				}),
			);
		});

		expect(invalidateSpy).not.toHaveBeenCalled();

		await act(async () => {
			setStreamState(createStreamState("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});
	});

	it("does nothing when streamState is null", () => {
		const { store } = createTestStore(null);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		renderHook(
			() =>
				useChatToolInvalidations({
					store,
					chatID: "chat-1",
					organizationName: ORGANIZATION_NAME,
					username: USERNAME,
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(invalidateSpy).not.toHaveBeenCalled();
	});
});
