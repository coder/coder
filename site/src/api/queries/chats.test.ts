import { hashKey, MutationObserver, QueryClient } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { resolvePinnedChatDrop } from "#/pages/AgentsPage/components/ChatsSidebar/chats/pinnedChatDrag";
import {
	ERROR_STATUSES,
	SUCCESS_STATUSES,
} from "#/pages/AgentsPage/components/RightPanel/DebugPanel/debugPanelUtils";
import { buildOptimisticEditedMessage } from "./chatMessageEdits";
import {
	addChildToParentInCache,
	archiveChat,
	CHAT_SUMMARY_STALE_MS,
	cancelChatListRefetches,
	cancelChatMutationRefetches,
	canonicalizeChatListFilters,
	chatACL,
	chatAdvisorConfig,
	chatAdvisorConfigKey,
	chatCost,
	chatCostSummary,
	chat as chatDetail,
	chatKeys,
	chatMessagesForInfiniteScroll,
	chatSearch,
	clearChatUnreadInCaches,
	createChat,
	createChatMessage,
	createCoalescedChatListInvalidator,
	deleteChatQueuedMessage,
	editChatMessage,
	findChatInCaches,
	infiniteChats,
	interruptChat,
	invalidateChatListQueries,
	invalidatePRStatusChatListQueries,
	listFiltersFromKey,
	mergeWatchedChatIntoCaches,
	mergeWatchedChatSummary,
	paginatedChatCostUsers,
	patchChatEverywhere,
	pinChat,
	promoteChatQueuedMessage,
	proposeChatTitle,
	removeChildFromParentInCache,
	reorderPinnedChat,
	selectSortedChatList,
	setChatGroupRole,
	setChatUserRole,
	TERMINAL_RUN_STATUSES,
	unarchiveChat,
	unpinChat,
	updateChatAdvisorConfig,
	updateChatPlanMode,
	updateChatTitle,
	updateChildInParentCache,
	updateInfiniteChatsCache,
} from "./chats";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			updateChat: vi.fn(),
			createChat: vi.fn(),
			deleteChatQueuedMessage: vi.fn(),
			getChats: vi.fn(),
			getChatCost: vi.fn(),
			getChatCostSummary: vi.fn(),
			getChatCostUsers: vi.fn(),
			createChatMessage: vi.fn(),
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

type InfiniteChatsTestOptions = Parameters<typeof chatKeys.list>[0];

const infiniteChatsTestKey = chatKeys.list();

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
	queryClient.setQueryData<InfiniteData>(chatKeys.list(opts), {
		pages: [chats],
		pageParams: [0],
	});
};

/** Read chats back from the infinite query cache. */
const readInfiniteChats = (
	queryClient: QueryClient,
	opts?: InfiniteChatsTestOptions,
): TypesGen.Chat[] | undefined => {
	const data = queryClient.getQueryData<InfiniteData>(chatKeys.list(opts));
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
	snapshot_version: 1,
	archived: false,
	shared: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_turn_summary: null,
	summary: null,
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
			mutations: {
				retry: false,
				// Never pause on the environment's online state: these tests
				// drive the mutation lifecycle directly.
				networkMode: "always",
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
	it("invalidates infinite chat list queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Sidebar queries.
		queryClient.setQueryData(chatKeys.list({ archived: false }), {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		// Per-chat queries that should NOT be touched.
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));
		queryClient.setQueryData(chatKeys.messages(chatId), []);
		queryClient.setQueryData(chatKeys.diffContents(chatId), {});
		queryClient.setQueryData(
			chatKeys.costSummary("me", undefined),
			{} as TypesGen.ChatCostSummary,
		);

		await invalidateChatListQueries(queryClient);

		// Sidebar queries should be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.list({ archived: false }))
				?.isInvalidated,
			"infinite chats should be invalidated",
		).toBe(true);

		// Per-chat queries should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.detail(chatId))?.isInvalidated,
			"the chat detail should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.messages(chatId))?.isInvalidated,
			"chat messages should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.diffContents(chatId))?.isInvalidated,
			"chat diff contents should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.costSummary("me", undefined))
				?.isInvalidated,
			"the cost summary should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidates the infinite query with undefined opts", async () => {
		const queryClient = createTestQueryClient();

		queryClient.setQueryData(chatKeys.list(), {
			pages: [[makeChat("chat-1")]],
			pageParams: [0],
		});

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatKeys.list())?.isInvalidated,
			"infinite chats with undefined opts should be invalidated",
		).toBe(true);
	});

	it("does not invalidate a different chat's queries", async () => {
		const queryClient = createTestQueryClient();
		const otherChatId = "chat-2";

		queryClient.setQueryData(
			chatKeys.detail(otherChatId),
			makeChat(otherChatId),
		);
		queryClient.setQueryData(chatKeys.messages(otherChatId), []);

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatKeys.detail(otherChatId))?.isInvalidated,
			"other chat's detail should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.messages(otherChatId))?.isInvalidated,
			"other chat's messages should NOT be invalidated",
		).not.toBe(true);
	});
});

describe("updateChatPlanMode optimistic update", () => {
	it("rolls back from the list cache when no detail cache exists", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = updateChatPlanMode(queryClient);
		const context = await mutation.onMutate({
			chatId,
			planMode: "plan",
		});

		expect(context.found).toBe(true);
		expect(context.previousPlanMode).toBeUndefined();
		expect(readInfiniteChats(queryClient)?.[0].plan_mode).toBe("plan");

		mutation.onError(
			new Error("server error"),
			{ chatId, planMode: "plan" },
			context,
		);

		expect(readInfiniteChats(queryClient)?.[0].plan_mode).toBeUndefined();
		expect(
			invalidateSpy,
			"rollback is the inverse patch, not a refetch",
		).not.toHaveBeenCalled();
	});

	it("keeps a plan mode a later write installed during the mutation", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { plan_mode: "plan" })]);
		queryClient.setQueryData(
			chatKeys.detail(chatId),
			makeChat(chatId, { plan_mode: "plan" }),
		);

		const mutation = updateChatPlanMode(queryClient);
		const context = await mutation.onMutate({ chatId, planMode: undefined });

		// A concurrent write lands while the PATCH is in flight.
		patchChatEverywhere(queryClient, chatId, (chat) => ({
			...chat,
			plan_mode: "plan",
		}));

		mutation.onError(
			new Error("server error"),
			{ chatId, planMode: undefined },
			context,
		);

		expect(readInfiniteChats(queryClient)?.[0].plan_mode).toBe("plan");
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.plan_mode,
		).toBe("plan");
	});

	it("converges a failed toggle without returning pending promises", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi
			.spyOn(queryClient, "invalidateQueries")
			.mockReturnValue(new Promise<void>(() => {}));

		const mutation = updateChatPlanMode(queryClient);
		const result = mutation.onSettled(undefined, new Error("boom"), {
			chatId,
			planMode: "plan",
		});

		expect(result).toBeUndefined();
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		invalidateSpy.mockRestore();
	});
});

