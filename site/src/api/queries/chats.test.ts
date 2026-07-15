import { InfiniteQueryObserver, QueryClient } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	ERROR_STATUSES,
	SUCCESS_STATUSES,
} from "#/pages/AgentsPage/components/RightPanel/DebugPanel/debugPanelUtils";
import {
	claimChatExecutionOverlay,
	releaseChatExecutionOverlay,
} from "./chatExecutionOverlay";
import { buildOptimisticEditedMessage } from "./chatMessageEdits";
import {
	addChildToParentInCache,
	applyExecutionSnapshotEvent,
	archiveChat,
	type ChatDetailProjection,
	cancelChatListRefetches,
	chat,
	chatACL,
	chatAdvisorConfig,
	chatAdvisorConfigKey,
	chatCostSummary,
	chatCostSummaryKey,
	chatDebugRun,
	chatDebugRuns,
	chatDiffContents,
	chatMessagesForInfiniteScroll,
	chatPromptsQuery,
	chatQueryKeys,
	chatSearch,
	chatsByWorkspace,
	createChat,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	infiniteChats,
	interruptChat,
	invalidateChatListQueries,
	mergeWatchedChatIntoCaches,
	mergeWatchedChatSummary,
	paginatedChatCostUsers,
	pinChat,
	prependToInfiniteChatsCache,
	promoteChatQueuedMessage,
	proposeChatTitle,
	refetchActiveChatMetadataProjections,
	refetchDirtyChatMetadataProjections,
	removeChildFromParentInCache,
	removeDeletedChatFamily,
	reorderPinnedChat,
	replaceCachedChatMessages,
	replaceCachedChatQueuedMessages,
	selectChatMessagesProjection,
	setChatGroupRole,
	setChatUserRole,
	TERMINAL_RUN_STATUSES,
	unarchiveChat,
	unpinChat,
	updateCachedChatExecutionSnapshot,
	updateChatAdvisorConfig,
	updateChatPlanMode,
	updateChatTitle,
	updateChatWorkspace,
	updateChildInParentCache,
	updateInfiniteChatsCache,
	upsertCachedChatMessages,
} from "./chats";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			updateChat: vi.fn(),
			createChat: vi.fn(),
			deleteChatQueuedMessage: vi.fn(),
			getChat: vi.fn(),
			getChats: vi.fn(),
			getChatCostSummary: vi.fn(),
			getChatCostUsers: vi.fn(),
			createChatMessage: vi.fn(),
			getChatMessages: vi.fn(),
			editChatMessage: vi.fn(),
			interruptChat: vi.fn(),
			promoteChatQueuedMessage: vi.fn(),
			proposeChatTitle: vi.fn(),
			getChatAdvisorConfig: vi.fn(),
			updateChatAdvisorConfig: vi.fn(),
			getChatACL: vi.fn(),
			updateChatACL: vi.fn(),
		},
	},
}));

type InfiniteChatsTestOptions = Parameters<typeof infiniteChats>[0];

const infiniteChatsTestKey = infiniteChats().queryKey;

type InfiniteData = {
	pages: TypesGen.Chat[][];
	pageParams: unknown[];
};

/** Seed the infinite chats cache in the format TanStack Query expects. */
const seedInfiniteChats = (
	queryClient: QueryClient,
	chats: TypesGen.Chat[],
	opts?: InfiniteChatsTestOptions,
) => {
	queryClient.setQueryData<InfiniteData>(infiniteChats(opts).queryKey, {
		pages: [chats],
		pageParams: [0],
	});
};

/** Read chats back from the infinite query cache. */
const readInfiniteChats = (
	queryClient: QueryClient,
	opts?: InfiniteChatsTestOptions,
): TypesGen.Chat[] | undefined => {
	const data = queryClient.getQueryData<InfiniteData>(
		infiniteChats(opts).queryKey,
	);
	return data?.pages.flat();
};

const makeChat = (
	id: string,
	overrides?: Partial<TypesGen.Chat>,
): TypesGen.Chat => ({
	id,
	organization_id: "test-org-id",
	owner_id: "owner-1",
	owner_username: "owner",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	title: `Chat ${id}`,
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	shared: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_turn_summary: null,
	children: [],
	...overrides,
});

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

describe("advisor config query factories", () => {
	it("builds the advisor config query and delegates to the API", async () => {
		const advisorConfig: TypesGen.AdvisorConfig = {
			enabled: true,
			max_uses_per_run: 5,
			max_output_tokens: 2048,
			model_config_id: "00000000-0000-0000-0000-000000000000",
		};
		vi.mocked(API.experimental.getChatAdvisorConfig).mockResolvedValue(
			advisorConfig,
		);

		const query = chatAdvisorConfig();

		expect(query.queryKey).toEqual(chatAdvisorConfigKey);
		await expect(query.queryFn()).resolves.toEqual(advisorConfig);
		expect(API.experimental.getChatAdvisorConfig).toHaveBeenCalled();
	});

	it("sends the update request and invalidates the advisor config cache", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatAdvisorConfigKey, {
			enabled: false,
			max_uses_per_run: 0,
			max_output_tokens: 0,
			model_config_id: "",
		} as TypesGen.AdvisorConfig);

		const req: TypesGen.UpdateAdvisorConfigRequest = {
			enabled: true,
			max_uses_per_run: 5,
			max_output_tokens: 2048,
			model_config_id: "00000000-0000-0000-0000-000000000000",
		};
		vi.mocked(API.experimental.updateChatAdvisorConfig).mockResolvedValue();

		const mutation = updateChatAdvisorConfig(queryClient);
		await mutation.mutationFn(req);
		expect(API.experimental.updateChatAdvisorConfig).toHaveBeenCalledWith(req);

		await mutation.onSuccess?.();
		expect(queryClient.getQueryState(chatAdvisorConfigKey)?.isInvalidated).toBe(
			true,
		);
	});
});

describe("invalidateChatListQueries", () => {
	it("invalidates chat list queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(infiniteChats({ archived: false }).queryKey, {
			pages: [[makeChat(chatId)]],
			pageParams: [undefined],
		});
		// Per-chat queries that should NOT be touched.
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatId).queryKey, {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		queryClient.setQueryData(chatQueryKeys.diffContents(chatId), {});
		queryClient.setQueryData(
			chatCostSummaryKey("me", undefined),
			{} as TypesGen.ChatCostSummary,
		);

		await invalidateChatListQueries(queryClient);

		// Sidebar queries should be invalidated.
		expect(
			queryClient.getQueryState(infiniteChats({ archived: false }).queryKey)
				?.isInvalidated,
			"infinite chats should be invalidated",
		).toBe(true);

		// Per-chat queries should NOT be invalidated.
		expect(
			queryClient.getQueryState(chat(chatId).queryKey)?.isInvalidated,
			"exact chat detail should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
				?.isInvalidated,
			"chat messages query should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatQueryKeys.diffContents(chatId))
				?.isInvalidated,
			"chatDiffContentsKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatCostSummaryKey("me", undefined))
				?.isInvalidated,
			"chatCostSummaryKey should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidates the infinite query with undefined opts", async () => {
		const queryClient = createTestQueryClient();

		queryClient.setQueryData(infiniteChats().queryKey, {
			pages: [[makeChat("chat-1")]],
			pageParams: [0],
		});

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(infiniteChats().queryKey)?.isInvalidated,
			"infinite chats with undefined opts should be invalidated",
		).toBe(true);
	});

	it("does not invalidate a different chat's queries", async () => {
		const queryClient = createTestQueryClient();
		const otherChatId = "chat-2";

		queryClient.setQueryData(chat(otherChatId).queryKey, makeChat(otherChatId));
		queryClient.setQueryData(
			chatMessagesForInfiniteScroll(otherChatId).queryKey,
			{
				pages: [{ messages: [], queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chat(otherChatId).queryKey)?.isInvalidated,
			"other chat's exact chat detail should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(
				chatMessagesForInfiniteScroll(otherChatId).queryKey,
			)?.isInvalidated,
			"other chat's chat messages query should NOT be invalidated",
		).not.toBe(true);
	});

	it("prepends new root chats to filtered list caches", () => {
		const queryClient = createTestQueryClient();
		const activeChat = makeChat("active-created", { archived: false });

		seedInfiniteChats(queryClient, [makeChat("active-existing")], {
			archived: false,
		});

		prependToInfiniteChatsCache(queryClient, activeChat);

		expect(readInfiniteChats(queryClient, { archived: false })?.[0]).toEqual(
			activeChat,
		);
	});
});

describe("removeDeletedChatFamily", () => {
	it("removes complete descendant families without treating subresources as details", () => {
		const queryClient = createTestQueryClient();
		const root = makeChat("root");
		const child = makeChat("child", {
			parent_chat_id: root.id,
			root_chat_id: root.id,
		});
		const rootWithChild = { ...root, children: [child] };
		const unrelated = makeChat("unrelated");
		const aclShapedDetail = makeChat("acl-shaped");
		const aclShapedLikeDescendant = makeChat(aclShapedDetail.id, {
			root_chat_id: root.id,
		});

		queryClient.setQueryData(chat(root.id).queryKey, rootWithChild);
		queryClient.setQueryData(chat(child.id).queryKey, child);
		queryClient.setQueryData(chat(unrelated.id).queryKey, unrelated);
		queryClient.setQueryData(
			chat(aclShapedDetail.id).queryKey,
			aclShapedDetail,
		);
		queryClient.setQueryData(
			chatQueryKeys.acl(root.id),
			aclShapedLikeDescendant,
		);
		queryClient.setQueryData(chatMessagesForInfiniteScroll(root.id).queryKey, {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		queryClient.setQueryData(chatMessagesForInfiniteScroll(child.id).queryKey, {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		const workspaceIDs = [
			"workspace-root",
			"workspace-child",
			"workspace-other",
		];
		queryClient.setQueryData(chatsByWorkspace(workspaceIDs).queryKey, {
			"workspace-root": root.id,
			"workspace-child": child.id,
			"workspace-other": unrelated.id,
		});
		seedInfiniteChats(queryClient, [rootWithChild, unrelated]);

		removeDeletedChatFamily(queryClient, rootWithChild);

		expect(queryClient.getQueryState(chat(root.id).queryKey)).toBeUndefined();
		expect(
			queryClient.getQueryState(
				chatMessagesForInfiniteScroll(root.id).queryKey,
			),
		).toBeUndefined();
		expect(queryClient.getQueryState(chat(child.id).queryKey)).toBeUndefined();
		expect(
			queryClient.getQueryState(
				chatMessagesForInfiniteScroll(child.id).queryKey,
			),
		).toBeUndefined();
		expect(queryClient.getQueryData(chat(unrelated.id).queryKey)).toEqual(
			unrelated,
		);
		expect(queryClient.getQueryData(chat(aclShapedDetail.id).queryKey)).toEqual(
			aclShapedDetail,
		);
		expect(
			queryClient.getQueryData(chatsByWorkspace(workspaceIDs).queryKey),
		).toEqual({ "workspace-other": unrelated.id });
		expect(readInfiniteChats(queryClient)).toEqual([unrelated]);
	});

	it("invalidates by-workspace when an uncached descendant cannot be discovered", () => {
		const queryClient = createTestQueryClient();
		const root = makeChat("root");
		const workspaceIDs = ["workspace-root", "workspace-hidden-child"];
		const workspaceKey = chatsByWorkspace(workspaceIDs).queryKey;
		queryClient.setQueryData(workspaceKey, {
			"workspace-root": root.id,
			"workspace-hidden-child": "hidden-child",
		});

		removeDeletedChatFamily(queryClient, root);

		expect(queryClient.getQueryData(workspaceKey)).toEqual({
			"workspace-hidden-child": "hidden-child",
		});
		expect(queryClient.getQueryState(workspaceKey)?.isInvalidated).toBe(true);
	});
});

describe("updateChatPlanMode optimistic update", () => {
	it("invalidates the chat list on error without a detail cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);

		const mutation = updateChatPlanMode(queryClient);
		const context = await mutation.onMutate({
			chatId,
			planMode: "plan",
		});

		expect(context?.previousChat).toBeUndefined();
		expect(readInfiniteChats(queryClient)?.[0].plan_mode).toBe("plan");

		mutation.onError(
			new Error("server error"),
			{ chatId, planMode: "plan" },
			context,
		);

		expect(
			queryClient.getQueryState(infiniteChatsTestKey)?.isInvalidated,
			"chat list should be invalidated when rollback lacks detail cache",
		).toBe(true);
	});
});

describe("updateChatTitle cache update", () => {
	it("patches chat detail and infinite chat list caches after success", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { title: "Old" }),
		);
		seedInfiniteChats(queryClient, [
			makeChat(chatId, { title: "Old" }),
			makeChat("chat-2", { title: "Other" }),
		]);
		seedInfiniteChats(
			queryClient,
			[makeChat(chatId, { archived: true, title: "Old" })],
			{ archived: true },
		);
		const workspaceMap = { "workspace-1": chatId };
		queryClient.setQueryData(
			chatsByWorkspace(Object.keys(workspaceMap)).queryKey,
			workspaceMap,
		);

		const mutation = updateChatTitle(queryClient);
		mutation.onSuccess(undefined, { chatId, title: "New" });

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.title,
		).toBe("New");
		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId),
		).toMatchObject({ title: "New" });
		expect(
			readInfiniteChats(queryClient, { archived: true })?.find(
				(chat) => chat.id === chatId,
			),
		).toMatchObject({ title: "New" });
		expect(
			queryClient.getQueryData(
				chatsByWorkspace(Object.keys(workspaceMap)).queryKey,
			),
		).toEqual(workspaceMap);
	});

	it("does not return pending invalidation promises from settlement", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi
			.spyOn(queryClient, "invalidateQueries")
			.mockReturnValue(new Promise<void>(() => {}));

		const mutation = updateChatTitle(queryClient);
		const result = mutation.onSettled(undefined, undefined, {
			chatId,
			title: "New",
		});

		expect(result).toBeUndefined();
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});
		invalidateSpy.mockRestore();
	});
});

