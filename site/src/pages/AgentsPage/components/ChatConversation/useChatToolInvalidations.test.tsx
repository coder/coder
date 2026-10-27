import { renderHook, waitFor } from "@testing-library/react";
import { act } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	chatEntityKey,
	chatPromptsKey,
	chatsByWorkspace,
} from "#/api/queries/chats";
import { getWorkspaceQuotaQueryKey } from "#/api/queries/workspaceQuota";
import type * as TypesGen from "#/api/typesGenerated";
import { workspacesQueryKeyPrefix } from "#/api/queries/workspaces";
import { createChatStore } from "./chatStore";
import { useChatToolInvalidations } from "./useChatToolInvalidations";

const ORGANIZATION_NAME = "coder";
const USERNAME = "alice";

const toolResult = (
	name: string,
	overrides: Partial<TypesGen.ChatToolResultPart> = {},
): TypesGen.ChatToolResultPart => ({
	type: "tool-result",
	tool_call_id: "tool-1",
	tool_name: name,
	result: {},
	...overrides,
});

const createWrapper = (
	queryClient: QueryClient,
): React.FC<React.PropsWithChildren> => {
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
		const store = createChatStore();
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
			applyMessagePart: store.applyMessagePart,
		};
	};

	it("dispatches chat binding and workspace mutation invalidations on create_workspace completion", async () => {
		const { invalidateSpy, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
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
		queryClient.setQueryData(chatPromptsKey("chat-1"), { prompts: [] });
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-1",
		});
		const { applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
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
			queryClient.getQueryState(chatPromptsKey("chat-1"))?.isInvalidated,
			"prompts entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("dispatches workspace mutation invalidations on start_workspace completion", async () => {
		const { invalidateSpy, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("start_workspace"));
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
		const { invalidateSpy, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("stop_workspace", { is_error: true }));
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
		const { invalidateSpy, applyMessagePart } = renderInvalidations({
			organizationName: "",
		});

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
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
		const { invalidateSpy, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("read_file"));
		});

		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("does not process the same tool call ID twice", async () => {
		const { invalidateSpy, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});
	});

	it("resets processed tool call IDs when chatID changes", async () => {
		const { invalidateSpy, rerender, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
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
			applyMessagePart(toolResult("create_workspace"));
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
		const { invalidateSpy, applyMessagePart } = renderInvalidations();

		await act(async () => {
			applyMessagePart(
				toolResult("create_workspace", {
					result: undefined,
					result_delta: "{",
				}),
			);
		});

		expect(invalidateSpy).not.toHaveBeenCalled();

		await act(async () => {
			applyMessagePart(toolResult("create_workspace"));
		});

		await waitFor(() => {
			expect(invalidateSpy).toHaveBeenCalledTimes(4);
		});
	});

	it("does nothing when streamState is null", () => {
		const store = createChatStore();
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