describe("updateChatTitle cache update", () => {
	it("patches chat detail and infinite chat list caches after success", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(
			chatKeys.detail(chatId),
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

		const mutation = updateChatTitle(queryClient);
		mutation.onSuccess(undefined, { chatId, title: "  New  " });

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))?.title,
			"the server trims the title, so the cache must too",
		).toBe("New");
		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId),
		).toMatchObject({ title: "New" });
		expect(
			readInfiniteChats(queryClient, { archived: true })?.find(
				(chat) => chat.id === chatId,
			),
		).toMatchObject({ title: "New" });
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
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKeys.detail(chatId),
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
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));

		vi.mocked(API.experimental.updateChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const cachedChat = queryClient.getQueryData<TypesGen.Chat>(
			chatKeys.detail(chatId),
		);
		expect(cachedChat?.archived).toBe(true);
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
			chatKeys.detail(chatId),
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
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId)),
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

	it("restores the chats list on error with an inverse patch", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 4 })]);
		queryClient.setQueryData(
			chatKeys.detail(chatId),
			makeChat(chatId, { pin_order: 4 }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify the optimistic update took effect.
		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			archived: true,
			pin_order: 0,
		});

		mutation.onError(new Error("server error"), chatId, context);

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			archived: false,
			pin_order: 4,
		});
		expect(
			invalidateSpy,
			"rollback is the inverse patch, not a refetch",
		).not.toHaveBeenCalled();
	});

	it("leaves a chat the server already archived alone on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 4 })]);

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// A concurrent write moves pin_order off the optimistic value,
		// so the value guard must skip the rollback.
		patchChatEverywhere(queryClient, chatId, (chat) => ({
			...chat,
			pin_order: 7,
		}));

		mutation.onError(new Error("server error"), chatId, context);

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			archived: true,
			pin_order: 7,
		});
	});

	it("rolls back the individual chat cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.archived,
		).toBe(true);

		mutation.onError(new Error("server error"), chatId, context);

		const rolledBack = queryClient.getQueryData<TypesGen.Chat>(
			chatKeys.detail(chatId),
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

		// Without a captured prior value there is nothing to restore, and
		// the onSettled invalidation is the only convergence path.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("handles onMutate when no individual chat cache exists", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		// Deliberately do NOT set chatKeys.detail(chatId) data.

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// The list should still be optimistically updated.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);
		// The prior state is read from the list cache instead.
		expect(context).toMatchObject({
			found: true,
			previousArchived: false,
			previousPinOrder: 0,
		});
	});

	it("reports nothing to roll back when the chat is in no cache", async () => {
		const queryClient = createTestQueryClient();

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate("chat-missing");

		expect(context.found).toBe(false);
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
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKeys.detail(chatId),
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
			chatKeys.detail(chatId),
			makeChat(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.archived,
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
			chatKeys.detail(chatId),
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
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId)),
		).toMatchObject({
			archived: false,
		});
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chatKeys.detail(chatId),
			makeChat(chatId, { archived: true }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unarchiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(false);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.archived,
		).toBe(false);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// Both layers are restored by the inverse patch, with no refetch.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.archived,
		).toBe(true);
		expect(invalidateSpy).not.toHaveBeenCalled();
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
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		invalidateSpy.mockRestore();
	});
});

describe("pinChat optimistic update", () => {
	it("optimistically appends a newly pinned chat after the highest cached pin order", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-new";
		seedInfiniteChats(queryClient, [
			makeChat("chat-pinned-1", { pin_order: 1 }),
			makeChat(chatId),
			makeChat("chat-pinned-2", { pin_order: 2 }),
		]);
		queryClient.setQueryData(chatKeys.list({ archived: true }), {
			pages: [[makeChat("chat-pinned-archived", { pin_order: 4 })]],
			pageParams: [0],
		});
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));

		const mutation = pinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)
				?.pin_order,
		).toBe(5);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.pin_order,
		).toBe(5);
	});

	it("rolls back the optimistic pin order on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-new";
		seedInfiniteChats(queryClient, [
			makeChat("chat-pinned-1", { pin_order: 1 }),
			makeChat(chatId),
		]);

		const mutation = pinChat(queryClient);
		const context = await mutation.onMutate(chatId);
		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)
				?.pin_order,
		).toBe(2);

		mutation.onError(new Error("server error"), chatId, context);

		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)
				?.pin_order,
		).toBe(0);
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
			chatKeys.detail(chatId),
			makeChat(chatId, { pin_order: 2 }),
		);

		const mutation = unpinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.pin_order,
		).toBe(0);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })]);
		queryClient.setQueryData(
			chatKeys.detail(chatId),
			makeChat(chatId, { pin_order: 3 }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].pin_order).toBe(0);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.pin_order,
		).toBe(0);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// Both layers are restored by the inverse patch, with no refetch.
		expect(readInfiniteChats(queryClient)?.[0].pin_order).toBe(3);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))
				?.pin_order,
		).toBe(3);
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("invalidates queries on settled", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		// Unpinning renumbers the remaining pinned chats, so every loaded
		// chat detail is reconciled, not just this mutation's target.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.details() }),
		);
	});
});