describe("archiveChat optimistic update", () => {
	it("optimistically sets archived to true in the chats list", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialChats = [makeChat(chatId), makeChat("chat-2")];
		seedInfiniteChats(queryClient, initialChats);

		vi.mocked(API.experimental.updateChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const updatedChats = readInfiniteChats(queryClient);
		expect(updatedChats).toHaveLength(2);
		expect(updatedChats?.find((c) => c.id === chatId)?.archived).toBe(true);
		// Other chats are unchanged.
		expect(updatedChats?.find((c) => c.id === "chat-2")?.archived).toBe(false);
	});

	it("optimistically sets archived to true in the individual chat cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));

		vi.mocked(API.experimental.updateChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const cachedChat = queryClient.getQueryData<TypesGen.Chat>(
			chat(chatId).queryKey,
		);
		expect(cachedChat?.archived).toBe(true);
	});

	it("repairs the full loaded family after archiving a root chat", async () => {
		const queryClient = createTestQueryClient();
		const root = makeChat("root");
		const child = makeChat("child", {
			parent_chat_id: root.id,
			root_chat_id: root.id,
		});
		const rootWithChild = { ...root, children: [child] };
		queryClient.setQueryData(chat(root.id).queryKey, rootWithChild);
		queryClient.setQueryData(chat(child.id).queryKey, child);
		queryClient.setQueryData(chatSearch("family").queryKey, [
			rootWithChild,
			child,
		]);
		queryClient.setQueryData(
			chatsByWorkspace(["root-workspace", "child-workspace"]).queryKey,
			{
				"root-workspace": root.id,
				"child-workspace": child.id,
			},
		);
		seedInfiniteChats(queryClient, [rootWithChild], { archived: true });

		const mutation = archiveChat(queryClient);
		mutation.onSuccess(undefined, root.id);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(child.id).queryKey),
		).toMatchObject({ archived: true, pin_order: 0 });
		expect(queryClient.getQueryData(chatSearch("family").queryKey)).toEqual([]);
		expect(
			queryClient.getQueryData(
				chatsByWorkspace(["root-workspace", "child-workspace"]).queryKey,
			),
		).toEqual({});
		expect(
			readInfiniteChats(queryClient, { archived: true })?.[0],
		).toMatchObject({
			archived: true,
			children: [expect.objectContaining({ archived: true, pin_order: 0 })],
		});

		mutation.onSettled(undefined, undefined, root.id);
		await new Promise((resolve) => setTimeout(resolve, 0));
		expect(
			queryClient.getQueryState(chat(root.id).queryKey)?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chat(child.id).queryKey)?.isInvalidated,
		).toBe(true);
	});

	it("removes an archived root chat from active filtered lists after success", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(
			queryClient,
			[
				makeChat(chatId, { pin_order: 2 }),
				makeChat("chat-2", { archived: false }),
			],
			{ archived: false },
		);
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { pin_order: 2 }),
		);

		const mutation = archiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		expect(
			readInfiniteChats(queryClient, { archived: false })?.map(
				(chat) => chat.id,
			),
		).toEqual(["chat-2"]);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey),
		).toMatchObject({
			archived: true,
			pin_order: 0,
		});
	});

	it("clears pin order for archived chats that remain in unfiltered lists", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })]);

		const mutation = archiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			archived: true,
			pin_order: 0,
		});
	});

	it("rolls back the chats list on error by invalidating", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialChats = [makeChat(chatId)];
		seedInfiniteChats(queryClient, initialChats);
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify the optimistic update took effect.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);

		// Simulate an error, the onError handler invalidates the
		// cache so a re-fetch restores the correct state.
		mutation.onError(new Error("server error"), chatId, context);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
	});

	it("rolls back the individual chat cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.archived,
		).toBe(true);

		mutation.onError(new Error("server error"), chatId, context);

		const rolledBack = queryClient.getQueryData<TypesGen.Chat>(
			chat(chatId).queryKey,
		);
		expect(rolledBack?.archived).toBe(false);
	});

	it("handles error rollback gracefully when context is undefined", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);

		// Calling onError with undefined context should not throw.
		expect(() => {
			mutation.onError(new Error("fail"), chatId, undefined);
		}).not.toThrow();

		// The handler should still invalidate to trigger a refetch.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
	});

	it("handles onMutate when no individual chat cache exists", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		// Deliberately do NOT set chat(chatId).queryKey data.

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// The list should still be optimistically updated.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);
		// previousChat should be undefined.
		expect(context?.previousChat).toBeUndefined();
	});

	it("invalidates on settled without returning pending promises", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		// Mock invalidateQueries to never resolve so a regression back to
		// an awaited (async) onSettled surfaces as a pending promise return
		// value, which is what keeps the mutation's loading state stuck.
		const invalidateSpy = vi
			.spyOn(queryClient, "invalidateQueries")
			.mockReturnValue(new Promise<void>(() => {}));

		const mutation = archiveChat(queryClient);
		const result = mutation.onSettled(undefined, undefined, chatId);

		expect(result).toBeUndefined();
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});
		invalidateSpy.mockRestore();
	});
});

describe("unarchiveChat optimistic update", () => {
	it("optimistically sets archived to false in the chats list", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(false);
	});

	it("optimistically sets archived to false in the individual chat cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.archived,
		).toBe(false);
	});

	it("removes an unarchived root chat from archived filtered lists after success", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(
			queryClient,
			[
				makeChat(chatId, { archived: true }),
				makeChat("chat-2", { archived: true }),
			],
			{ archived: true },
		);
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		expect(
			readInfiniteChats(queryClient, { archived: true })?.map(
				(chat) => chat.id,
			),
		).toEqual(["chat-2"]);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey),
		).toMatchObject({
			archived: false,
		});
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { archived: true }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unarchiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(false);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.archived,
		).toBe(false);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// The chats list is rolled back via invalidation.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		// The individual chat cache is restored directly.
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.archived,
		).toBe(true);
	});

	it("invalidates on settled without returning pending promises", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		// Mock invalidateQueries to never resolve so a regression back to
		// an awaited (async) onSettled surfaces as a pending promise return
		// value, which is what keeps the mutation's loading state stuck.
		const invalidateSpy = vi
			.spyOn(queryClient, "invalidateQueries")
			.mockReturnValue(new Promise<void>(() => {}));

		const mutation = unarchiveChat(queryClient);
		const result = mutation.onSettled(undefined, undefined, chatId);

		expect(result).toBeUndefined();
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});
		invalidateSpy.mockRestore();
	});
});

describe("pinChat optimistic update", () => {
	it("uses a deterministic temporary pin order until REST repair", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-new";
		seedInfiniteChats(queryClient, [
			makeChat("chat-pinned-1", { pin_order: 1 }),
			makeChat(chatId),
			makeChat("chat-pinned-2", { pin_order: 2 }),
		]);
		queryClient.setQueryData(infiniteChats({ archived: true }).queryKey, {
			pages: [[makeChat("chat-pinned-archived", { pin_order: 4 })]],
			pageParams: [0],
		});
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));

		const mutation = pinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)
				?.pin_order,
		).toBe(1);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.pin_order,
		).toBe(1);
	});
});

describe("unpinChat optimistic update", () => {
	it("optimistically sets pin_order to 0 in the chats list", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 2 })]);

		const mutation = unpinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(readInfiniteChats(queryClient)?.[0].pin_order).toBe(0);
	});

	it("optimistically sets pin_order to 0 in the individual chat cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 2 })]);
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { pin_order: 2 }),
		);

		const mutation = unpinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.pin_order,
		).toBe(0);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })]);
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { pin_order: 3 }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].pin_order).toBe(0);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.pin_order,
		).toBe(0);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// Both denormalized collections and exact detail are restored directly.
		expect(invalidateSpy).not.toHaveBeenCalled();
		// The individual chat cache is restored directly.
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey)?.pin_order,
		).toBe(3);
	});

	it("invalidates queries on settled", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});
	});
});

