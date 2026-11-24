import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	chatEntityKey,
	chatPromptsKey,
	chatsByWorkspace,
} from "#/api/queries/chats";
import { getWorkspaceQuotaQueryKey } from "#/api/queries/workspaceQuota";
import { workspacesQueryKeyPrefix } from "#/api/queries/workspaces";
import { useChatToolInvalidations } from "./useChatToolInvalidations";

const ORGANIZATION_NAME = "coder";
const USERNAME = "alice";

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
		organizationName = ORGANIZATION_NAME,
	}: {
		organizationName?: string;
	} = {}) => {
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const { result } = renderHook(
			() =>
				useChatToolInvalidations({
					chatID: "chat-1",
					organizationName,
					username: USERNAME,
				}),
			{ wrapper: createWrapper(queryClient) },
		);

		return { invalidateSpy, invalidate: result.current };
	};

	it("dispatches chat binding and workspace mutation invalidations on create_workspace completion", async () => {
		const { invalidateSpy, invalidate } = renderInvalidations();

		invalidate(["create_workspace"]);

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
		const { invalidate } = renderInvalidations();

		invalidate(["create_workspace"]);

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
		const { invalidateSpy, invalidate } = renderInvalidations();

		invalidate(["start_workspace"]);

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

	it("invalidates workspace queries without quota when the quota key is incomplete", async () => {
		const { invalidateSpy, invalidate } = renderInvalidations({
			organizationName: "",
		});

		invalidate(["create_workspace"]);

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
		const { invalidateSpy, invalidate } = renderInvalidations();

		invalidate(["read_file"]);

		expect(invalidateSpy).not.toHaveBeenCalled();
	});
});