describe("reorderPinnedChat", () => {
	const pinnedChats = [
		makeChat("chat-1", { pin_order: 1 }),
		makeChat("chat-2", { pin_order: 2 }),
		makeChat("chat-3", { pin_order: 3 }),
	];

	it("updates a single chat via updateChat and invalidates list and detail queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, pinnedChats);
		vi.mocked(API.experimental.updateChat).mockResolvedValue(undefined);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		const variables = { chatId, pinOrder: 2, visibleChats: pinnedChats };
		const mutation = reorderPinnedChat(queryClient);
		await mutation.onMutate(variables);
		await mutation.mutationFn(variables);
		const settledResult = mutation.onSettled(undefined, undefined, variables);

		expect(settledResult).toBeUndefined();
		expect(cancelSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(cancelSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				queryKey: chatKeys.detail(chatId),
				exact: true,
			}),
		);
		expect(API.experimental.updateChat).toHaveBeenCalledWith(chatId, {
			pin_order: 2,
		});
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.details() }),
		);
	});

	it("renumbers from the visible order, not the first cached list variant", async () => {
		const queryClient = createTestQueryClient();
		// An archived variant iterates first and holds no pinned chats.
		seedInfiniteChats(queryClient, [makeChat("chat-archived")], {
			archived: true,
		});
		seedInfiniteChats(queryClient, pinnedChats);

		const mutation = reorderPinnedChat(queryClient);
		await mutation.onMutate({
			chatId: "chat-3",
			pinOrder: 1,
			visibleChats: pinnedChats,
		});

		expect(
			readInfiniteChats(queryClient)?.map((chat) => [chat.id, chat.pin_order]),
		).toEqual([
			["chat-1", 2],
			["chat-2", 3],
			["chat-3", 1],
		]);
	});

	it("restores prior pin orders on error, guarded on the optimistic value", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, pinnedChats);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const variables = {
			chatId: "chat-3",
			pinOrder: 1,
			visibleChats: pinnedChats,
		};
		const mutation = reorderPinnedChat(queryClient);
		const context = await mutation.onMutate(variables);

		// chat-2 is moved again while the PATCH is in flight, so its
		// pin_order no longer matches what this mutation assigned.
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === "chat-2" ? { ...chat, pin_order: 9 } : chat,
			),
		);

		mutation.onError(new Error("server error"), variables, context);

		expect(
			readInfiniteChats(queryClient)?.map((chat) => [chat.id, chat.pin_order]),
		).toEqual([
			["chat-1", 1],
			["chat-2", 9],
			["chat-3", 3],
		]);
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("is a no-op when the dragged chat is not pinned", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, pinnedChats);

		const mutation = reorderPinnedChat(queryClient);
		const context = await mutation.onMutate({
			chatId: "chat-unpinned",
			pinOrder: 1,
			visibleChats: pinnedChats,
		});

		expect(context).toBeUndefined();
		expect(
			readInfiniteChats(queryClient)?.map((chat) => chat.pin_order),
		).toEqual([1, 2, 3]);
	});

	it("serializes overlapping drags and invalidates once, after the last one", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, pinnedChats);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const deferred: Array<() => void> = [];
		vi.mocked(API.experimental.updateChat).mockImplementation(
			() =>
				new Promise<void>((resolve) => {
					deferred.push(resolve);
				}),
		);

		const base = reorderPinnedChat(queryClient);
		const first = new MutationObserver(queryClient, base);
		const second = new MutationObserver(queryClient, base);
		const firstResult = first.mutate({
			chatId: "chat-3",
			pinOrder: 1,
			visibleChats: pinnedChats,
		});
		await vi.waitFor(() => expect(deferred).toHaveLength(1));
		expect(
			readInfiniteChats(queryClient)?.map((chat) => [chat.id, chat.pin_order]),
		).toEqual([
			["chat-1", 2],
			["chat-2", 3],
			["chat-3", 1],
		]);

		const secondResult = second.mutate({
			chatId: "chat-1",
			pinOrder: 1,
			// The sidebar renders pinned chats in ascending pin_order, so
			// the second drag sees the first drag's optimistic ordering.
			visibleChats: (readInfiniteChats(queryClient) ?? []).toSorted(
				(a, b) => a.pin_order - b.pin_order,
			),
		});

		// The second optimistic write lands immediately: onMutate runs
		// before the scope gate pauses mutationFn, so the drag is visible
		// while the first request is still in flight.
		await vi.waitFor(() =>
			expect(
				readInfiniteChats(queryClient)?.find((chat) => chat.id === "chat-1")
					?.pin_order,
			).toBe(1),
		);
		expect(
			readInfiniteChats(queryClient)?.map((chat) => [chat.id, chat.pin_order]),
		).toEqual([
			["chat-1", 1],
			["chat-2", 3],
			["chat-3", 2],
		]);
		expect(deferred, "the second PATCH waits on the shared scope").toHaveLength(
			1,
		);

		deferred[0]();
		await firstResult;
		expect(
			invalidateSpy,
			"an early settle refetch would overwrite the pending optimistic write",
		).not.toHaveBeenCalled();

		await vi.waitFor(() => expect(deferred).toHaveLength(2));
		deferred[1]();
		await secondResult;

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatKeys.lists() }),
		);
		expect(invalidateSpy).toHaveBeenCalledTimes(2);
	});

	it("skips the rollback while a later reorder is still pending", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, pinnedChats);

		const deferred: Array<{
			resolve: () => void;
			reject: (error: Error) => void;
		}> = [];
		vi.mocked(API.experimental.updateChat).mockImplementation(
			() =>
				new Promise<void>((resolve, reject) => {
					deferred.push({ resolve, reject });
				}),
		);

		const base = reorderPinnedChat(queryClient);
		const first = new MutationObserver(queryClient, base);
		const second = new MutationObserver(queryClient, base);
		const firstResult = first
			.mutate({ chatId: "chat-3", pinOrder: 1, visibleChats: pinnedChats })
			.catch(() => {});
		await vi.waitFor(() => expect(deferred).toHaveLength(1));

		const secondResult = second.mutate({
			chatId: "chat-1",
			pinOrder: 1,
			visibleChats: (readInfiniteChats(queryClient) ?? []).toSorted(
				(a, b) => a.pin_order - b.pin_order,
			),
		});
		await vi.waitFor(() =>
			expect(
				readInfiniteChats(queryClient)?.find((chat) => chat.id === "chat-1")
					?.pin_order,
			).toBe(1),
		);

		deferred[0].reject(new Error("reorder failed"));
		await firstResult;

		// The first drag's inverse patch would restore chat-2 to 2, which
		// the second drag has already given to chat-3.
		expect(
			readInfiniteChats(queryClient)?.map((chat) => [chat.id, chat.pin_order]),
			"a superseded rollback would corrupt the pending drag's order",
		).toEqual([
			["chat-1", 1],
			["chat-2", 3],
			["chat-3", 2],
		]);

		await vi.waitFor(() => expect(deferred).toHaveLength(2));
		deferred[1].resolve();
		await secondResult;
	});

	it("rolls back to the previous drop's order when the sidebar prop is stale", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, pinnedChats);
		vi.mocked(API.experimental.updateChat)
			.mockResolvedValueOnce(undefined)
			.mockRejectedValueOnce(new Error("reorder failed"));

		const base = reorderPinnedChat(queryClient);
		const readPinOrders = () =>
			readInfiniteChats(queryClient)?.map((chat) => [chat.id, chat.pin_order]);

		// First drop, against the chats the sidebar was rendered with.
		const first = resolvePinnedChatDrop({
			pinnedChats,
			hasLocalOrder: false,
			activeId: "chat-3",
			overId: "chat-1",
		});
		if (!first) {
			throw new Error("expected the first drop to resolve");
		}
		await new MutationObserver(queryClient, base).mutate({
			chatId: first.chatId,
			pinOrder: first.pinOrder,
			visibleChats: first.mutationChats,
		});
		expect(first.localOrder).toEqual(["chat-3", "chat-1", "chat-2"]);
		expect(readPinOrders()).toEqual([
			["chat-1", 2],
			["chat-2", 3],
			["chat-3", 1],
		]);

		// Second drop before the parent rerenders: the panel renders its
		// local order over chats whose pin_order fields still describe the
		// pre-first-drop world.
		const rendered = [pinnedChats[2], pinnedChats[0], pinnedChats[1]];
		const second = resolvePinnedChatDrop({
			pinnedChats: rendered,
			hasLocalOrder: true,
			activeId: "chat-2",
			overId: "chat-3",
		});
		if (!second) {
			throw new Error("expected the second drop to resolve");
		}
		await new MutationObserver(queryClient, base)
			.mutate({
				chatId: second.chatId,
				pinOrder: second.pinOrder,
				visibleChats: second.mutationChats,
			})
			.catch(() => {});

		// A rollback built from the stale fields would restore the order
		// from before the first drop, undoing a request the server
		// accepted.
		expect(readPinOrders()).toEqual([
			["chat-1", 2],
			["chat-2", 3],
			["chat-3", 1],
		]);
	});
});