describe("metadata mutation rollback execution ownership", () => {
	const streamError: TypesGen.ChatError = {
		message: "new stream error",
		retryable: false,
	};
	const cases = [
		{
			name: "archive",
			initial: { archived: false, pin_order: 3 },
			expected: { archived: false, pin_order: 3 },
			mutate: async (queryClient: QueryClient, chatId: string) => {
				const mutation = archiveChat(queryClient);
				const context = await mutation.onMutate(chatId);
				return () => mutation.onError(new Error("failed"), chatId, context);
			},
		},
		{
			name: "unarchive",
			initial: { archived: true, pin_order: 0 },
			expected: { archived: true, pin_order: 0 },
			mutate: async (queryClient: QueryClient, chatId: string) => {
				const mutation = unarchiveChat(queryClient);
				const context = await mutation.onMutate(chatId);
				return () => mutation.onError(new Error("failed"), chatId, context);
			},
		},
		{
			name: "plan mode",
			initial: { plan_mode: undefined },
			expected: { plan_mode: undefined },
			mutate: async (queryClient: QueryClient, chatId: string) => {
				const mutation = updateChatPlanMode(queryClient);
				const variables = { chatId, planMode: "plan" as const };
				const context = await mutation.onMutate(variables);
				return () => mutation.onError(new Error("failed"), variables, context);
			},
		},
		{
			name: "workspace",
			initial: { workspace_id: "workspace-old" },
			expected: { workspace_id: "workspace-old" },
			mutate: async (queryClient: QueryClient, chatId: string) => {
				const mutation = updateChatWorkspace(queryClient);
				const variables = { chatId, workspaceId: "workspace-new" };
				const context = await mutation.onMutate(variables);
				return () => mutation.onError(new Error("failed"), variables, context);
			},
		},
		{
			name: "pin",
			initial: { pin_order: 0 },
			expected: { pin_order: 0 },
			mutate: async (queryClient: QueryClient, chatId: string) => {
				const mutation = pinChat(queryClient);
				const context = await mutation.onMutate(chatId);
				return () => mutation.onError(new Error("failed"), chatId, context);
			},
		},
		{
			name: "unpin",
			initial: { pin_order: 3 },
			expected: { pin_order: 3 },
			mutate: async (queryClient: QueryClient, chatId: string) => {
				const mutation = unpinChat(queryClient);
				const context = await mutation.onMutate(chatId);
				return () => mutation.onError(new Error("failed"), chatId, context);
			},
		},
	] as const;

	for (const testCase of cases) {
		it(`${testCase.name} preserves a newer stream execution snapshot on error`, async () => {
			const queryClient = createTestQueryClient();
			const chatId = `chat-${testCase.name.replace(" ", "-")}`;
			const initialChat = makeChat(chatId, testCase.initial);
			queryClient.setQueryData(chat(chatId).queryKey, initialChat);
			seedInfiniteChats(queryClient, [initialChat]);

			const rollback = await testCase.mutate(queryClient, chatId);
			updateCachedChatExecutionSnapshot(queryClient, chatId, {
				type: "error",
				chat_id: chatId,
				error: streamError,
			});
			rollback();

			expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
				...testCase.expected,
				status: "error",
				last_error: streamError,
			});
		});
	}
});

describe("reorderPinnedChat", () => {
	it("updates a single chat via updateChat and invalidates list and detail queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		vi.mocked(API.experimental.updateChat).mockResolvedValue(undefined);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		const mutation = reorderPinnedChat(queryClient);
		await mutation.onMutate?.({ chatId, pinOrder: 2 });
		await mutation.mutationFn({ chatId, pinOrder: 2 });
		await mutation.onSettled?.(undefined, undefined, { chatId, pinOrder: 2 });

		expect(cancelSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});
		expect(API.experimental.updateChat).toHaveBeenCalledWith(chatId, {
			pin_order: 2,
		});
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatQueryKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});
	});
});

describe("chat cost query factories", () => {
	it("builds the summary query key and forwards snake_case params", async () => {
		const user = "user-1";
		const params = {
			start_date: "2025-01-01",
			end_date: "2025-01-31",
		};
		vi.mocked(API.experimental.getChatCostSummary).mockResolvedValue(
			{} as TypesGen.ChatCostSummary,
		);

		const query = chatCostSummary(user, params);

		expect(chatCostSummaryKey(user, params)).toEqual([
			"chats",
			"costSummary",
			user,
			params,
		]);
		expect(query.queryKey).toEqual(["chats", "costSummary", user, params]);
		await query.queryFn();
		expect(API.experimental.getChatCostSummary).toHaveBeenCalledWith(
			user,
			params,
		);
	});

	it("builds paginated cost users query with correct key and coerces empty username", async () => {
		const payload = {
			start_date: "2025-01-01",
			end_date: "2025-01-31",
			username: "",
		};
		vi.mocked(API.experimental.getChatCostUsers).mockResolvedValue(
			{} as TypesGen.ChatCostUsersResponse,
		);
		const result = paginatedChatCostUsers(payload);

		// queryPayload returns the original payload.
		const pageParams = {
			pageNumber: 2,
			limit: 25,
			offset: 25,
			searchParams: new URLSearchParams(),
		};
		expect(result.queryPayload(pageParams)).toEqual(payload);

		// queryKey includes the payload and page number.
		const key = result.queryKey({ ...pageParams, payload });
		expect(key).toEqual(["chats", "costUsers", payload, 2]);

		// queryFn coerces empty username to undefined.
		// Cast needed because PaginatedQueryFnContext includes
		// react-query internal fields that aren't relevant here.
		await (
			result.queryFn as (params: Record<string, unknown>) => Promise<unknown>
		)({
			...pageParams,
			payload,
		});
		expect(API.experimental.getChatCostUsers).toHaveBeenCalledWith(
			expect.objectContaining({ username: undefined, limit: 25, offset: 25 }),
		);
	});
});