describe("pin mutation settlement", () => {
	it("invalidates every loaded chat detail once the last pin mutation settles", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, [
			makeChat("chat-1", { pin_order: 1 }),
			makeChat("chat-2", { pin_order: 2 }),
		]);
		queryClient.setQueryData(
			chatKeys.detail("chat-1"),
			makeChat("chat-1", { pin_order: 1 }),
		);
		queryClient.setQueryData(
			chatKeys.detail("chat-2"),
			makeChat("chat-2", { pin_order: 2 }),
		);
		queryClient.setQueryData(chatKeys.messages("chat-1"), {
			pages: [],
			pageParams: [],
		});

		const deferred: Array<() => void> = [];
		vi.mocked(API.experimental.updateChat).mockImplementation(
			() =>
				new Promise<void>((resolve) => {
					deferred.push(resolve);
				}),
		);

		const pinning = new MutationObserver(
			queryClient,
			pinChat(queryClient),
		).mutate("chat-1");
		const unpinning = new MutationObserver(
			queryClient,
			unpinChat(queryClient),
		).mutate("chat-2");
		await vi.waitFor(() => expect(deferred).toHaveLength(2));

		deferred[0]();
		await pinning;
		expect(
			queryClient.getQueryState(chatKeys.detail("chat-2"))?.isInvalidated,
			"an early settle refetch would overwrite the pending optimistic write",
		).toBe(false);

		deferred[1]();
		await unpinning;

		// The server renumbers the whole pinned sequence, so the last
		// settle has to reconcile every loaded chat detail, not just the
		// chat it targeted.
		expect(
			queryClient.getQueryState(chatKeys.detail("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.detail("chat-2"))?.isInvalidated,
		).toBe(true);
		expect(queryClient.getQueryState(chatKeys.list())?.isInvalidated).toBe(
			true,
		);
		expect(
			queryClient.getQueryState(chatKeys.messages("chat-1"))?.isInvalidated,
			"nested detail caches hold nothing a pin_order change invalidates",
		).toBe(false);
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

		expect(chatKeys.costSummary(user, params)).toEqual([
			"chats",
			"cost-summary",
			user,
			params,
		]);
		expect(query.queryKey).toEqual(["chats", "cost-summary", user, params]);
		await query.queryFn();
		expect(API.experimental.getChatCostSummary).toHaveBeenCalledWith(
			user,
			params,
		);
	});

	it("builds the per-chat cost query key and forwards the chat id", async () => {
		const chatId = "chat-1";
		vi.mocked(API.experimental.getChatCost).mockResolvedValue(
			{} as TypesGen.ChatCost,
		);

		const query = chatCost(chatId);

		expect(chatKeys.cost(chatId)).toEqual(["chats", "detail", chatId, "cost"]);
		expect(query.queryKey).toEqual(["chats", "detail", chatId, "cost"]);
		await query.queryFn();
		expect(API.experimental.getChatCost).toHaveBeenCalledWith(chatId);
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
		expect(key).toEqual(["chats", "cost-users", payload, 2]);

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
	// needs to refresh, not the entire chatKeys.all prefix tree. The
	// WebSocket stream already delivers real-time updates for
	// messages, status changes, and sidebar ordering, so broad
	// prefix invalidation causes a burst of redundant HTTP requests
	// on the /agents page.

	/** Populate the QueryClient with every query key that is actively
	 *  observed on the /agents/:id detail page. */
	const seedAllActiveQueries = (queryClient: QueryClient, chatId: string) => {
		queryClient.setQueryData(chatKeys.list({ archived: false }), {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));
		queryClient.setQueryData(chatKeys.messages(chatId), []);
		queryClient.setQueryData(chatKeys.debugRuns(chatId), []);
		queryClient.setQueryData(chatKeys.diffContents(chatId), { files: [] });
		queryClient.setQueryData(
			chatKeys.costSummary("me", undefined),
			{} as TypesGen.ChatCostSummary,
		);
	};

	/** Keys that should NEVER be invalidated by chat message mutations
	 *  because they are completely unrelated to the message flow. */
	const unrelatedKeys = (chatId: string) => [
		{ label: "diff-contents", key: chatKeys.diffContents(chatId) },
		{ label: "cost-summary", key: chatKeys.costSummary("me", undefined) },
	];

	it("createChatMessage does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChatMessage(queryClient, chatId);
		await mutation.onSuccess?.();

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by createChatMessage`,
			).not.toBe(true);
		}
	});

	it("createChatMessage invalidates debug runs and chat detail, not messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChatMessage(queryClient, chatId);
		await mutation.onSuccess?.();

		expect(
			queryClient.getQueryState(chatKeys.debugRuns(chatId))?.isInvalidated,
			"debug runs should be invalidated",
		).toBe(true);

		const chatState = queryClient.getQueryState(chatKeys.detail(chatId));
		expect(
			chatState?.isInvalidated,
			"the chat detail should be invalidated",
		).toBe(true);

		const messagesState = queryClient.getQueryState(chatKeys.messages(chatId));
		expect(
			messagesState?.isInvalidated,
			"chat messages should NOT be invalidated",
		).not.toBe(true);
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

	it("editChatMessage invalidates chat detail and debug runs, not messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = editChatMessage(queryClient, chatId);
		mutation.onSettled();

		await new Promise((r) => setTimeout(r, 0));

		// Chat metadata and debug runs should be invalidated because
		// editing changes the chat's updated_at and can start a new
		// debug run.
		const chatState = queryClient.getQueryState(chatKeys.detail(chatId));
		expect(
			chatState?.isInvalidated,
			"the chat detail should be invalidated",
		).toBe(true);

		// Messages are NOT invalidated. The per-chat WebSocket handles
		// post-edit message delivery, making REST invalidation
		// unnecessary.
		const messagesState = queryClient.getQueryState(chatKeys.messages(chatId));
		expect(
			messagesState?.isInvalidated,
			"chat messages should not be invalidated",
		).not.toBe(true);

		expect(
			queryClient.getQueryState(chatKeys.debugRuns(chatId))?.isInvalidated,
			"debug runs should be invalidated",
		).toBe(true);
	});

	it("editChatMessage onError invalidates messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

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

		await new Promise((r) => setTimeout(r, 0));

		const messagesState = queryClient.getQueryState(chatKeys.messages(chatId));
		expect(
			messagesState?.isInvalidated,
			"chat messages should be invalidated on error",
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

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
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

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [
				{
					messages,
					queued_messages: queuedMessages,
					has_more: false,
				},
			],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
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

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		expect(
			queryClient.getQueryData<InfMessages>(chatKeys.messages(chatId))?.pages[0]
				?.messages,
		).toHaveLength(3);

		mutation.onError(
			new Error("network failure"),
			{ messageId: 3, optimisticMessage, req: editReq },
			context,
		);

		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
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

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});
		queryClient.setQueryData<InfMessages | undefined>(
			chatKeys.messages(chatId),
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
			chatKeys.messages(chatId),
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			10, 9, 2, 1,
		]);
		expect(data?.pages[0]?.messages[1]?.content).toEqual(
			responseMessage.content,
		);
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
		expect(queryClient.getQueryData(chatKeys.messages(chatId))).toBeUndefined();
	});

	it("editChatMessage onError handles undefined context gracefully", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);

		// Pass undefined context. This simulates onMutate throwing before
		// it could return a snapshot.
		mutation.onError(
			new Error("fail"),
			{ messageId: 2, req: editReq },
			undefined,
		);

		// Cache should be untouched: no crash, no corruption.
		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
		);
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([3, 2, 1]);

		await new Promise((r) => setTimeout(r, 0));
		const messagesState = queryClient.getQueryState(chatKeys.messages(chatId));
		expect(
			messagesState?.isInvalidated,
			"chat messages should be invalidated even without context",
		).toBe(true);
	});

	it("editChatMessage onMutate updates the first page and preserves older pages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Page 0 (newest): IDs 10 to 6. Page 1 (older): IDs 5 to 1.
		const page0 = [10, 9, 8, 7, 6].map((id) => makeMsg(chatId, id));
		const page1 = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(requireMessage(page0, 7));

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [
				{ messages: page0, queued_messages: [], has_more: true },
				{ messages: page1, queued_messages: [], has_more: false },
			],
			pageParams: [undefined, 6],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 7,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
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

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 1,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
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

		queryClient.setQueryData<InfMessages>(chatKeys.messages(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 5,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(
			chatKeys.messages(chatId),
		);
		expect(data?.pages[0]?.messages.map((message) => message.id)).toEqual([
			5, 4, 3, 2, 1,
		]);
		expect(data?.pages[0]?.messages[0]?.content).toEqual(
			optimisticMessage.content,
		);
	});

	it("interruptChat invalidates debug runs without touching unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = interruptChat(queryClient, chatId);
		await mutation.onSuccess?.();

		expect(
			queryClient.getQueryState(chatKeys.debugRuns(chatId))?.isInvalidated,
			"debug runs should be invalidated",
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
		await mutation.onSuccess?.();

		expect(
			queryClient.getQueryState(chatKeys.debugRuns(chatId))?.isInvalidated,
			"debug runs should be invalidated",
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
				queryClient.getQueryState(chatKeys.debugRuns(chatId))?.isInvalidated,
				"debug runs should be invalidated",
			).toBe(true);

			for (const { label, key } of [
				{
					label: "infinite chats",
					key: chatKeys.list({ archived: false }),
				},
				{ label: "chat detail", key: chatKeys.detail(chatId) },
				{ label: "messages", key: chatKeys.messages(chatId) },
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

	it("createChat invalidates only sidebar queries on success", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChat(queryClient);
		mutation.onSuccess();

		await new Promise((r) => setTimeout(r, 0));

		// Sidebar lists SHOULD be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.list({ archived: false }))
				?.isInvalidated,
			"infinite chats should be invalidated",
		).toBe(true);

		// Per-chat queries should NOT be touched.
		for (const { label, key } of unrelatedKeys(chatId)) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} should NOT be invalidated by createChat`,
			).not.toBe(true);
		}
		expect(
			queryClient.getQueryState(chatKeys.detail(chatId))?.isInvalidated,
			"the chat detail should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.messages(chatId))?.isInvalidated,
			"chat messages should NOT be invalidated",
		).not.toBe(true);
	});

	it("deleteChatQueuedMessage invalidates only chat detail and messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = deleteChatQueuedMessage(queryClient, chatId);
		await mutation.onSuccess();

		// These two should be invalidated (exact match).
		expect(
			queryClient.getQueryState(chatKeys.detail(chatId))?.isInvalidated,
			"the chat detail should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.messages(chatId))?.isInvalidated,
			"chat messages should be invalidated",
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
			queryClient.getQueryState(chatKeys.list({ archived: false }))
				?.isInvalidated,
			"infinite chats should NOT be invalidated",
		).not.toBe(true);
	});
});

describe("chatKeys", () => {
	it("composes every key from a literal in slot 1", () => {
		const chatId = "chat-1";

		expect(chatKeys.all).toEqual(["chats"]);
		expect(chatKeys.lists()).toEqual(["chats", "list"]);
		expect(chatKeys.list({ archived: true })).toEqual([
			"chats",
			"list",
			{ filters: { archived: true } },
		]);
		expect(chatKeys.details()).toEqual(["chats", "detail"]);
		expect(chatKeys.detail(chatId)).toEqual(["chats", "detail", chatId]);
		expect(chatKeys.messages(chatId)).toEqual([
			"chats",
			"detail",
			chatId,
			"messages",
		]);
		expect(chatKeys.debugRun(chatId, "run-1")).toEqual([
			"chats",
			"detail",
			chatId,
			"debug-runs",
			"run-1",
		]);
		expect(chatKeys.search("title:fix")).toEqual([
			"chats",
			"search",
			{ q: "title:fix" },
		]);
		expect(chatKeys.byWorkspacePrefix()).toEqual(["chats", "by-workspace"]);
	});

	it("sorts workspace ids so argument order cannot fork the cache", () => {
		expect(hashKey(chatKeys.byWorkspace(["ws-2", "ws-1"]))).toBe(
			hashKey(chatKeys.byWorkspace(["ws-1", "ws-2"])),
		);
	});

	it("hashes every empty filter input to one cache entry", () => {
		const expected = hashKey(chatKeys.list());

		expect(hashKey(chatKeys.list({}))).toBe(expected);
		expect(hashKey(chatKeys.list({ prStatuses: [] }))).toBe(expected);
		expect(hashKey(chatKeys.list({ sources: [] }))).toBe(expected);
		expect(hashKey(chatKeys.list({ chatStatus: undefined }))).toBe(expected);
	});

	it("keeps archived:false, which is a real filter value", () => {
		expect(hashKey(chatKeys.list({ archived: false }))).not.toBe(
			hashKey(chatKeys.list({})),
		);
	});

	it("hashes array order variants together and requests the same q", () => {
		const ascending = infiniteChats({
			prStatuses: ["open", "draft"],
			sources: ["shared_with_me", "created_by_me"],
		});
		const descending = infiniteChats({
			prStatuses: ["draft", "open"],
			sources: ["created_by_me", "shared_with_me"],
		});

		expect(hashKey(ascending.queryKey)).toBe(hashKey(descending.queryKey));

		vi.mocked(API.experimental.getChats).mockResolvedValue([]);
		ascending.queryFn({ pageParam: 0 });
		descending.queryFn({ pageParam: 0 });
		const [firstCall, secondCall] = vi.mocked(API.experimental.getChats).mock
			.calls;
		expect(firstCall[0]?.q).toBe(
			"pr_status:draft,open source:created_by_me,shared_with_me",
		);
		expect(secondCall[0]?.q).toBe(firstCall[0]?.q);
	});

	it("matches only infinite lists under the lists() prefix", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(chatKeys.list({ archived: false }), {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		queryClient.setQueryData(chatKeys.search("fix"), [makeChat(chatId)]);
		queryClient.setQueryData(chatKeys.byWorkspace(["ws-1"]), {
			"ws-1": chatId,
		});
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));
		queryClient.setQueryData(chatKeys.messages(chatId), []);
		queryClient.setQueryData(
			chatKeys.costSummary("me"),
			{} as TypesGen.ChatCostSummary,
		);

		const matched = queryClient
			.getQueryCache()
			.findAll({ queryKey: chatKeys.lists() })
			.map((query) => query.queryKey);

		expect(matched).toEqual([chatKeys.list({ archived: false })]);
	});
});

describe("canonicalizeChatListFilters", () => {
	it("drops absent fields and empty arrays", () => {
		expect(
			canonicalizeChatListFilters({
				archived: false,
				chatStatus: undefined,
				prStatuses: [],
				sources: [],
			}),
		).toEqual({ archived: false });
	});

	it("orders array values", () => {
		expect(
			canonicalizeChatListFilters({
				prStatuses: ["closed", "draft"],
				sources: ["shared_with_me", "created_by_me"],
			}),
		).toEqual({
			prStatuses: ["draft", "closed"],
			sources: ["created_by_me", "shared_with_me"],
		});
	});
});