describe("mutation invalidation scope", () => {
	// These tests assert the CORRECT (narrow) invalidation behaviour.
	// Each mutation should only invalidate the queries it actually
	// needs to refresh, not the entire ["chats"] prefix tree. The
	// WebSocket stream already delivers real-time updates for
	// messages, status changes, and sidebar ordering, so broad
	// prefix invalidation causes a burst of redundant HTTP requests
	// on the /agents page.

	/** Populate the QueryClient with every query key that is actively
	 *  observed on the /agents/:id detail page. */
	const seedAllActiveQueries = (queryClient: QueryClient, chatId: string) => {
		// Infinite sidebar list: ["chats", "list", { archived: false }]
		queryClient.setQueryData(infiniteChats({ archived: false }).queryKey, {
			pages: [[makeChat(chatId)]],
			pageParams: [undefined],
		});
		// Individual chat: ["chats", "detail", chatId]
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));
		// Messages: ["chats", chatId, "messages"]
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatId).queryKey, {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		// Debug runs: ["chats", chatId, "debug-runs"]
		queryClient.setQueryData(chatQueryKeys.debugRuns(chatId), []);
		// Diff contents: ["chats", chatId, "diff-contents"]
		queryClient.setQueryData(chatQueryKeys.diffContents(chatId), { files: [] });
		// Cost summary: ["chats", "costSummary", "me", undefined]
		queryClient.setQueryData(
			chatCostSummaryKey("me", undefined),
			{} as TypesGen.ChatCostSummary,
		);
	};

	/** Keys that should NEVER be invalidated by chat message mutations
	 *  because they are completely unrelated to the message flow. */
	const unrelatedKeys = (chatId: string) => [
		{ label: "diff-contents", key: chatQueryKeys.diffContents(chatId) },
		{ label: "cost-summary", key: chatCostSummaryKey("me", undefined) },
	];

	it("createChatMessage does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChatMessage(queryClient, chatId);
		await mutation.onSettled(undefined, undefined, { content: [] });

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by createChatMessage`,
			).not.toBe(true);
		}
	});

	it("createChatMessage reconciles detail, messages, prompts, and debug runs", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChatMessage(queryClient, chatId);
		await mutation.onSettled(undefined, undefined, { content: [] });

		expect(
			queryClient.getQueryState(chatQueryKeys.debugRuns(chatId))?.isInvalidated,
			"chatDebugRunsKey should be invalidated",
		).toBe(true);

		const chatState = queryClient.getQueryState(chat(chatId).queryKey);
		expect(
			chatState?.isInvalidated,
			"exact chat detail should be invalidated",
		).toBe(true);

		const messagesState = queryClient.getQueryState(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(
			messagesState?.isInvalidated,
			"chat messages query should be invalidated",
		).toBe(true);
	});

	it("createChatMessage applies returned queued and committed row hints", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const queuedMessage: TypesGen.ChatQueuedMessage = {
			id: 10,
			chat_id: chatId,
			created_at: "2025-01-01T00:01:00Z",
			content: [{ type: "text", text: "queued" }],
		};
		const committedMessage = makeMsg(chatId, 11);
		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages: [], queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);
		const mutation = createChatMessage(queryClient, chatId);
		const context = await mutation.onMutate();

		mutation.onSuccess(
			{ queued: true, queued_message: queuedMessage },
			{ content: [] },
			context,
		);
		mutation.onSuccess(
			{ queued: false, message: committedMessage },
			{ content: [] },
			context,
		);

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.queued_messages).toEqual([queuedMessage]);
		expect(data?.pages[0]?.messages).toEqual([committedMessage]);
	});

	it("createChatMessage updates exact execution only for a current non-queued response", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-create-execution";
		const actionRequired: TypesGen.ChatStreamActionRequired = {
			tool_calls: [
				{ tool_call_id: "call-1", tool_name: "dynamic_tool", args: "{}" },
			],
		};
		queryClient.setQueryData(chat(chatId).queryKey, {
			...makeChat(chatId, {
				status: "error",
				last_error: { message: "old", retryable: false },
			}),
			action_required: actionRequired,
		});
		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages: [], queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);
		const mutation = createChatMessage(queryClient, chatId);
		const queuedContext = await mutation.onMutate();
		mutation.onSuccess({ queued: true }, { content: [] }, queuedContext);
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "error",
			last_error: { message: "old" },
			action_required: actionRequired,
		});

		const acceptedContext = await mutation.onMutate();
		mutation.onSuccess({ queued: false }, { content: [] }, acceptedContext);
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "running",
		});
		expect(
			queryClient.getQueryData<ChatDetailProjection>(chat(chatId).queryKey)
				?.last_error,
		).toBeUndefined();
		expect(
			queryClient.getQueryData<ChatDetailProjection>(chat(chatId).queryKey)
				?.action_required,
		).toBeUndefined();
	});

	it("createChatMessage does not overwrite a newer stream execution snapshot", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-create-stream-wins";
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));
		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages: [], queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);
		const mutation = createChatMessage(queryClient, chatId);
		const context = await mutation.onMutate();
		const error: TypesGen.ChatError = {
			message: "stream failed",
			retryable: false,
		};
		updateCachedChatExecutionSnapshot(queryClient, chatId, {
			type: "error",
			chat_id: chatId,
			error,
		});
		mutation.onSuccess({ queued: false }, { content: [] }, context);
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "error",
			last_error: error,
		});
	});

	it("editChatMessage does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = editChatMessage(queryClient, chatId);
		mutation.onSettled();

		await new Promise((r) => setTimeout(r, 0));

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by editChatMessage`,
			).not.toBe(true);
		}
	});

	it("editChatMessage reconciles chat detail, messages, and debug runs", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = editChatMessage(queryClient, chatId);
		mutation.onSettled();

		await new Promise((r) => setTimeout(r, 0));

		// Chat metadata and debug runs should be invalidated because
		// editing changes the chat's updated_at and can start a new
		// debug run.
		const chatState = queryClient.getQueryState(chat(chatId).queryKey);
		expect(
			chatState?.isInvalidated,
			"exact chat detail should be invalidated",
		).toBe(true);

		const messagesState = queryClient.getQueryState(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(
			messagesState?.isInvalidated,
			"chat messages query should be invalidated",
		).toBe(true);

		expect(
			queryClient.getQueryState(chatQueryKeys.debugRuns(chatId))?.isInvalidated,
			"chatDebugRunsKey should be invalidated",
		).toBe(true);
	});

	it("editChatMessage onError invalidates messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		mutation.onError(
			new Error("fail"),
			{ messageId: 2, req: editReq },
			{
				previousData: {
					pages: [{ messages, queued_messages: [], has_more: false }],
					pageParams: [undefined],
				},
			},
		);
		await mutation.onSettled();

		const messagesState = queryClient.getQueryState(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(
			messagesState?.isInvalidated,
			"chat messages query should be invalidated on error",
		).toBe(true);
	});

	// Shared type for the infinite messages cache shape used by
	// editChatMessage tests below.
	type InfMessages = {
		pages: TypesGen.ChatMessagesResponse[];
		pageParams: (number | undefined)[];
	};

	const makeMsg = (chatId: string, id: number): TypesGen.ChatMessage => ({
		id,
		chat_id: chatId,
		created_at: `2025-01-01T00:00:${String(id).padStart(2, "0")}Z`,
		role: "user" as const,
		content: [{ type: "text" as const, text: `msg ${id}` }],
	});

	const makeQueuedMessage = (
		chatId: string,
		id: number,
	): TypesGen.ChatQueuedMessage => ({
		id,
		chat_id: chatId,
		created_at: `2025-01-01T00:10:${String(id).padStart(2, "0")}Z`,
		content: [{ type: "text" as const, text: `queued ${id}` }],
	});

	const editReq = {
		content: [{ type: "text" as const, text: "edited" }],
	};

	const requireMessage = (
		messages: readonly TypesGen.ChatMessage[],
		messageId: number,
	): TypesGen.ChatMessage => {
		const message = messages.find((candidate) => candidate.id === messageId);
		if (!message) {
			throw new Error(`missing message ${messageId}`);
		}
		return message;
	};

	const buildOptimisticMessage = (message: TypesGen.ChatMessage) =>
		buildOptimisticEditedMessage({
			originalMessage: message,
			requestContent: editReq.content,
		});

	it("editChatMessage writes the optimistic replacement into cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 3),
		);

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			3, 2, 1,
		]);
		expect(data?.pages[0]?.messages[0]?.content).toEqual(
			optimisticMessage.content,
		);
		expect(context?.previousData?.pages[0]?.messages).toHaveLength(5);
	});

	it("editChatMessage clears queued messages in cache during optimistic history edit", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 3),
		);
		const queuedMessages = [makeQueuedMessage(chatId, 11)];

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [
					{
						messages,
						queued_messages: queuedMessages,
						has_more: false,
					},
				],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.queued_messages).toEqual([]);
	});

	it("editChatMessage restores cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 3),
		);

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		expect(
			queryClient.getQueryData<InfMessages>(
				chatMessagesForInfiniteScroll(chatId).queryKey,
			)?.pages[0]?.messages,
		).toHaveLength(3);

		mutation.onError(
			new Error("network failure"),
			{ messageId: 3, optimisticMessage, req: editReq },
			context,
		);

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			5, 4, 3, 2, 1,
		]);
	});

	it("editChatMessage preserves websocket-upserted newer messages on success", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 3),
		);
		const responseMessage = {
			...makeMsg(chatId, 9),
			content: [{ type: "text" as const, text: "edited authoritative" }],
		};
		const websocketMessage = {
			...makeMsg(chatId, 10),
			content: [{ type: "text" as const, text: "assistant follow-up" }],
			role: "assistant" as const,
		};

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});
		queryClient.setQueryData<InfMessages | undefined>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			(current) => {
				if (!current) {
					return current;
				}
				return {
					...current,
					pages: [
						{
							...current.pages[0],
							messages: [websocketMessage, ...current.pages[0].messages],
						},
						...current.pages.slice(1),
					],
				};
			},
		);
		mutation.onSuccess(
			{ message: responseMessage },
			{ messageId: 3, optimisticMessage, req: editReq },
		);

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			10, 9, 2, 1,
		]);
		expect(data?.pages[0]?.messages[1]?.content).toEqual(
			responseMessage.content,
		);
	});

	it("editChatMessage error does not roll back over a concurrent stream update", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 3),
		);
		const websocketMessage = {
			...makeMsg(chatId, 10),
			role: "assistant" as const,
			content: [{ type: "text" as const, text: "assistant follow-up" }],
		};
		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		queryClient.setQueryData(chat(chatId).queryKey, {
			...makeChat(chatId, {
				status: "error",
				last_error: { message: "old error", retryable: false },
			}),
			action_required: {
				tool_calls: [
					{ tool_call_id: "call-1", tool_name: "dynamic_tool", args: "{}" },
				],
			},
		});
		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "running",
		});
		expect(
			queryClient.getQueryData<ChatDetailProjection>(chat(chatId).queryKey)
				?.last_error,
		).toBeUndefined();
		const streamError: TypesGen.ChatError = {
			message: "new stream error",
			retryable: false,
		};
		updateCachedChatExecutionSnapshot(queryClient, chatId, {
			type: "error",
			chat_id: chatId,
			error: streamError,
		});
		upsertCachedChatMessages(queryClient, chatId, [websocketMessage]);
		mutation.onError(
			new Error("network failure"),
			{ messageId: 3, optimisticMessage, req: editReq },
			context,
		);
		await mutation.onSettled();

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toContain(10);
		expect(
			queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
				?.isInvalidated,
		).toBe(true);
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "error",
			last_error: streamError,
		});
	});

	it("editChatMessage onMutate is a no-op when cache is empty", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			req: editReq,
		});

		expect(context.previousData).toBeUndefined();
		expect(
			queryClient.getQueryData(chatMessagesForInfiniteScroll(chatId).queryKey),
		).toBeUndefined();
	});

	it("editChatMessage onError handles undefined context gracefully", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);

		// Pass undefined context. This simulates onMutate throwing before
		// it could return a snapshot.
		mutation.onError(
			new Error("fail"),
			{ messageId: 2, req: editReq },
			undefined,
		);
		await mutation.onSettled();

		// Cache should be untouched: no crash, no corruption.
		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([3, 2, 1]);

		await new Promise((r) => setTimeout(r, 0));
		const messagesState = queryClient.getQueryState(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(
			messagesState?.isInvalidated,
			"chat messages query should be invalidated even without context",
		).toBe(true);
	});

	it("editChatMessage onMutate updates the first page and preserves older pages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Page 0 (newest): IDs 10 to 6. Page 1 (older): IDs 5 to 1.
		const page0 = [10, 9, 8, 7, 6].map((id) => makeMsg(chatId, id));
		const page1 = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(requireMessage(page0, 7));

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [
					{ messages: page0, queued_messages: [], has_more: true },
					{ messages: page1, queued_messages: [], has_more: false },
				],
				pageParams: [undefined, 6],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 7,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			7, 6,
		]);
		expect(data?.pages[1]?.messages.map((message) => message.id)).toEqual([
			5, 4, 3, 2, 1,
		]);
	});

	it("editChatMessage onMutate keeps the optimistic replacement when editing the first message", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 1),
		);

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 1,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([1]);
		expect(data?.pages[0]?.queued_messages).toEqual([]);
		expect(data?.pages[0]?.has_more).toBe(false);
	});

	it("editChatMessage onMutate keeps earlier messages when editing the latest message", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 5),
		);

		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages, queued_messages: [], has_more: false }],
				pageParams: [undefined],
			},
		);

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 5,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			5, 4, 3, 2, 1,
		]);
		expect(data?.pages[0]?.messages[0]?.content).toEqual(
			optimisticMessage.content,
		);
	});

	it("delete queue optimism preserves newer stream snapshots on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const first: TypesGen.ChatQueuedMessage = {
			id: 10,
			chat_id: chatId,
			created_at: "2025-01-01T00:01:00Z",
			content: [{ type: "text", text: "first" }],
		};
		const second = {
			...first,
			id: 11,
			content: [{ type: "text" as const, text: "second" }],
		};
		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [{ messages: [], queued_messages: [first], has_more: false }],
				pageParams: [undefined],
			},
		);
		const mutation = deleteChatQueuedMessage(queryClient, chatId);
		const context = await mutation.onMutate(first.id);
		replaceCachedChatQueuedMessages(queryClient, chatId, [second]);

		mutation.onError(new Error("failed"), first.id, context);

		const data = queryClient.getQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
		);
		expect(data?.pages[0]?.queued_messages).toEqual([second]);
	});

	it("promote queue optimism removes the row and rolls back while owned", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const queuedMessage: TypesGen.ChatQueuedMessage = {
			id: 10,
			chat_id: chatId,
			created_at: "2025-01-01T00:01:00Z",
			content: [{ type: "text", text: "queued" }],
		};
		queryClient.setQueryData<InfMessages>(
			chatMessagesForInfiniteScroll(chatId).queryKey,
			{
				pages: [
					{ messages: [], queued_messages: [queuedMessage], has_more: false },
				],
				pageParams: [undefined],
			},
		);
		queryClient.setQueryData(chat(chatId).queryKey, {
			...makeChat(chatId, {
				status: "error",
				last_error: { message: "old error", retryable: false },
			}),
			action_required: {
				tool_calls: [
					{ tool_call_id: "call-1", tool_name: "dynamic_tool", args: "{}" },
				],
			},
		});
		const mutation = promoteChatQueuedMessage(queryClient, chatId);
		const context = await mutation.onMutate(queuedMessage.id);
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "running",
		});
		expect(
			queryClient.getQueryData<ChatDetailProjection>(chat(chatId).queryKey)
				?.last_error,
		).toBeUndefined();
		expect(
			queryClient.getQueryData<InfMessages>(
				chatMessagesForInfiniteScroll(chatId).queryKey,
			)?.pages[0]?.queued_messages,
		).toEqual([]);

		mutation.onError(new Error("failed"), queuedMessage.id, context);

		expect(
			queryClient.getQueryData<InfMessages>(
				chatMessagesForInfiniteScroll(chatId).queryKey,
			)?.pages[0]?.queued_messages,
		).toEqual([queuedMessage]);
		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "error",
			last_error: { message: "old error" },
			action_required: { tool_calls: [{ tool_call_id: "call-1" }] },
		});
	});

	it("interruptChat response does not overwrite a newer stream execution snapshot", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-interrupt-race";
		queryClient.setQueryData(
			chat(chatId).queryKey,
			makeChat(chatId, { status: "running" }),
		);
		const mutation = interruptChat(queryClient, chatId);
		const context = await mutation.onMutate();
		const streamError: TypesGen.ChatError = {
			message: "new stream error",
			retryable: false,
		};
		updateCachedChatExecutionSnapshot(queryClient, chatId, {
			type: "error",
			chat_id: chatId,
			error: streamError,
		});

		mutation.onSuccess(
			makeChat(chatId, { status: "waiting", last_error: undefined }),
			undefined,
			context,
		);

		expect(queryClient.getQueryData(chat(chatId).queryKey)).toMatchObject({
			status: "error",
			last_error: streamError,
		});
	});

	it("interruptChat invalidates debug runs without touching unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = interruptChat(queryClient, chatId);
		await mutation.onSettled();

		expect(
			queryClient.getQueryState(chatQueryKeys.debugRuns(chatId))?.isInvalidated,
			"chatDebugRunsKey should be invalidated",
		).toBe(true);

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by interruptChat`,
			).not.toBe(true);
		}
	});

	it("promoteChatQueuedMessage invalidates debug runs without touching unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = promoteChatQueuedMessage(queryClient, chatId);
		await mutation.onSettled();

		expect(
			queryClient.getQueryState(chatQueryKeys.debugRuns(chatId))?.isInvalidated,
			"chatDebugRunsKey should be invalidated",
		).toBe(true);

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by promoteChatQueuedMessage`,
			).not.toBe(true);
		}
	});

	for (const { label, error } of [
		{ label: "success", error: undefined },
		{ label: "failure", error: new Error("proposal failed") },
	]) {
		it(`proposeChatTitle invalidates debug runs on ${label} without touching unrelated queries`, async () => {
			const queryClient = createTestQueryClient();
			const chatId = "chat-1";
			seedAllActiveQueries(queryClient, chatId);

			const mutation = proposeChatTitle(queryClient);
			await mutation.onSettled(undefined, error, chatId);

			expect(
				queryClient.getQueryState(chatQueryKeys.debugRuns(chatId))
					?.isInvalidated,
				"chatDebugRunsKey should be invalidated",
			).toBe(true);

			for (const { label, key } of [
				{
					label: "infinite chats",
					key: infiniteChats({ archived: false }).queryKey,
				},
				{ label: "chat detail", key: chat(chatId).queryKey },
				{
					label: "messages",
					key: chatMessagesForInfiniteScroll(chatId).queryKey,
				},
				...unrelatedKeys(chatId),
			]) {
				const state = queryClient.getQueryState(key);
				expect(
					state?.isInvalidated,
					`${label} should NOT be invalidated by proposeChatTitle`,
				).not.toBe(true);
			}
		});
	}

	it("createChat repairs collections and exact created-chat metadata", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChat(queryClient);
		await mutation.onSettled(makeChat(chatId));

		await new Promise((r) => setTimeout(r, 0));

		// Sidebar lists SHOULD be invalidated.
		expect(
			queryClient.getQueryState(infiniteChats({ archived: false }).queryKey)
				?.isInvalidated,
			"infinite chats should be invalidated",
		).toBe(true);

		// Auxiliary per-chat queries should NOT be touched.
		for (const { label, key } of unrelatedKeys(chatId)) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} should NOT be invalidated by createChat`,
			).not.toBe(true);
		}
		expect(
			queryClient.getQueryState(chat(chatId).queryKey)?.isInvalidated,
			"exact chat detail should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
				?.isInvalidated,
			"chat messages query should NOT be invalidated",
		).not.toBe(true);
	});

	it("deleteChatQueuedMessage invalidates only chat detail and messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = deleteChatQueuedMessage(queryClient, chatId);
		await mutation.onSettled();

		// These two should be invalidated (exact match).
		expect(
			queryClient.getQueryState(chat(chatId).queryKey)?.isInvalidated,
			"exact chat detail should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
				?.isInvalidated,
			"chat messages query should be invalidated",
		).toBe(true);

		// Unrelated queries should NOT be touched.
		for (const { label, key } of unrelatedKeys(chatId)) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} should NOT be invalidated by deleteChatQueuedMessage`,
			).not.toBe(true);
		}

		// Sidebar list should NOT be touched.
		expect(
			queryClient.getQueryState(infiniteChats({ archived: false }).queryKey)
				?.isInvalidated,
			"infinite chats should NOT be invalidated",
		).not.toBe(true);
	});
});