describe("infiniteChats", () => {
	const PAGE_LIMIT = 50;

	describe("getNextPageParam", () => {
		it("returns undefined when lastPage has fewer items than the limit", () => {
			const { getNextPageParam } = infiniteChats();
			const lastPage = Array.from({ length: PAGE_LIMIT - 1 }, (_, i) =>
				makeChat(`chat-${i}`),
			);
			expect(getNextPageParam(lastPage, [lastPage])).toBeUndefined();
		});

		it("returns pages.length + 1 when lastPage has exactly the limit", () => {
			const { getNextPageParam } = infiniteChats();
			const lastPage = Array.from({ length: PAGE_LIMIT }, (_, i) =>
				makeChat(`chat-${i}`),
			);
			const pages = [lastPage];
			expect(getNextPageParam(lastPage, pages)).toBe(pages.length + 1);
		});
	});

	describe("queryFn", () => {
		it("computes offset 0 for pageParam 0", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats();
			await queryFn({ pageParam: 0 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: 0,
			});
		});

		it("computes offset 0 for pageParam <= 0", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats();
			await queryFn({ pageParam: -1 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: 0,
			});
		});

		it("computes correct offset for subsequent pages", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats();

			await queryFn({ pageParam: 2 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: PAGE_LIMIT,
			});

			await queryFn({ pageParam: 3 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: PAGE_LIMIT * 2,
			});
		});

		it("builds q from archived, prStatuses, chatStatus, and sources", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats({
				archived: true,
				prStatuses: ["draft", "open", "merged"],
				chatStatus: "unread",
				sources: ["created_by_me", "shared_with_me"],
			});

			await queryFn({ pageParam: 0 });

			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: 0,
				q: "archived:true pr_status:draft,open,merged has_unread:true source:created_by_me,shared_with_me",
			});
		});

		it("builds q for read chat status", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats({
				archived: false,
				chatStatus: "read",
			});

			await queryFn({ pageParam: 0 });

			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: 0,
				q: "archived:false has_unread:false",
			});
		});

		it("throws when pageParam is not a number", () => {
			const { queryFn } = infiniteChats();
			expect(() => queryFn({ pageParam: "bad" })).toThrow(
				"pageParam must be a number",
			);
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

	it("exact detail invalidation does not cascade to messages or diff-contents", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Seed all the queries that are active on the /agents/:id page.
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));
		queryClient.setQueryData(chatKeys.messages(chatId), []);
		queryClient.setQueryData(chatKeys.diffContents(chatId), { files: [] });
		queryClient.setQueryData(chatKeys.list(), {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});

		// This is what the fixed handler does, exact: true.
		await queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});

		// The chat detail itself should be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.detail(chatId))?.isInvalidated,
			"the chat detail should be invalidated",
		).toBe(true);

		// Messages should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.messages(chatId))?.isInvalidated,
			"chat messages should NOT be invalidated by an exact detail filter",
		).not.toBe(true);

		// Diff-contents should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.diffContents(chatId))?.isInvalidated,
			"chat diff contents should NOT be invalidated by an exact detail filter",
		).not.toBe(true);

		// Chat list should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatKeys.list())?.isInvalidated,
			"infinite chats should NOT be invalidated by an exact detail filter",
		).not.toBe(true);
	});

	it("without exact: true, detail invalidation cascades to messages and diff-contents (the old bug)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));
		queryClient.setQueryData(chatKeys.messages(chatId), []);
		queryClient.setQueryData(chatKeys.diffContents(chatId), { files: [] });

		// This is what the OLD (broken) handler did, no exact: true.
		await queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
		});

		// Without exact: true, every sub-resource of the chat detail key
		// gets invalidated, including messages and diff-contents.
		expect(
			queryClient.getQueryState(chatKeys.messages(chatId))?.isInvalidated,
			"chat messages ARE invalidated without exact: true (old bug)",
		).toBe(true);

		expect(
			queryClient.getQueryState(chatKeys.diffContents(chatId))?.isInvalidated,
			"chat diff contents ARE invalidated without exact: true (old bug)",
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
		// refetch is in flight. This mirrors what AgentsPageLayout does.
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

describe("patchChatEverywhere", () => {
	it("patches the detail cache, every list variant, and embedded children", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatKeys.detail(chatId), makeChat(chatId));
		seedInfiniteChats(queryClient, [makeChat(chatId), makeChat("chat-2")]);
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })], {
			archived: true,
		});
		seedInfiniteChats(
			queryClient,
			[makeChat("parent-1", { children: [makeChat(chatId)] })],
			{ chatStatus: "unread" },
		);

		patchChatEverywhere(queryClient, chatId, (chat) => ({
			...chat,
			title: "Patched",
		}));

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))?.title,
		).toBe("Patched");
		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)?.title,
		).toBe("Patched");
		expect(readInfiniteChats(queryClient, { archived: true })?.[0].title).toBe(
			"Patched",
		);
		expect(
			readInfiniteChats(queryClient, { chatStatus: "unread" })?.[0]
				.children?.[0].title,
		).toBe("Patched");
		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === "chat-2")
				?.title,
		).toBe("Chat chat-2");
	});

	it("keeps the page reference when the patch changes nothing", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		const before = queryClient.getQueryData(infiniteChatsTestKey);

		patchChatEverywhere(queryClient, chatId, (chat) => chat);

		expect(queryClient.getQueryData(infiniteChatsTestKey)).toBe(before);
	});

	it("does not create a detail cache entry for an unopened chat", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);

		patchChatEverywhere(queryClient, chatId, (chat) => ({
			...chat,
			title: "Patched",
		}));

		expect(queryClient.getQueryData(chatKeys.detail(chatId))).toBeUndefined();
	});
});

describe("findChatInCaches", () => {
	it("prefers the detail cache", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(
			chatKeys.detail(chatId),
			makeChat(chatId, { title: "Detail" }),
		);
		seedInfiniteChats(queryClient, [makeChat(chatId, { title: "List" })]);

		expect(findChatInCaches(queryClient, chatId)?.title).toBe("Detail");
	});

	it("falls back to any cached list variant", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat("chat-2")]);
		seedInfiniteChats(queryClient, [makeChat(chatId, { title: "Archived" })], {
			archived: true,
		});

		expect(findChatInCaches(queryClient, chatId)?.title).toBe("Archived");
	});

	it("falls back to a parent's embedded children", () => {
		const queryClient = createTestQueryClient();
		const child = makeChat("child-1", { title: "Child" });
		seedInfiniteChats(queryClient, [
			makeChat("parent-1", { children: [child] }),
		]);

		expect(findChatInCaches(queryClient, "child-1")?.title).toBe("Child");
	});

	it("returns undefined when no cache holds the chat", () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, [makeChat("chat-2")]);

		expect(findChatInCaches(queryClient, "chat-1")).toBeUndefined();
	});
});