describe("chat list query shape", () => {
	it("uses an explicit list namespace and normalized filters", () => {
		expect(
			infiniteChats({
				archived: true,
				prStatuses: ["open", "draft", "open"],
				sources: ["shared_with_me", "created_by_me"],
			}).queryKey,
		).toEqual([
			"chats",
			"list",
			{
				archived: true,
				chatStatus: undefined,
				prStatuses: ["draft", "open"],
				sources: ["created_by_me", "shared_with_me"],
			},
		]);
	});
});

describe("infiniteChats", () => {
	const PAGE_LIMIT = 50;
	const runQuery = async (
		options: ReturnType<typeof infiniteChats>,
		pageParam: string | undefined,
	) => options.queryFn?.({ pageParam } as never);

	describe("getNextPageParam", () => {
		it("returns undefined when lastPage has fewer items than the limit", () => {
			const { getNextPageParam } = infiniteChats();
			const lastPage = Array.from({ length: PAGE_LIMIT - 1 }, (_, i) =>
				makeChat(`chat-${i}`),
			);
			expect(
				getNextPageParam?.(lastPage, [lastPage], undefined, []),
			).toBeUndefined();
		});

		it("returns the last chat ID when the page is full", () => {
			const { getNextPageParam } = infiniteChats();
			const lastPage = Array.from({ length: PAGE_LIMIT }, (_, i) =>
				makeChat(`chat-${i}`),
			);
			expect(getNextPageParam?.(lastPage, [lastPage], undefined, [])).toBe(
				`chat-${PAGE_LIMIT - 1}`,
			);
		});
	});

	describe("queryFn", () => {
		it("omits after_id for the first page", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			await runQuery(infiniteChats(), undefined);
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				after_id: undefined,
				limit: PAGE_LIMIT,
				q: undefined,
			});
		});

		it("uses the previous page's last chat ID as the cursor", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			await runQuery(infiniteChats(), "chat-cursor");
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				after_id: "chat-cursor",
				limit: PAGE_LIMIT,
				q: undefined,
			});
		});

		it("builds q from archived, prStatuses, chatStatus, and sources", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const options = infiniteChats({
				archived: true,
				prStatuses: ["draft", "open", "merged"],
				chatStatus: "unread",
				sources: ["created_by_me", "shared_with_me"],
			});

			await runQuery(options, undefined);

			expect(API.experimental.getChats).toHaveBeenCalledWith({
				after_id: undefined,
				limit: PAGE_LIMIT,
				q: "archived:true pr_status:draft,open,merged has_unread:true source:created_by_me,shared_with_me",
			});
		});

		it("builds q for read chat status", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const options = infiniteChats({
				archived: false,
				chatStatus: "read",
			});

			await runQuery(options, undefined);

			expect(API.experimental.getChats).toHaveBeenCalledWith({
				after_id: undefined,
				limit: PAGE_LIMIT,
				q: "archived:false has_unread:false",
			});
		});
	});
});

describe("chatSearch", () => {
	it("requests chats with q and a fixed limit", async () => {
		vi.mocked(API.experimental.getChats).mockResolvedValue([]);
		const query = chatSearch("title:fix");
		const queryClient = createTestQueryClient();

		expect(query.queryKey).toEqual(["chats", "search", { q: "title:fix" }]);
		await queryClient.fetchQuery(query);
		expect(API.experimental.getChats).toHaveBeenCalledWith({
			limit: 50,
			q: "title:fix",
		});
	});
});

describe("diff_status_change invalidation scope", () => {
	// These tests verify the CORRECT invalidation pattern for
	// diff_status_change WebSocket events. The handler should
	// invalidate only the individual chat detail and diff-contents
	// queries, NOT the chat list (sidebar) or messages.

	it("exact exact chat detail invalidation does not cascade to messages or diff-contents", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Seed all the queries that are active on the /agents/:id page.
		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatId).queryKey, {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		queryClient.setQueryData(chatQueryKeys.diffContents(chatId), { files: [] });
		seedInfiniteChats(queryClient, [makeChat(chatId)]);

		// This is what the fixed handler does, exact: true.
		await queryClient.invalidateQueries({
			queryKey: chat(chatId).queryKey,
			exact: true,
		});

		// exact chat detail itself should be invalidated.
		expect(
			queryClient.getQueryState(chat(chatId).queryKey)?.isInvalidated,
			"exact chat detail should be invalidated",
		).toBe(true);

		// Messages should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
				?.isInvalidated,
			"chat messages query should NOT be invalidated by exact exact chat detail",
		).not.toBe(true);

		// Diff-contents should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatQueryKeys.diffContents(chatId))
				?.isInvalidated,
			"chatDiffContentsKey should NOT be invalidated by exact exact chat detail",
		).not.toBe(true);

		// Chat list should NOT be invalidated.
		expect(
			queryClient.getQueryState(infiniteChats().queryKey)?.isInvalidated,
			"chat list query should NOT be invalidated by exact exact chat detail",
		).not.toBe(true);
	});

	it("without exact: true, exact chat detail invalidation cascades to messages and diff-contents (the old bug)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(chat(chatId).queryKey, makeChat(chatId));
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatId).queryKey, {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		queryClient.setQueryData(chatQueryKeys.diffContents(chatId), { files: [] });

		// This is what the OLD (broken) handler did, no exact: true.
		await queryClient.invalidateQueries({
			queryKey: chat(chatId).queryKey,
		});

		// Without exact: true, ALL queries starting with ["chats", chatId]
		// get invalidated, including messages and diff-contents.
		expect(
			queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
				?.isInvalidated,
			"chat messages query IS invalidated without exact: true (old bug)",
		).toBe(true);

		expect(
			queryClient.getQueryState(chatQueryKeys.diffContents(chatId))
				?.isInvalidated,
			"chatDiffContentsKey IS invalidated without exact: true (old bug)",
		).toBe(true);
	});
});

describe("sidebar title race condition", () => {
	const readTitle = (
		queryClient: QueryClient,
		chatId: string,
	): string | undefined => {
		const data = queryClient.getQueryData<InfiniteData>(infiniteChatsTestKey);
		return data?.pages.flat().find((c) => c.id === chatId)?.title;
	};

	it("in-flight refetch overwrites a WebSocket title update (the bug)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		seedInfiniteChats(queryClient, [
			makeChat(chatId, { title: "fallback title" }),
		]);

		// Simulate invalidateChatListQueries triggering a refetch that
		// returns stale data (the server hadn't generated the title yet
		// when it processed this request).
		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "fallback title" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		// Simulate the title_change WebSocket event arriving while the
		// refetch is in flight. This mirrors what AgentsPage does.
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((c) =>
				c.id === chatId ? { ...c, title: "generated title" } : c,
			),
		);

		// The cache shows the generated title immediately.
		expect(readTitle(queryClient, chatId)).toBe("generated title");

		// After the refetch settles, it overwrites with stale data.
		await fetchDone;
		expect(readTitle(queryClient, chatId)).toBe("fallback title");
	});

	it("cancelChatListRefetches before the update prevents the overwrite (the fix)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		seedInfiniteChats(queryClient, [
			makeChat(chatId, { title: "fallback title" }),
		]);

		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "fallback title" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		// Cancel, then write. Matches the new WebSocket handler code.
		await cancelChatListRefetches(queryClient);

		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((c) =>
				c.id === chatId ? { ...c, title: "generated title" } : c,
			),
		);

		expect(readTitle(queryClient, chatId)).toBe("generated title");

		await fetchDone;
		expect(readTitle(queryClient, chatId)).toBe("generated title");
	});
});

describe("cancelChatListRefetches", () => {
	it("cancels a regular refetch", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		seedInfiniteChats(queryClient, [makeChat(chatId, { title: "original" })]);

		// Start an in-flight refetch (no fetchMeta, simulates a
		// regular invalidation or window-focus refetch).
		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "stale" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		await cancelChatListRefetches(queryClient);
		await fetchDone;

		// The refetch was cancelled and reverted, so the original
		// data is preserved.
		const title = readInfiniteChats(queryClient)?.find(
			(c) => c.id === chatId,
		)?.title;
		expect(title).toBe("original");
	});

	it("does not cancel a fetchNextPage fetch", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		seedInfiniteChats(queryClient, [makeChat(chatId, { title: "original" })]);

		// Start an in-flight fetch.
		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "page-2-data" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		// Simulate fetchNextPage via the public setState API.
		// In react-query v5, fetchNextPage dispatches a fetch
		// action with meta: { fetchMore: { direction: "forward" } }
		// which is stored in query.state.fetchMeta.
		const query = queryClient
			.getQueryCache()
			.find({ queryKey: infiniteChatsTestKey });
		expect(query).toBeDefined();
		query!.setState({ fetchMeta: { fetchMore: { direction: "forward" } } });

		await cancelChatListRefetches(queryClient);
		await fetchDone;

		// The fetch was NOT cancelled, the new data landed.
		const title = readInfiniteChats(queryClient)?.find(
			(c) => c.id === chatId,
		)?.title;
		expect(title).toBe("page-2-data");
	});

	it("does not cancel a fetchPreviousPage fetch", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		seedInfiniteChats(queryClient, [makeChat(chatId, { title: "original" })]);

		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "prev-page" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		const query = queryClient
			.getQueryCache()
			.find({ queryKey: infiniteChatsTestKey });
		expect(query).toBeDefined();
		query!.setState({ fetchMeta: { fetchMore: { direction: "backward" } } });

		await cancelChatListRefetches(queryClient);
		await fetchDone;

		const title = readInfiniteChats(queryClient)?.find(
			(c) => c.id === chatId,
		)?.title;
		expect(title).toBe("prev-page");
	});

	it("does not cancel the initial load when no data is cached yet", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Do NOT seed the cache, simulate the very first fetch
		// where no data exists yet.
		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "first-load" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		// A WebSocket event arrives while the initial fetch is
		// in-flight. Without the data guard, this would cancel
		// the fetch and leave the query stuck in pending/idle.
		await cancelChatListRefetches(queryClient);
		await fetchDone;

		const title = readInfiniteChats(queryClient)?.find(
			(c) => c.id === chatId,
		)?.title;
		expect(title).toBe("first-load");
	});
});