describe("cancelChatMutationRefetches", () => {
	it("cancels a loaded list refetch and the chat detail fetch", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { title: "cached" })]);
		queryClient.setQueryData(
			chatKeys.detail(chatId),
			makeChat(chatId, { title: "cached" }),
		);

		const listFetch = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "server" })]],
								pageParams: [0],
							}),
						50,
					);
				}),
		});
		const detailFetch = queryClient.prefetchQuery({
			queryKey: chatKeys.detail(chatId),
			queryFn: () =>
				new Promise<TypesGen.Chat>((resolve) => {
					setTimeout(() => resolve(makeChat(chatId, { title: "server" })), 50);
				}),
		});

		await cancelChatMutationRefetches(queryClient, chatId);
		await Promise.all([listFetch, detailFetch]);

		expect(readInfiniteChats(queryClient)?.[0].title).toBe("cached");
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))?.title,
		).toBe("cached");
	});

	it("leaves a never-loaded query alone so it cannot wedge at pending", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		const listFetch = queryClient.prefetchQuery({
			queryKey: infiniteChatsTestKey,
			queryFn: () =>
				new Promise<InfiniteData>((resolve) => {
					setTimeout(
						() =>
							resolve({
								pages: [[makeChat(chatId, { title: "first-load" })]],
								pageParams: [0],
							}),
						20,
					);
				}),
		});
		const detailFetch = queryClient.prefetchQuery({
			queryKey: chatKeys.detail(chatId),
			queryFn: () =>
				new Promise<TypesGen.Chat>((resolve) => {
					setTimeout(
						() => resolve(makeChat(chatId, { title: "first-load" })),
						20,
					);
				}),
		});

		await cancelChatMutationRefetches(queryClient, chatId);
		await Promise.all([listFetch, detailFetch]);

		expect(readInfiniteChats(queryClient)?.[0].title).toBe("first-load");
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId))?.title,
		).toBe("first-load");
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

	it("applies chat_summary_change even when event updated_at is older", () => {
		const cachedChat = makeChat("chat-1", {
			summary: null,
			last_turn_summary: "Latest turn",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			summary: "Implemented the whole-chat summary feature.",
			// chat_summary_change preserves updated_at, so the event carries an
			// equal-or-older timestamp than the cached chat.
			last_turn_summary: "Stale turn",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		const merged = mergeWatchedChatSummary(cachedChat, watchedChat, {
			eventKind: "chat_summary_change",
		});
		expect(merged.summary).toBe("Implemented the whole-chat summary feature.");
		// A chat_summary_change event must not clobber last_turn_summary with the
		// event's stale snapshot.
		expect(merged.last_turn_summary).toBe("Latest turn");
	});

	it("does not clobber the whole-chat summary on a summary_change with equal updated_at", () => {
		const cachedChat = makeChat("chat-1", {
			summary: "Whole-chat summary.",
			last_turn_summary: "Old turn",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		// Neither summary write bumps chats.updated_at, so both events replay
		// the triggering turn's timestamp and arrive with equal updated_at. The
		// summary_change snapshot still carries a stale whole-chat summary from
		// when the turn finished.
		const watchedChat = makeChat("chat-1", {
			summary: null,
			last_turn_summary: "New turn",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		const merged = mergeWatchedChatSummary(cachedChat, watchedChat, {
			eventKind: "summary_change",
		});
		expect(merged.last_turn_summary).toBe("New turn");
		expect(merged.summary).toBe("Whole-chat summary.");
	});

	it("does not clobber last_turn_summary on a chat_summary_change with equal updated_at", () => {
		const cachedChat = makeChat("chat-1", {
			summary: null,
			last_turn_summary: "Latest turn",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			summary: "Implemented the whole-chat summary feature.",
			last_turn_summary: "Stale turn",
			updated_at: "2025-01-01T00:00:00.000Z",
		});

		const merged = mergeWatchedChatSummary(cachedChat, watchedChat, {
			eventKind: "chat_summary_change",
		});
		expect(merged.summary).toBe("Implemented the whole-chat summary feature.");
		expect(merged.last_turn_summary).toBe("Latest turn");
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

	it("marks other chats unread on fresh status updates", () => {
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
		).toBe(true);
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
	it("merges last_model_config_id into the root list cache and per-chat cache", () => {
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
		queryClient.setQueryData(chatKeys.detail(chatId), cachedChat);

		mergeWatchedChatIntoCaches(queryClient, watchedChat, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId)),
		).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
	});

	it("merges last_model_config_id into the parent-embedded child snapshot and child cache", () => {
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
		queryClient.setQueryData(chatKeys.detail(childId), cachedChild);

		mergeWatchedChatIntoCaches(queryClient, watchedChild, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0].children?.[0]).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(childId)),
		).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
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
		queryClient.setQueryData(chatKeys.detail(chatId), cachedChat);

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
			queryClient.getQueryData<TypesGen.Chat>(chatKeys.detail(chatId)),
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

		expect(chatKeys.acl(chatId)).toEqual(["chats", "detail", chatId, "acl"]);
		expect(query.queryKey).toEqual(chatKeys.acl(chatId));
		await expect(query.queryFn()).resolves.toEqual(acl);
		expect(API.experimental.getChatACL).toHaveBeenCalledWith(chatId);
	});

	it("sets one chat user role and invalidates the ACL", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatKeys.acl(chatId), { users: [], groups: [] });
		vi.mocked(API.experimental.updateChatACL).mockResolvedValue();

		const mutation = setChatUserRole(queryClient);
		const variables = { chatId, userId: "user-1", role: "read" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
			user_roles: { "user-1": "read" },
		});

		await mutation.onSuccess?.(undefined, variables);
		expect(queryClient.getQueryState(chatKeys.acl(chatId))?.isInvalidated).toBe(
			true,
		);
	});

	it("sets one chat group role and invalidates the ACL", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatKeys.acl(chatId), { users: [], groups: [] });
		vi.mocked(API.experimental.updateChatACL).mockResolvedValue();

		const mutation = setChatGroupRole(queryClient);
		const variables = { chatId, groupId: "group-1", role: "" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
			group_roles: { "group-1": "" },
		});

		await mutation.onSuccess?.(undefined, variables);
		expect(queryClient.getQueryState(chatKeys.acl(chatId))?.isInvalidated).toBe(
			true,
		);
	});
});

describe("chat query freshness policy", () => {
	it("keeps the sidebar list finitely fresh and refetching on focus", () => {
		const query = infiniteChats({ archived: false });

		expect(query.staleTime).toBe(CHAT_SUMMARY_STALE_MS);
		expect(CHAT_SUMMARY_STALE_MS).toBeGreaterThan(0);
		expect(Number.isFinite(CHAT_SUMMARY_STALE_MS)).toBe(true);
		expect(query.refetchOnWindowFocus).toBe(true);
	});

	it("gives the chat detail the same finite freshness and focus refetch", () => {
		const query = chatDetail("chat-1");

		expect(query.staleTime).toBe(CHAT_SUMMARY_STALE_MS);
		expect(query.refetchOnWindowFocus).toBe(true);
	});

	it("never marks the per-chat message pages stale", () => {
		const query = chatMessagesForInfiniteScroll("chat-1");

		expect(query.staleTime).toBe(Number.POSITIVE_INFINITY);
	});

	it("leaves the shared chats prefix free of freshness overrides", () => {
		// staleTime lives on the three WebSocket-backed factories, not on
		// chatKeys.all, which also parents search, cost, and ACL queries.
		expect(chatSearch("title:fix")).not.toHaveProperty("staleTime");
		expect(chatACL("chat-1")).not.toHaveProperty("staleTime");
	});
});