describe("mutation onMutate cancels pagination fetches", () => {
	it("archiveChat onMutate cancels a pagination fetch to protect optimistic updates", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: false })]);

		// Start a fetch and mark it as a fetchNextPage via
		// fetchMeta so we can verify the broad predicate in
		// mutation onMutate still cancels it (unlike the
		// narrow cancelChatListRefetches used by the WS
		// handler).
		const fetchDone = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { archived: false })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});

		const query = queryClient
			.getQueryCache()
			.find({ queryKey: infiniteChatsTestKey });
		expect(query).toBeDefined();
		query!.setState({ fetchMeta: { fetchMore: { direction: "forward" } } });

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);
		await fetchDone;

		// The optimistic archive survives because onMutate
		// cancelled the pagination fetch before it could
		// overwrite the cache with stale oldPages.
		const chat = readInfiniteChats(queryClient)?.find((c) => c.id === chatId);
		expect(chat?.archived).toBe(true);
	});
});

describe("addChildToParentInCache", () => {
	it("prepends new child to the parent's children array", () => {
		const queryClient = createTestQueryClient();
		const parent = makeChat("parent-1");
		seedInfiniteChats(queryClient, [parent]);

		const child = makeChat("child-1", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
		});
		addChildToParentInCache(queryClient, child, "parent-1");

		const result = readInfiniteChats(queryClient);
		expect(result).toHaveLength(1);
		expect(result?.[0].children).toHaveLength(1);
		expect(result?.[0].children?.[0].id).toBe("child-1");
	});

	it("silently drops the child when the parent is not in any page", () => {
		const queryClient = createTestQueryClient();
		const other = makeChat("other-root");
		seedInfiniteChats(queryClient, [other]);

		const child = makeChat("orphan-child", {
			parent_chat_id: "missing-parent",
			root_chat_id: "missing-parent",
		});
		addChildToParentInCache(queryClient, child, "missing-parent");

		const result = readInfiniteChats(queryClient);
		expect(result).toHaveLength(1);
		expect(result?.[0].id).toBe("other-root");
		expect(result?.[0].children).toHaveLength(0);
	});

	it("does not duplicate a child that already exists under the parent", () => {
		const queryClient = createTestQueryClient();
		const existingChild = makeChat("child-1", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
		});
		const parent = makeChat("parent-1", { children: [existingChild] });
		seedInfiniteChats(queryClient, [parent]);

		addChildToParentInCache(queryClient, existingChild, "parent-1");

		const result = readInfiniteChats(queryClient);
		expect(result?.[0].children).toHaveLength(1);
	});
});

describe("updateChildInParentCache", () => {
	it("applies the updater to a child nested under its parent", () => {
		const queryClient = createTestQueryClient();
		const child = makeChat("child-1", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
			title: "Original title",
		});
		const parent = makeChat("parent-1", { children: [child] });
		seedInfiniteChats(queryClient, [parent]);

		const found = updateChildInParentCache(
			queryClient,
			(c) => ({ ...c, title: "Updated title" }),
			"child-1",
		);
		expect(found).toBe(true);

		const result = readInfiniteChats(queryClient);
		expect(result?.[0].children?.[0].title).toBe("Updated title");
	});

	it("returns false when the child is not present under any parent", () => {
		const queryClient = createTestQueryClient();
		const parent = makeChat("parent-1");
		seedInfiniteChats(queryClient, [parent]);

		const found = updateChildInParentCache(
			queryClient,
			(c) => ({ ...c, title: "Never applied" }),
			"missing-child",
		);
		expect(found).toBe(false);
	});

	it("preserves the same reference when the updater returns the child unchanged", () => {
		const queryClient = createTestQueryClient();
		const child = makeChat("child-1", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
		});
		const parent = makeChat("parent-1", { children: [child] });
		seedInfiniteChats(queryClient, [parent]);

		const before = readInfiniteChats(queryClient)?.[0];
		const found = updateChildInParentCache(queryClient, (c) => c, "child-1");
		const after = readInfiniteChats(queryClient)?.[0];

		expect(found).toBe(false);
		expect(after).toBe(before);
	});
});

describe("mergeWatchedChatSummary", () => {
	it("applies context_dirty flags while preserving the pinned resource list", () => {
		const cachedChat = makeChat("chat-1", {
			updated_at: "2025-01-01T00:00:00.000Z",
			context: {
				dirty: false,
				resources: [
					{
						source: "/AGENTS.md",
						kind: "instruction_file",
						size_bytes: 10,
						status: "ok",
					},
				],
			},
		});
		const watchedChat = makeChat("chat-1", {
			// Drift is tracked outside updated_at, so an older event timestamp
			// still applies the dirty flags.
			updated_at: "2024-12-31T00:00:00.000Z",
			context: { dirty: true, dirty_since: "2025-01-02T00:00:00.000Z" },
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "context_dirty",
			}).context,
		).toEqual({
			dirty: true,
			dirty_since: "2025-01-02T00:00:00.000Z",
			// The lightweight watch payload omits resources; the merge keeps the
			// pinned list a prior single-chat GET populated.
			resources: [
				{
					source: "/AGENTS.md",
					kind: "instruction_file",
					size_bytes: 10,
					status: "ok",
				},
			],
		});
	});

	it("leaves context untouched for non-context events", () => {
		const context = { dirty: true, dirty_since: "2025-01-02T00:00:00.000Z" };
		const cachedChat = makeChat("chat-1", {
			status: "waiting",
			updated_at: "2025-01-01T00:00:00.000Z",
			context,
		});
		const watchedChat = makeChat("chat-1", {
			status: "running",
			updated_at: "2025-01-01T00:05:00.000Z",
			context: { dirty: false },
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
			}).context,
		).toBe(context);
	});

	it("merges fresh status updates without clobbering a newer title snapshot", () => {
		const cachedChat = makeChat("chat-1", {
			status: "waiting",
			title: "Fresh title",
			last_model_config_id: "model-old",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "running",
			title: "Stale title",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
			}),
		).toMatchObject({
			status: "running",
			title: "Fresh title",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
	});

	it("merges last_model_config_id when watched updated_at equals cached updated_at", () => {
		const cachedChat = makeChat("chat-1", {
			last_model_config_id: "11111111-1111-4111-8111-111111111111",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			last_model_config_id: "22222222-2222-4222-8222-222222222222",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
			}).last_model_config_id,
		).toBe("22222222-2222-4222-8222-222222222222");
	});

	it("merges last_turn_summary when watched updated_at equals cached updated_at", () => {
		const cachedChat = makeChat("chat-1", {
			last_turn_summary: "Previous summary",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			last_turn_summary: "Updated summary",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "summary_change",
			}).last_turn_summary,
		).toBe("Updated summary");
	});

	it("applies summary_change even when event updated_at is older", () => {
		const cachedChat = makeChat("chat-1", {
			last_turn_summary: null,
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			last_turn_summary: "Fixed the issue",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "summary_change",
			}).last_turn_summary,
		).toBe("Fixed the issue");
	});

	it("clears last_turn_summary on summary updates with matching updated_at", () => {
		const cachedChat = makeChat("chat-1", {
			last_turn_summary: "Previous summary",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			last_turn_summary: null,
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "summary_change",
			}).last_turn_summary,
		).toBeNull();
	});

	it("compares updated_at values as instants instead of strings", () => {
		const cachedChat = makeChat("chat-1", {
			status: "waiting",
			last_model_config_id: "model-old",
			updated_at: "2025-01-01T00:00:00.12Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:00:00.1203Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
			}),
		).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:00:00.1203Z",
		});
	});

	it("merges fresh title updates without clobbering a newer status snapshot", () => {
		const cachedChat = makeChat("chat-1", {
			status: "running",
			title: "Fresh title",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "waiting",
			title: "Updated title",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "title_change",
			}),
		).toMatchObject({
			status: "running",
			title: "Updated title",
		});
	});

	it("merges title updates even when chat updated_at is older", () => {
		const cachedChat = makeChat("chat-1", {
			status: "running",
			title: "Fresh title",
			updated_at: "2025-01-01T00:10:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "waiting",
			title: "Newer generated title",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "title_change",
			}),
		).toMatchObject({
			status: "running",
			title: "Newer generated title",
			updated_at: "2025-01-01T00:10:00.000Z",
		});
	});

	it("merges fresh diff status updates without clobbering status or title", () => {
		const cachedDiffStatus = {
			chat_id: "chat-1",
			url: "https://example.com/pr/1",
			pull_request_state: "open",
			pull_request_title: "Old title",
			pull_request_draft: false,
			changes_requested: false,
			additions: 1,
			deletions: 2,
			changed_files: 3,
			refreshed_at: "2025-01-01T00:00:00.000Z",
			stale_at: "2025-01-01T01:00:00.000Z",
		};
		const watchedDiffStatus = {
			chat_id: "chat-1",
			url: "https://example.com/pr/2",
			pull_request_state: "merged",
			pull_request_title: "New title",
			pull_request_draft: false,
			changes_requested: true,
			additions: 4,
			deletions: 5,
			changed_files: 6,
			refreshed_at: "2025-01-01T00:05:00.000Z",
			stale_at: "2025-01-01T01:05:00.000Z",
		};
		const cachedChat = makeChat("chat-1", {
			status: "running",
			title: "Fresh title",
			diff_status: cachedDiffStatus,
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "waiting",
			title: "Stale title",
			diff_status: watchedDiffStatus,
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "diff_status_change",
			}),
		).toMatchObject({
			status: "running",
			title: "Fresh title",
			diff_status: watchedDiffStatus,
		});
	});

	it("merges diff status updates even when chat updated_at is older", () => {
		const cachedDiffStatus = {
			chat_id: "chat-1",
			url: "https://example.com/pr/1",
			pull_request_state: "open",
			pull_request_title: "Old title",
			pull_request_draft: false,
			changes_requested: false,
			additions: 1,
			deletions: 2,
			changed_files: 3,
			refreshed_at: "2025-01-01T00:00:00.000Z",
			stale_at: "2025-01-01T01:00:00.000Z",
		};
		const watchedDiffStatus = {
			chat_id: "chat-1",
			url: "https://example.com/pr/2",
			pull_request_state: "open",
			pull_request_title: "New title",
			pull_request_draft: true,
			changes_requested: true,
			additions: 4,
			deletions: 5,
			changed_files: 6,
			refreshed_at: "2025-01-01T00:10:00.000Z",
			stale_at: "2025-01-01T01:10:00.000Z",
		};
		const cachedChat = makeChat("chat-1", {
			status: "running",
			title: "Fresh title",
			diff_status: cachedDiffStatus,
			updated_at: "2025-01-01T00:10:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "waiting",
			title: "Stale title",
			diff_status: watchedDiffStatus,
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "diff_status_change",
			}),
		).toMatchObject({
			status: "running",
			title: "Fresh title",
			diff_status: watchedDiffStatus,
			updated_at: "2025-01-01T00:10:00.000Z",
		});
	});

	it("does not derive unread from fresh status updates", () => {
		const cachedChat = makeChat("chat-1", {
			has_unread: false,
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "waiting",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
				activeChatId: "chat-2",
			}).has_unread,
		).toBe(false);
	});

	it("preserves has_unread for summary changes on inactive chats", () => {
		const cachedChat = makeChat("chat-1", {
			has_unread: false,
			last_turn_summary: null,
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			last_turn_summary: "Updated summary",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "summary_change",
				activeChatId: "chat-2",
			}).has_unread,
		).toBe(false);
	});

	it("preserves has_unread for the active chat", () => {
		const cachedChat = makeChat("chat-1", {
			has_unread: false,
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			status: "waiting",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
				activeChatId: "chat-1",
			}).has_unread,
		).toBe(false);
	});
});

describe("mergeWatchedChatIntoCaches", () => {
	it("merges last_model_config_id into the root list cache without changing exact detail", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const cachedChat = makeChat(chatId, {
			status: "waiting",
			last_model_config_id: "model-old",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat(chatId, {
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		seedInfiniteChats(queryClient, [cachedChat]);
		queryClient.setQueryData(chat(chatId).queryKey, cachedChat);

		mergeWatchedChatIntoCaches(queryClient, watchedChat, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey),
		).toEqual(cachedChat);
	});

	it("merges last_model_config_id into the parent-embedded child snapshot without changing exact detail", () => {
		const queryClient = createTestQueryClient();
		const childId = "child-1";
		const cachedChild = makeChat(childId, {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
			status: "waiting",
			last_model_config_id: "model-old",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const parent = makeChat("parent-1", { children: [cachedChild] });
		const watchedChild = makeChat(childId, {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		seedInfiniteChats(queryClient, [parent]);
		queryClient.setQueryData(chat(childId).queryKey, cachedChild);

		mergeWatchedChatIntoCaches(queryClient, watchedChild, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0].children?.[0]).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(childId).queryKey),
		).toEqual(cachedChild);
	});

	it("does not let an older watch payload clobber newer cached metadata", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const cachedChat = makeChat(chatId, {
			status: "waiting",
			title: "Fresh title",
			last_model_config_id: "model-new",
			workspace_id: "workspace-new",
			build_id: "build-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		const staleWatchChat = makeChat(chatId, {
			status: "running",
			title: "Stale title",
			last_model_config_id: "model-old",
			workspace_id: "workspace-old",
			build_id: "build-old",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		seedInfiniteChats(queryClient, [cachedChat]);
		queryClient.setQueryData(chat(chatId).queryKey, cachedChat);

		mergeWatchedChatIntoCaches(queryClient, staleWatchChat, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			status: "waiting",
			title: "Fresh title",
			last_model_config_id: "model-new",
			workspace_id: "workspace-new",
			build_id: "build-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chat(chatId).queryKey),
		).toMatchObject({
			status: "waiting",
			title: "Fresh title",
			last_model_config_id: "model-new",
			workspace_id: "workspace-new",
			build_id: "build-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
	});
});

describe("chat execution projection", () => {
	it("applies status, error, and action invariants", () => {
		const initial = makeChat("chat-execution", {
			status: "error",
			last_error: { message: "old error", retryable: false },
		});
		const running = applyExecutionSnapshotEvent(initial, {
			type: "status",
			chat_id: initial.id,
			status: { status: "running" },
		});
		expect(running).toMatchObject({ status: "running" });
		expect(running.last_error).toBeUndefined();

		const actionRequired: TypesGen.ChatStreamActionRequired = {
			tool_calls: [
				{ tool_call_id: "call-1", tool_name: "dynamic_tool", args: "{}" },
			],
		};
		const pending = applyExecutionSnapshotEvent(running, {
			type: "action_required",
			chat_id: initial.id,
			action_required: actionRequired,
		});
		expect(pending).toMatchObject({
			status: "requires_action",
			action_required: actionRequired,
		});
		expect(pending.last_error).toBeUndefined();

		const error: TypesGen.ChatError = { message: "failed", retryable: false };
		const failed = applyExecutionSnapshotEvent(pending, {
			type: "error",
			chat_id: initial.id,
			error,
		});
		expect(failed).toMatchObject({ status: "error", last_error: error });
		expect(failed.action_required).toBeUndefined();
	});

	it("preserves current execution fields across an owned REST resolution", async () => {
		const queryClient = createTestQueryClient();
		const restChat = makeChat("chat-overlay", {
			status: "waiting",
			last_error: { message: "stale error", retryable: false },
			title: "Fresh metadata",
		});
		queryClient.setQueryData(chat(restChat.id).queryKey, {
			...restChat,
			status: "requires_action",
			last_error: undefined,
			action_required: {
				tool_calls: [
					{ tool_call_id: "call-1", tool_name: "dynamic_tool", args: "{}" },
				],
			},
		});
		vi.mocked(API.experimental.getChat).mockResolvedValue(restChat);
		claimChatExecutionOverlay(queryClient, restChat.id);

		const result = await queryClient.fetchQuery(chat(restChat.id));
		expect(result).toMatchObject({
			title: "Fresh metadata",
			status: "requires_action",
			action_required: { tool_calls: [{ tool_call_id: "call-1" }] },
		});
		expect(result.last_error).toBeUndefined();
	});

	it("releases ownership only for the matching token", async () => {
		const queryClient = createTestQueryClient();
		const restChat = makeChat("chat-overlay-release", { status: "waiting" });
		queryClient.setQueryData(chat(restChat.id).queryKey, {
			...restChat,
			status: "running",
		});
		vi.mocked(API.experimental.getChat).mockResolvedValue(restChat);
		const staleToken = claimChatExecutionOverlay(queryClient, restChat.id);
		const activeToken = claimChatExecutionOverlay(queryClient, restChat.id);
		releaseChatExecutionOverlay(queryClient, restChat.id, staleToken);
		await expect(
			queryClient.fetchQuery(chat(restChat.id)),
		).resolves.toMatchObject({
			status: "running",
		});
		releaseChatExecutionOverlay(queryClient, restChat.id, activeToken);
		await queryClient.invalidateQueries({
			queryKey: chat(restChat.id).queryKey,
			exact: true,
		});
		await expect(queryClient.fetchQuery(chat(restChat.id))).resolves.toEqual(
			restChat,
		);
	});
});

describe("committed message cache updates", () => {
	const makeMessage = (id: number): TypesGen.ChatMessage => ({
		id,
		chat_id: "chat-1",
		created_at: `2025-01-01T00:00:${String(id).padStart(2, "0")}Z`,
		role: id % 2 === 0 ? "assistant" : "user",
		content: [{ type: "text", text: `message ${id}` }],
	});

	it("cancels a background refetch before applying a stream upsert", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialPage: TypesGen.ChatMessagesResponse = {
			messages: [makeMessage(4), makeMessage(3)],
			queued_messages: [],
			has_more: true,
		};
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatId).queryKey, {
			pages: [initialPage],
			pageParams: [undefined],
		});
		let resolveRefetch!: (page: TypesGen.ChatMessagesResponse) => void;
		vi.mocked(API.experimental.getChatMessages).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					resolveRefetch = resolve;
				}),
		);
		const observer = new InfiniteQueryObserver(
			queryClient,
			chatMessagesForInfiniteScroll(chatId),
		);
		const unsubscribe = observer.subscribe(() => undefined);
		const refetch = observer.refetch();
		await vi.waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalledOnce();
		});

		const streamedMessage = makeMessage(5);
		upsertCachedChatMessages(queryClient, chatId, [streamedMessage]);
		resolveRefetch({
			...initialPage,
			messages: [makeMessage(4), makeMessage(3)],
		});
		await refetch;

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll(chatId).queryKey);
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			5, 4, 3,
		]);
		unsubscribe();
	});

	it("cancels fetchNextPage before applying authoritative history", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialPage: TypesGen.ChatMessagesResponse = {
			messages: [makeMessage(4), makeMessage(3)],
			queued_messages: [],
			has_more: true,
		};
		queryClient.setQueryData(chatMessagesForInfiniteScroll(chatId).queryKey, {
			pages: [initialPage],
			pageParams: [undefined],
		});
		let resolveNextPage!: (page: TypesGen.ChatMessagesResponse) => void;
		vi.mocked(API.experimental.getChatMessages).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					resolveNextPage = resolve;
				}),
		);
		const observer = new InfiniteQueryObserver(
			queryClient,
			chatMessagesForInfiniteScroll(chatId),
		);
		const unsubscribe = observer.subscribe(() => undefined);
		const fetchNextPage = observer.fetchNextPage();
		await vi.waitFor(() => {
			expect(API.experimental.getChatMessages).toHaveBeenCalledWith(chatId, {
				before_id: 3,
				limit: 50,
			});
		});

		const replacement = [makeMessage(8), makeMessage(7)];
		replaceCachedChatMessages(queryClient, chatId, replacement);
		resolveNextPage({
			messages: [makeMessage(2), makeMessage(1)],
			queued_messages: [],
			has_more: false,
		});
		await fetchNextPage;

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll(chatId).queryKey);
		expect(cached?.pages).toHaveLength(1);
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			8, 7,
		]);
		expect(cached?.pageParams).toEqual([undefined]);
		unsubscribe();
	});

	it("replaces complete queue snapshots by value while preserving pagination", () => {
		const queryClient = createTestQueryClient();
		const original: TypesGen.ChatQueuedMessage = {
			id: 10,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:01:00Z",
			content: [{ type: "text", text: "original" }],
		};
		const updated = {
			...original,
			content: [{ type: "text" as const, text: "updated" }],
		};
		const pageTwo = {
			messages: [makeMessage(2), makeMessage(1)],
			queued_messages: [],
			has_more: false,
		};
		queryClient.setQueryData(chatMessagesForInfiniteScroll("chat-1").queryKey, {
			pages: [
				{
					messages: [makeMessage(4), makeMessage(3)],
					queued_messages: [original],
					has_more: true,
				},
				pageTwo,
			],
			pageParams: [undefined, 3],
		});

		replaceCachedChatQueuedMessages(queryClient, "chat-1", [updated]);

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll("chat-1").queryKey);
		expect(cached?.pages[0]?.queued_messages).toEqual([updated]);
		expect(cached?.pages[1]).toBe(pageTwo);
		expect(cached?.pageParams).toEqual([undefined, 3]);
	});

	it("preserves all pages and page parameters during normal stream upserts", () => {
		const queryClient = createTestQueryClient();
		const pageOne = {
			messages: [makeMessage(4), makeMessage(3)],
			queued_messages: [],
			has_more: true,
		};
		const pageTwo = {
			messages: [makeMessage(2), makeMessage(1)],
			queued_messages: [],
			has_more: false,
		};
		queryClient.setQueryData(chatMessagesForInfiniteScroll("chat-1").queryKey, {
			pages: [pageOne, pageTwo],
			pageParams: [undefined, 3],
		});

		upsertCachedChatMessages(queryClient, "chat-1", [makeMessage(5)]);

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll("chat-1").queryKey);
		expect(cached?.pages).toHaveLength(2);
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			5, 4, 3,
		]);
		expect(cached?.pages[1]).toBe(pageTwo);
		expect(cached?.pageParams).toEqual([undefined, 3]);
	});

	it("atomically replaces authoritative history with one complete page", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatMessagesForInfiniteScroll("chat-1").queryKey, {
			pages: [
				{
					messages: [makeMessage(4), makeMessage(3)],
					queued_messages: [],
					has_more: true,
				},
				{
					messages: [makeMessage(2), makeMessage(1)],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined, 3],
		});
		const replacement = [makeMessage(1), makeMessage(7)];

		replaceCachedChatMessages(queryClient, "chat-1", replacement);

		const cached = queryClient.getQueryData<{
			pages: TypesGen.ChatMessagesResponse[];
			pageParams: unknown[];
		}>(chatMessagesForInfiniteScroll("chat-1").queryKey);
		expect(cached?.pages).toHaveLength(1);
		expect(cached?.pages[0]?.messages.map((message) => message.id)).toEqual([
			7, 1,
		]);
		expect(cached?.pages[0]?.has_more).toBe(false);
		expect(cached?.pageParams).toEqual([undefined]);
	});
});