describe(selectSortedChatList.name, () => {
	const chatAt = (id: string, updatedAt: string, pinOrder = 0): TypesGen.Chat =>
		makeChat(id, { updated_at: updatedAt, pin_order: pinOrder });

	it("orders pinned chats first by ascending pin_order", () => {
		const sorted = selectSortedChatList({
			pages: [
				[
					chatAt("unpinned", "2025-01-03T00:00:00.000Z"),
					chatAt("pinned-second", "2025-01-01T00:00:00.000Z", 2),
					chatAt("pinned-first", "2025-01-02T00:00:00.000Z", 1),
				],
			],
			pageParams: [0],
		});

		expect(sorted.map((chat) => chat.id)).toEqual([
			"pinned-first",
			"pinned-second",
			"unpinned",
		]);
	});

	it("promotes a chat whose updated_at moved across a page boundary", () => {
		const sorted = selectSortedChatList({
			pages: [
				[
					chatAt("page-1-newest", "2025-01-05T00:00:00.000Z"),
					chatAt("page-1-oldest", "2025-01-04T00:00:00.000Z"),
				],
				[chatAt("page-2-bumped", "2025-01-06T00:00:00.000Z")],
			],
			pageParams: [0, 2],
		});

		expect(sorted.map((chat) => chat.id)).toEqual([
			"page-2-bumped",
			"page-1-newest",
			"page-1-oldest",
		]);
	});

	it("breaks updated_at ties on descending id and leaves the cache pages alone", () => {
		const pages = [
			[
				chatAt("chat-a", "2025-01-01T00:00:00.000Z"),
				chatAt("chat-b", "2025-01-01T00:00:00.000Z"),
			],
		];
		const cached = { pages, pageParams: [0] };

		const sorted = selectSortedChatList(cached);

		expect(sorted.map((chat) => chat.id)).toEqual(["chat-b", "chat-a"]);
		expect(cached.pages[0].map((chat) => chat.id)).toEqual([
			"chat-a",
			"chat-b",
		]);
	});

	it("orders sub-second updated_at values by their fractional part", () => {
		const sorted = selectSortedChatList({
			pages: [
				[
					chatAt("older", "2025-01-01T00:00:00.000001Z"),
					chatAt("newer", "2025-01-01T00:00:00.000002Z"),
				],
			],
			pageParams: [0],
		});

		expect(sorted.map((chat) => chat.id)).toEqual(["newer", "older"]);
	});
});

describe(listFiltersFromKey.name, () => {
	it("round-trips the canonical filters a list key was built from", () => {
		const filters = {
			archived: false,
			prStatuses: ["open", "draft"] as const,
			chatStatus: "unread" as const,
			sources: ["created_by_me"] as const,
		};

		expect(listFiltersFromKey(chatKeys.list(filters))).toEqual(
			canonicalizeChatListFilters(filters),
		);
	});

	it("returns empty filters for the unfiltered list key", () => {
		expect(listFiltersFromKey(chatKeys.list())).toEqual({});
	});

	it("rejects keys that are not a single list variant", () => {
		expect(listFiltersFromKey(chatKeys.lists())).toBeUndefined();
		expect(listFiltersFromKey(chatKeys.detail("chat-1"))).toBeUndefined();
		expect(listFiltersFromKey(chatKeys.search("q"))).toBeUndefined();
	});
});

describe(invalidatePRStatusChatListQueries.name, () => {
	it("invalidates only the lists filtered by pull request status", async () => {
		const queryClient = createTestQueryClient();
		const prFiltered = { archived: false, prStatuses: ["open"] as const };
		const unfiltered = { archived: false };

		seedInfiniteChats(queryClient, [makeChat("chat-1")], prFiltered);
		seedInfiniteChats(queryClient, [makeChat("chat-1")], unfiltered);
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);

		await invalidatePRStatusChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatKeys.list(prFiltered))?.isInvalidated,
			"pr_status filtered lists should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.list(unfiltered))?.isInvalidated,
			"lists without a pr_status filter should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.list())?.isInvalidated,
			"the unfiltered list should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidates every cached pr_status variant", async () => {
		const queryClient = createTestQueryClient();
		const openOnly = { prStatuses: ["open"] as const };
		const mergedOnly = { archived: true, prStatuses: ["merged"] as const };

		seedInfiniteChats(queryClient, [makeChat("chat-1")], openOnly);
		seedInfiniteChats(queryClient, [makeChat("chat-1")], mergedOnly);

		await invalidatePRStatusChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatKeys.list(openOnly))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatKeys.list(mergedOnly))?.isInvalidated,
		).toBe(true);
	});
});

describe(createCoalescedChatListInvalidator.name, () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("collapses a burst of requests into one invalidation", () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const invalidator = createCoalescedChatListInvalidator(queryClient, 500);

		invalidator.schedule();
		vi.advanceTimersByTime(100);
		invalidator.schedule();
		vi.advanceTimersByTime(100);
		invalidator.schedule();

		expect(invalidateSpy).not.toHaveBeenCalled();

		vi.advanceTimersByTime(300);

		expect(invalidateSpy).toHaveBeenCalledTimes(1);
		expect(queryClient.getQueryState(chatKeys.list())?.isInvalidated).toBe(
			true,
		);
	});

	it("opens a new window after the previous one fired", () => {
		const queryClient = createTestQueryClient();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const invalidator = createCoalescedChatListInvalidator(queryClient, 500);

		invalidator.schedule();
		vi.advanceTimersByTime(500);
		invalidator.schedule();
		vi.advanceTimersByTime(500);

		expect(invalidateSpy).toHaveBeenCalledTimes(2);
	});

	it("drops a pending invalidation when cancelled", () => {
		const queryClient = createTestQueryClient();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
		const invalidator = createCoalescedChatListInvalidator(queryClient, 500);

		invalidator.schedule();
		invalidator.cancel();
		vi.advanceTimersByTime(1000);

		expect(invalidateSpy).not.toHaveBeenCalled();
	});
});

describe(clearChatUnreadInCaches.name, () => {
	it("removes the chat from unread-filtered lists", () => {
		const queryClient = createTestQueryClient();
		const unreadOnly = { archived: false, chatStatus: "unread" as const };
		seedInfiniteChats(
			queryClient,
			[
				makeChat("chat-1", { has_unread: true }),
				makeChat("chat-2", { has_unread: true }),
			],
			unreadOnly,
		);

		clearChatUnreadInCaches(queryClient, "chat-1");

		expect(
			readInfiniteChats(queryClient, unreadOnly)?.map((chat) => chat.id),
		).toEqual(["chat-2"]);
	});

	it("clears has_unread in the other loaded variants", () => {
		const queryClient = createTestQueryClient();
		const readOnly = { chatStatus: "read" as const };
		seedInfiniteChats(queryClient, [makeChat("chat-1", { has_unread: true })]);
		seedInfiniteChats(
			queryClient,
			[makeChat("chat-1", { has_unread: true })],
			readOnly,
		);

		clearChatUnreadInCaches(queryClient, "chat-1");

		expect(readInfiniteChats(queryClient)?.[0].has_unread).toBe(false);
		expect(readInfiniteChats(queryClient, readOnly)?.[0].has_unread).toBe(
			false,
		);
	});

	it("does not invalidate the lists it patched", () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, [makeChat("chat-1", { has_unread: true })]);

		clearChatUnreadInCaches(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatKeys.list())?.isInvalidated,
			"a refetch here can return has_unread:true and undo the clear",
		).not.toBe(true);
	});

	it("leaves unrelated chats and untouched caches by reference", () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, [makeChat("chat-2", { has_unread: true })]);
		const before = queryClient.getQueryData(chatKeys.list());

		clearChatUnreadInCaches(queryClient, "chat-1");

		expect(queryClient.getQueryData(chatKeys.list())).toBe(before);
	});
});