describe("selectChatMessagesProjection", () => {
	const makeMessage = (
		id: number,
		createdAt: string,
		text = `message ${id}`,
	): TypesGen.ChatMessage => ({
		id,
		chat_id: "chat-1",
		created_at: createdAt,
		role: "user",
		content: [{ type: "text", text }],
	});

	it("flattens, deduplicates, and chronologically orders message pages", () => {
		const pageZeroDuplicate = makeMessage(
			2,
			"2025-01-01T00:00:02Z",
			"newer page value",
		);
		const olderPageDuplicate = makeMessage(
			2,
			"2025-01-01T00:00:02Z",
			"older page value",
		);
		const queuedMessage: TypesGen.ChatQueuedMessage = {
			id: 10,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:01:00Z",
			content: [{ type: "text", text: "queued" }],
		};
		const data = {
			pages: [
				{
					messages: [makeMessage(3, "2025-01-01T00:00:03Z"), pageZeroDuplicate],
					queued_messages: [queuedMessage],
					has_more: true,
				},
				{
					messages: [
						olderPageDuplicate,
						makeMessage(1, "2025-01-01T00:00:01Z"),
					],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined, 2],
		};

		const projection = selectChatMessagesProjection(data);

		expect(projection.messages.map((message) => message.id)).toEqual([1, 2, 3]);
		expect(projection.messages[1]).toBe(pageZeroDuplicate);
		expect(projection.queuedMessages).toBe(data.pages[0].queued_messages);
		expect(data.pages[0].messages).toEqual([
			expect.objectContaining({ id: 3 }),
			pageZeroDuplicate,
		]);
		expect(data.pageParams).toEqual([undefined, 2]);
	});

	it("uses message ID as a deterministic timestamp tie-breaker", () => {
		const projection = selectChatMessagesProjection({
			pages: [
				{
					messages: [
						makeMessage(2, "2025-01-01T00:00:01Z"),
						makeMessage(1, "2025-01-01T00:00:01Z"),
					],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});

		expect(projection.messages.map((message) => message.id)).toEqual([1, 2]);
	});

	it("registers the stable projection selector on the infinite query", () => {
		expect(chatMessagesForInfiniteScroll("chat-1").select).toBe(
			selectChatMessagesProjection,
		);
	});
});

describe("chat query freshness policies", () => {
	const metadataPolicy = {
		staleTime: 15_000,
		gcTime: 300_000,
		retry: 3,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	};

	it.each([
		["list", infiniteChats()],
		["search", chatSearch("status:running")],
		["by workspace", chatsByWorkspace(["workspace-1"])],
		["detail", chat("chat-1")],
	] as const)("makes %s REST repair behavior explicit", (_name, options) => {
		expect(options).toMatchObject(metadataPolicy);
		expect(options.staleTime).not.toBe("static");
	});

	it("uses the per-chat snapshot stream instead of automatic message refetches", () => {
		expect(chatMessagesForInfiniteScroll("chat-1")).toMatchObject({
			staleTime: Number.POSITIVE_INFINITY,
			gcTime: 300_000,
			retry: 3,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
		});
	});

	it.each([
		["prompts", chatPromptsQuery("chat-1"), 30_000, 3],
		["ACL", chatACL("chat-1"), 0, false],
		["diff", chatDiffContents("chat-1"), 30_000, 3],
		["debug runs", chatDebugRuns("chat-1"), 0, false],
		["debug run", chatDebugRun("chat-1", "run-1"), 0, false],
	] as const)("makes %s auxiliary freshness explicit", (_name, options, staleTime, retry) => {
		expect(options).toMatchObject({
			staleTime,
			gcTime: 300_000,
			retry,
			refetchOnMount: true,
			refetchOnWindowFocus: true,
			refetchOnReconnect: true,
		});
		expect(options.staleTime).not.toBe("static");
	});
});

describe("chat projection resynchronization", () => {
	it("refetches every active metadata projection for the baseline", async () => {
		const queryClient = createTestQueryClient();
		const refetch = vi
			.spyOn(queryClient, "refetchQueries")
			.mockResolvedValue(undefined);

		await refetchActiveChatMetadataProjections(queryClient);

		expect(refetch).toHaveBeenCalledWith(
			{
				queryKey: chatQueryKeys.lists(),
				type: "active",
			},
			{ throwOnError: true },
		);
		expect(refetch).toHaveBeenCalledWith(
			{
				queryKey: chatQueryKeys.searches(),
				type: "active",
			},
			{ throwOnError: true },
		);
		expect(refetch).toHaveBeenCalledWith(
			{
				queryKey: chatQueryKeys.byWorkspace(),
				type: "active",
			},
			{ throwOnError: true },
		);
		expect(refetch).toHaveBeenCalledWith(
			expect.objectContaining({
				queryKey: chatQueryKeys.details(),
				type: "active",
				predicate: expect.any(Function),
			}),
			{ throwOnError: true },
		);
	});

	it("rechecks collections and only dirty exact details after the baseline", async () => {
		const queryClient = createTestQueryClient();
		const refetch = vi
			.spyOn(queryClient, "refetchQueries")
			.mockResolvedValue(undefined);
		const invalidate = vi
			.spyOn(queryClient, "invalidateQueries")
			.mockResolvedValue(undefined);

		await refetchDirtyChatMetadataProjections(
			queryClient,
			new Set(["chat-1", "chat-2"]),
		);

		expect(refetch).toHaveBeenCalledTimes(3);
		expect(invalidate).toHaveBeenCalledWith(
			{
				queryKey: chatQueryKeys.detail("chat-1"),
				exact: true,
			},
			{ throwOnError: true },
		);
		expect(invalidate).toHaveBeenCalledWith(
			{
				queryKey: chatQueryKeys.detail("chat-2"),
				exact: true,
			},
			{ throwOnError: true },
		);
	});
});

describe("removeChildFromParentInCache", () => {
	it("removes the child from its parent's children array", () => {
		const queryClient = createTestQueryClient();
		const child = makeChat("child-1", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
		});
		const sibling = makeChat("child-2", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
		});
		const parent = makeChat("parent-1", { children: [child, sibling] });
		seedInfiniteChats(queryClient, [parent]);

		const found = removeChildFromParentInCache(queryClient, "child-1");
		expect(found).toBe(true);

		const result = readInfiniteChats(queryClient);
		expect(result?.[0].children).toHaveLength(1);
		expect(result?.[0].children?.[0].id).toBe("child-2");
	});

	it("returns false when no parent embeds the given child", () => {
		const queryClient = createTestQueryClient();
		const parent = makeChat("parent-1");
		seedInfiniteChats(queryClient, [parent]);

		const found = removeChildFromParentInCache(queryClient, "missing-child");
		expect(found).toBe(false);
	});

	it("preserves the parent reference when the child is not found", () => {
		const queryClient = createTestQueryClient();
		const child = makeChat("child-1", {
			parent_chat_id: "parent-1",
			root_chat_id: "parent-1",
		});
		const parent = makeChat("parent-1", { children: [child] });
		seedInfiniteChats(queryClient, [parent]);

		const before = readInfiniteChats(queryClient)?.[0];
		removeChildFromParentInCache(queryClient, "missing-child");
		const after = readInfiniteChats(queryClient)?.[0];

		expect(after).toBe(before);
	});
});

describe("TERMINAL_RUN_STATUSES", () => {
	// `TERMINAL_RUN_STATUSES` lives in the api/queries layer to avoid a
	// dependency on the page tree, but it must stay in sync with the
	// debug panel's display classification. This test pins that invariant
	// so adding a new success/error status in the panel is immediately
	// caught if the polling set is forgotten.
	it("contains every SUCCESS and ERROR status from the debug panel", () => {
		for (const status of SUCCESS_STATUSES) {
			expect(TERMINAL_RUN_STATUSES.has(status)).toBe(true);
		}
		for (const status of ERROR_STATUSES) {
			expect(TERMINAL_RUN_STATUSES.has(status)).toBe(true);
		}
	});

	// The reverse direction catches a TERMINAL status that stops polling
	// but renders a neutral badge. Adding e.g. "timed_out" to TERMINAL
	// without SUCCESS or ERROR would paint a finished run gray, so the
	// status classification must stay bidirectional.
	it("covers every TERMINAL status with SUCCESS or ERROR", () => {
		for (const status of TERMINAL_RUN_STATUSES) {
			const classified =
				SUCCESS_STATUSES.has(status) || ERROR_STATUSES.has(status);
			expect(classified).toBe(true);
		}
	});
});

describe("chat ACL query factories", () => {
	it("builds the ACL query under the chat key hierarchy", async () => {
		const chatId = "chat-1";
		const acl: TypesGen.ChatACL = { users: [], groups: [] };
		vi.mocked(API.experimental.getChatACL).mockResolvedValue(acl);

		const query = chatACL(chatId);

		expect(chatACL(chatId).queryKey).toEqual([
			"chats",
			"detail",
			chatId,
			"acl",
		]);
		expect(query.queryKey).toEqual(chatACL(chatId).queryKey);
		await expect(query.queryFn?.({} as never)).resolves.toEqual(acl);
		expect(API.experimental.getChatACL).toHaveBeenCalledWith(chatId);
	});

	it("sets one chat user role and invalidates the ACL", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatACL(chatId).queryKey, {
			users: [],
			groups: [],
		});
		vi.mocked(API.experimental.updateChatACL).mockResolvedValue();

		const mutation = setChatUserRole(queryClient);
		const variables = { chatId, userId: "user-1", role: "read" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
			user_roles: { "user-1": "read" },
		});

		await mutation.onSettled(undefined, undefined, variables);
		expect(
			queryClient.getQueryState(chatACL(chatId).queryKey)?.isInvalidated,
		).toBe(true);
	});

	it("sets one chat group role and invalidates the ACL", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatACL(chatId).queryKey, {
			users: [],
			groups: [],
		});
		vi.mocked(API.experimental.updateChatACL).mockResolvedValue();

		const mutation = setChatGroupRole(queryClient);
		const variables = { chatId, groupId: "group-1", role: "" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
			group_roles: { "group-1": "" },
		});

		await mutation.onSettled(undefined, undefined, variables);
		expect(
			queryClient.getQueryState(chatACL(chatId).queryKey)?.isInvalidated,
		).toBe(true);
	});
});
