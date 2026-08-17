import { QueryClient, QueryObserver } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatWatchEventKinds } from "#/api/typesGenerated";
import {
	ERROR_STATUSES,
	SUCCESS_STATUSES,
} from "#/pages/AgentsPage/components/RightPanel/DebugPanel/debugPanelUtils";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { buildOptimisticEditedMessage } from "./chatMessageEdits";
import {
	addChildToParentInCache,
	applyChatArchiveStateToCaches,
	applyWatchedChatArchived,
	applyWatchedChatCreatedOrUnarchived,
	archiveChat,
	type ChatListInput,
	cancelChatEntity,
	cancelChatListQueries,
	cancelChatListRefetches,
	cancelChatMessages,
	cancelLoadedChatEntityRefetch,
	chatACL,
	chatACLKey,
	chatAdvisorConfig,
	chatAdvisorConfigKey,
	chatCost,
	chatCostTreeKey,
	chatDebugRunKey,
	chatDebugRunsKey,
	chatDiffContentsKey,
	chatEntitiesFamilyKey,
	chatEntityKey,
	chatListFamilyKey,
	chatListKey,
	chatMessagesKey,
	chatPromptsKey,
	chatSearch,
	chatsByWorkspace,
	createChat,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	getChatListQueryString,
	infiniteChats,
	interruptChat,
	invalidateChatACL,
	invalidateChatCostTree,
	invalidateChatDebugRuns,
	invalidateChatDiffContents,
	invalidateChatEntity,
	invalidateChatListQueries,
	invalidateChatMessages,
	invalidateChatPrompts,
	invalidateChatSearches,
	invalidateChatsByWorkspace,
	mergeWatchedChatIntoCaches,
	mergeWatchedChatSummary,
	patchChatEntity,
	patchChatMessages,
	pinChat,
	prependToInfiniteChatsCache,
	promoteChatQueuedMessage,
	proposeChatTitle,
	removeChatEntity,
	removeChatFromChatsByWorkspace,
	removeChildFromParentInCache,
	reorderPinnedChat,
	replaceChatMessagesHistory,
	resetUnloadedChatEntity,
	setChatGroupRole,
	setChatUserRole,
	shouldInvalidateChatSearches,
	shouldInvalidateChatsByWorkspace,
	TERMINAL_RUN_STATUSES,
	toChatListParams,
	unarchiveChat,
	unpinChat,
	updateChatAdvisorConfig,
	updateChatPlanMode,
	updateChatTitle,
	updateChatWorkspace,
	updateChildInParentCache,
	updateInfiniteChatsCache,
	upsertChatMessages,
} from "./chats";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			updateChat: vi.fn(),
			createChat: vi.fn(),
			deleteChatQueuedMessage: vi.fn(),
			getChats: vi.fn(),
			getChatsByWorkspace: vi.fn(),
			getChatCost: vi.fn(),
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

type InfiniteChatsTestOptions = ChatListInput;

const infiniteChatsTestKey = chatListKey(toChatListParams());

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
	queryClient.setQueryData<InfiniteData>(chatListKey(toChatListParams(opts)), {
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
		chatListKey(toChatListParams(opts)),
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
		},
	});

const observeChatWithDeferredFirstFetch = (
	queryClient: QueryClient,
	staleChat: TypesGen.Chat,
	durableChat: TypesGen.Chat,
) => {
	const firstFetch = createDeferred<TypesGen.Chat>();
	const durableResult = createDeferred<TypesGen.Chat>();
	let fetchCount = 0;
	const observer = new QueryObserver<TypesGen.Chat>(queryClient, {
		queryKey: chatEntityKey(staleChat.id),
		queryFn: () => {
			fetchCount++;
			return fetchCount === 1 ? firstFetch.promise : durableChat;
		},
	});
	const unsubscribe = observer.subscribe((result) => {
		if (result.data === durableChat) {
			durableResult.resolve(result.data);
		}
	});
	return {
		durableResult,
		firstFetch,
		fetchCount: () => fetchCount,
		unsubscribe,
	};
};

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
	it("invalidates flat and infinite chat list queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(chatListKey(toChatListParams()), {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		queryClient.setQueryData(
			chatListKey(toChatListParams({ archived: true })),
			{
				pages: [[makeChat(chatId)]],
				pageParams: [0],
			},
		);
		// Per-chat queries that should NOT be touched.
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), {});

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatListKey(toChatListParams()))?.isInvalidated,
			"default chat list should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(
				chatListKey(toChatListParams({ archived: true })),
			)?.isInvalidated,
			"archived chat list should be invalidated",
		).toBe(true);

		// Per-chat queries should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatEntityKey(chatId))?.isInvalidated,
			"chatEntityKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatDiffContentsKey(chatId))?.isInvalidated,
			"chatDiffContentsKey should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidates the list query built from default params", async () => {
		const queryClient = createTestQueryClient();

		queryClient.setQueryData(chatListKey(toChatListParams()), {
			pages: [[makeChat("chat-1")]],
			pageParams: [0],
		});

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatListKey(toChatListParams()))?.isInvalidated,
			"default params chat list should be invalidated",
		).toBe(true);
	});

	it("does not invalidate a different chat's queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const otherChatId = "chat-2";

		queryClient.setQueryData(chatListKey(toChatListParams()), [
			makeChat(chatId),
		]);
		queryClient.setQueryData(chatEntityKey(otherChatId), makeChat(otherChatId));
		queryClient.setQueryData(chatMessagesKey(otherChatId), []);

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatEntityKey(otherChatId))?.isInvalidated,
			"other chat's chatEntityKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey(otherChatId))?.isInvalidated,
			"other chat's chatMessagesKey should NOT be invalidated",
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
			chatEntityKey(chatId),
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
		mutation.onSuccess(undefined, { chatId, title: "New" });

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.title,
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
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey(chatId),
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
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));

		vi.mocked(API.experimental.updateChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const cachedChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		expect(cachedChat?.archived).toBe(true);
	});

	it("strips an individually-archived child from its parent's embedded children", async () => {
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

		vi.mocked(API.experimental.updateChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate("child-1");

		const result = readInfiniteChats(queryClient);
		expect(result?.[0].children).toHaveLength(1);
		expect(result?.[0].children?.[0].id).toBe("child-2");
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
			chatEntityKey(chatId),
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
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId)),
		).toMatchObject({
			archived: true,
			pin_order: 0,
		});
	});

	it("clears pin order for archived chats that remain in archived lists", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })], {
			archived: true,
		});

		const mutation = archiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		expect(
			readInfiniteChats(queryClient, { archived: true })?.[0],
		).toMatchObject({
			archived: true,
			pin_order: 0,
		});
	});

	it("removes newly archived chats from lists filtered to active chats", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })], {
			archived: false,
		});

		const mutation = archiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		expect(readInfiniteChats(queryClient, { archived: false })).toEqual([]);
	});

	it("removes loaded search rows after success", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const unrelatedRow = makeChat("chat-2");
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, [
			makeChat(chatId, { pin_order: 2 }),
			unrelatedRow,
		]);

		const mutation = archiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		const rows = queryClient.getQueryData<TypesGen.Chat[]>(
			chatSearch({ q: "alpha" }).queryKey,
		);
		expect(rows?.find((row) => row.id === chatId)).toBeUndefined();
		expect(rows?.find((row) => row.id === "chat-2")).toEqual(unrelatedRow);
	});

	it("rolls back the chats list on error by invalidating", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialChats = [makeChat(chatId)];
		seedInfiniteChats(queryClient, initialChats);
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify the optimistic update took effect.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);

		// Simulate an error, the onError handler invalidates the
		// cache so a re-fetch restores the correct state.
		mutation.onError(new Error("server error"), chatId, context);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
	});

	it("rolls back the individual chat cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
		).toBe(true);

		mutation.onError(new Error("server error"), chatId, context);

		const rolledBack = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
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
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
	});

	it("handles onMutate when no individual chat cache exists", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);

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
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey(chatId),
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
			chatEntityKey(chatId),
			makeChat(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
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
			chatEntityKey(chatId),
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
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId)),
		).toMatchObject({
			archived: false,
		});
	});

	it("removes loaded search rows after success", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const unrelatedRow = makeChat("chat-2", { archived: true });
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, [
			makeChat(chatId, { archived: true }),
			unrelatedRow,
		]);

		const mutation = unarchiveChat(queryClient);
		mutation.onSuccess(undefined, chatId);

		const rows = queryClient.getQueryData<TypesGen.Chat[]>(
			chatSearch({ q: "alpha" }).queryKey,
		);
		expect(rows?.find((row) => row.id === chatId)).toBeUndefined();
		expect(rows?.find((row) => row.id === "chat-2")).toEqual(unrelatedRow);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chatEntityKey(chatId),
			makeChat(chatId, { archived: true }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unarchiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(false);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
		).toBe(false);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// The chats list is rolled back via invalidation.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		// The individual chat cache is restored directly.
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
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
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey(chatId),
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
		queryClient.setQueryData(
			chatListKey(toChatListParams({ archived: true })),
			{
				pages: [[makeChat("chat-pinned-archived", { pin_order: 4 })]],
				pageParams: [0],
			},
		);
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));

		const mutation = pinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)
				?.pin_order,
		).toBe(5);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.pin_order,
		).toBe(5);
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
			chatEntityKey(chatId),
			makeChat(chatId, { pin_order: 2 }),
		);

		const mutation = unpinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.pin_order,
		).toBe(0);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })]);
		queryClient.setQueryData(
			chatEntityKey(chatId),
			makeChat(chatId, { pin_order: 3 }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].pin_order).toBe(0);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.pin_order,
		).toBe(0);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// The chats list is rolled back via invalidation.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		// The individual chat cache is restored directly.
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.pin_order,
		).toBe(3);
	});

	it("invalidates queries on settled", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey(chatId),
			exact: true,
		});
	});
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
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey(chatId),
			exact: true,
		});
		expect(API.experimental.updateChat).toHaveBeenCalledWith(chatId, {
			pin_order: 2,
		});
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatListFamilyKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey(chatId),
			exact: true,
		});
	});
});

describe("chat cost query factories", () => {
	it("builds the per-chat cost query key and forwards the chat id", async () => {
		const chatId = "chat-1";
		vi.mocked(API.experimental.getChatCost).mockResolvedValue(
			{} as TypesGen.ChatCost,
		);

		const query = chatCost(chatId);

		expect(chatCostTreeKey(chatId)).toEqual([
			"chats",
			"analytics",
			"cost",
			"tree",
			chatId,
		]);
		expect(query.queryKey).toEqual([
			"chats",
			"analytics",
			"cost",
			"tree",
			chatId,
		]);
		await query.queryFn();
		expect(API.experimental.getChatCost).toHaveBeenCalledWith(chatId);
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
		queryClient.setQueryData(chatListKey(toChatListParams()), {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDebugRunsKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });
	};

	/** Keys that should NEVER be invalidated by chat message mutations
	 *  because they are completely unrelated to the message flow. */
	const unrelatedKeys = (chatId: string) => [
		{ label: "diff-contents", key: chatDiffContentsKey(chatId) },
	];

	it("createChatMessage does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, []);

		const mutation = createChatMessage(queryClient, chatId);
		await mutation.onSuccess?.();

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by createChatMessage`,
			).not.toBe(true);
		}
		// The send path invalidates searches through
		// useChatStore.upsertCacheMessages; doing it here too would
		// double-invalidate every send.
		expect(
			queryClient.getQueryState(chatSearch({ q: "alpha" }).queryKey)
				?.isInvalidated,
			"chat searches should NOT be invalidated by createChatMessage",
		).not.toBe(true);
	});

	it("createChatMessage invalidates debug runs and chat detail, not messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChatMessage(queryClient, chatId);
		await mutation.onSuccess?.();

		expect(
			queryClient.getQueryState(chatDebugRunsKey(chatId))?.isInvalidated,
			"chatDebugRunsKey should be invalidated",
		).toBe(true);

		const chatState = queryClient.getQueryState(chatEntityKey(chatId));
		expect(
			chatState?.isInvalidated,
			"chatEntityKey should be invalidated",
		).toBe(true);

		const messagesState = queryClient.getQueryState(chatMessagesKey(chatId));
		expect(
			messagesState?.isInvalidated,
			"chatMessagesKey should NOT be invalidated",
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
		const chatState = queryClient.getQueryState(chatEntityKey(chatId));
		expect(
			chatState?.isInvalidated,
			"chatEntityKey should be invalidated",
		).toBe(true);

		// Messages are NOT invalidated. The per-chat WebSocket handles
		// post-edit message delivery, making REST invalidation
		// unnecessary.
		const messagesState = queryClient.getQueryState(chatMessagesKey(chatId));
		expect(
			messagesState?.isInvalidated,
			"chatMessagesKey should not be invalidated",
		).not.toBe(true);

		expect(
			queryClient.getQueryState(chatDebugRunsKey(chatId))?.isInvalidated,
			"chatDebugRunsKey should be invalidated",
		).toBe(true);
	});

	it("editChatMessage onError invalidates messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
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

		const messagesState = queryClient.getQueryState(chatMessagesKey(chatId));
		expect(
			messagesState?.isInvalidated,
			"chatMessagesKey should be invalidated on error",
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

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
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

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
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

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		expect(data?.pages[0]?.queued_messages).toEqual([]);
	});

	it("editChatMessage restores cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(
			requireMessage(messages, 3),
		);

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
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
			queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId))?.pages[0]
				?.messages,
		).toHaveLength(3);

		mutation.onError(
			new Error("network failure"),
			{ messageId: 3, optimisticMessage, req: editReq },
			context,
		);

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
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

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
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
			chatMessagesKey(chatId),
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

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
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
		expect(queryClient.getQueryData(chatMessagesKey(chatId))).toBeUndefined();
	});

	it("editChatMessage onError handles undefined context gracefully", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
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
		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([3, 2, 1]);

		await new Promise((r) => setTimeout(r, 0));
		const messagesState = queryClient.getQueryState(chatMessagesKey(chatId));
		expect(
			messagesState?.isInvalidated,
			"chatMessagesKey should be invalidated even without context",
		).toBe(true);
	});

	it("editChatMessage onMutate updates the first page and preserves older pages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Page 0 (newest): IDs 10 to 6. Page 1 (older): IDs 5 to 1.
		const page0 = [10, 9, 8, 7, 6].map((id) => makeMsg(chatId, id));
		const page1 = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));
		const optimisticMessage = buildOptimisticMessage(requireMessage(page0, 7));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
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

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
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

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 1,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
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

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({
			messageId: 5,
			optimisticMessage,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
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
			queryClient.getQueryState(chatDebugRunsKey(chatId))?.isInvalidated,
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
		await mutation.onSuccess?.();

		expect(
			queryClient.getQueryState(chatDebugRunsKey(chatId))?.isInvalidated,
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
				queryClient.getQueryState(chatDebugRunsKey(chatId))?.isInvalidated,
				"chatDebugRunsKey should be invalidated",
			).toBe(true);

			for (const { label, key } of [
				{
					label: "chat list",
					key: chatListKey(toChatListParams()),
				},
				{ label: "chat detail", key: chatEntityKey(chatId) },
				{ label: "messages", key: chatMessagesKey(chatId) },
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
			queryClient.getQueryState(chatListKey(toChatListParams()))?.isInvalidated,
			"chat list should be invalidated",
		).toBe(true);

		// Per-chat queries should NOT be touched.
		for (const { label, key } of unrelatedKeys(chatId)) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} should NOT be invalidated by createChat`,
			).not.toBe(true);
		}
		expect(
			queryClient.getQueryState(chatEntityKey(chatId))?.isInvalidated,
			"chatEntityKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey should NOT be invalidated",
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
			queryClient.getQueryState(chatEntityKey(chatId))?.isInvalidated,
			"chatEntityKey should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey should be invalidated",
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
			queryClient.getQueryState(chatListKey(toChatListParams()))?.isInvalidated,
			"chat list should NOT be invalidated",
		).not.toBe(true);
	});

	it.each<{
		name: string;
		settle: (queryClient: QueryClient) => unknown;
	}>([
		{
			name: "archiveChat onSettled",
			settle: (queryClient) =>
				archiveChat(queryClient).onSettled(undefined, undefined, "chat-1"),
		},
		{
			name: "unarchiveChat onSettled",
			settle: (queryClient) =>
				unarchiveChat(queryClient).onSettled(undefined, undefined, "chat-1"),
		},
		{
			name: "updateChatTitle onSettled",
			settle: (queryClient) =>
				updateChatTitle(queryClient).onSettled(undefined, undefined, {
					chatId: "chat-1",
					title: "New",
				}),
		},
		{
			name: "editChatMessage onSettled",
			settle: (queryClient) =>
				editChatMessage(queryClient, "chat-1").onSettled(),
		},
		{
			name: "createChat onSuccess",
			settle: (queryClient) => createChat(queryClient).onSuccess(),
		},
	])("$name invalidates chat searches", async ({ settle }) => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, []);

		settle(queryClient);
		await new Promise((r) => setTimeout(r, 0));

		expect(
			queryClient.getQueryState(chatSearch({ q: "alpha" }).queryKey)
				?.isInvalidated,
			"chat search entry should be invalidated",
		).toBe(true);
	});

	it.each<{
		name: string;
		settle: (queryClient: QueryClient) => unknown;
	}>([
		{
			name: "archiveChat onSettled",
			settle: (queryClient) =>
				archiveChat(queryClient).onSettled(undefined, undefined, "chat-1"),
		},
		{
			name: "unarchiveChat onSettled",
			settle: (queryClient) =>
				unarchiveChat(queryClient).onSettled(undefined, undefined, "chat-1"),
		},
		{
			name: "updateChatWorkspace onSettled",
			settle: (queryClient) =>
				updateChatWorkspace(queryClient).onSettled(undefined, undefined, {
					chatId: "chat-1",
					workspaceId: "ws-1",
				}),
		},
		{
			name: "createChat onSuccess",
			settle: (queryClient) => createChat(queryClient).onSuccess(),
		},
	])("$name invalidates chats by workspace", async ({ settle }) => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-1",
		});

		settle(queryClient);
		await new Promise((r) => setTimeout(r, 0));

		expect(
			queryClient.getQueryState(chatsByWorkspace(["ws-1"]).queryKey)
				?.isInvalidated,
			"by-workspace entry should be invalidated",
		).toBe(true);
	});

	it("archiveChat onSuccess synchronously removes the chat's by-workspace mappings", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-1",
			"ws-2": "chat-2",
		});

		archiveChat(queryClient).onSuccess(undefined, "chat-1");

		// Assert before any timer flush: the archived chat must be gone
		// from the mapping without waiting for the onSettled refetch.
		expect(
			queryClient.getQueryData(chatsByWorkspace(["ws-1"]).queryKey),
		).toEqual({ "ws-2": "chat-2" });
	});

	it.each<{
		name: string;
		settle: (queryClient: QueryClient) => unknown;
	}>([
		{
			name: "updateChatTitle onSettled",
			settle: (queryClient) =>
				updateChatTitle(queryClient).onSettled(undefined, undefined, {
					chatId: "chat-1",
					title: "New",
				}),
		},
		{
			name: "createChatMessage onSuccess",
			settle: (queryClient) =>
				createChatMessage(queryClient, "chat-1").onSuccess?.(),
		},
	])("$name does not invalidate chats by workspace", async ({ settle }) => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-1",
		});

		settle(queryClient);
		await new Promise((r) => setTimeout(r, 0));

		expect(
			queryClient.getQueryState(chatsByWorkspace(["ws-1"]).queryKey)
				?.isInvalidated,
			"by-workspace entry should NOT be invalidated",
		).not.toBe(true);
	});
});

describe("chatListKey shape", () => {
	it("places the params object one slot after the list family prefix", () => {
		const key = chatListKey(toChatListParams({ archived: true }));
		expect(key.length).toBe(chatListFamilyKey.length + 1);
		expect(key[chatListFamilyKey.length]).toEqual({
			archived: true,
			prStatuses: [],
			status: "all",
			sources: [],
		});
	});
});

describe("getChatListQueryString", () => {
	it("emits sidebar query shapes accepted by searchquery.Chats", () => {
		// These strings must match TestSearchChatsFrontendEmitted in
		// coderd/searchquery/search_test.go.
		expect(getChatListQueryString(toChatListParams())).toBe("archived:false");
		expect(
			getChatListQueryString(toChatListParams({ chatStatus: "unread" })),
		).toBe("archived:false has_unread:true");
		expect(
			getChatListQueryString(
				toChatListParams({
					prStatuses: ["draft", "closed"],
					sources: ["created_by_me", "shared_with_me"],
				}),
			),
		).toBe(
			"archived:false pr_status:draft,closed source:created_by_me,shared_with_me",
		);
	});
});

describe("chatsByWorkspace", () => {
	it("disables the query when no workspace IDs are given", () => {
		expect(chatsByWorkspace([]).enabled).toBe(false);
		expect(chatsByWorkspace(["ws-1"]).enabled).toBe(true);
	});

	it("canonicalizes the key with sorted, deduplicated workspace IDs", () => {
		const options = chatsByWorkspace(["ws-b", "ws-a", "ws-b"]);
		expect(options.queryKey).toEqual([
			"chats",
			"collections",
			"by-workspace",
			["ws-a", "ws-b"],
		]);
	});

	it("fetches with the sorted, deduplicated workspace IDs", async () => {
		const getChatsByWorkspace = vi.mocked(API.experimental.getChatsByWorkspace);
		getChatsByWorkspace.mockResolvedValue({});
		await chatsByWorkspace(["ws-b", "ws-a", "ws-b"]).queryFn();
		expect(getChatsByWorkspace).toHaveBeenCalledWith(["ws-a", "ws-b"]);
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
				q: "archived:false",
			});
		});

		it("computes offset 0 for pageParam <= 0", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats();
			await queryFn({ pageParam: -1 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: 0,
				q: "archived:false",
			});
		});

		it("computes correct offset for subsequent pages", async () => {
			vi.mocked(API.experimental.getChats).mockResolvedValue([]);
			const { queryFn } = infiniteChats();

			await queryFn({ pageParam: 2 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: PAGE_LIMIT,
				q: "archived:false",
			});

			await queryFn({ pageParam: 3 });
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: PAGE_LIMIT,
				offset: PAGE_LIMIT * 2,
				q: "archived:false",
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
		const query = chatSearch({ q: "title:fix" });
		const queryClient = createTestQueryClient();

		expect(query.queryKey).toEqual([
			"chats",
			"collections",
			"search",
			{ q: "title:fix" },
		]);
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

	it("exact chatEntityKey invalidation does not cascade to messages or diff-contents", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Seed all the queries that are active on the /agents/:id page.
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });
		queryClient.setQueryData(chatListKey(toChatListParams()), [
			makeChat(chatId),
		]);

		// This is what the fixed handler does, exact: true.
		await queryClient.invalidateQueries({
			queryKey: chatEntityKey(chatId),
			exact: true,
		});

		expect(
			queryClient.getQueryState(chatEntityKey(chatId))?.isInvalidated,
			"chatEntityKey should be invalidated",
		).toBe(true);

		// Messages should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey should NOT be invalidated by exact chatEntityKey",
		).not.toBe(true);

		// Diff-contents should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatDiffContentsKey(chatId))?.isInvalidated,
			"chatDiffContentsKey should NOT be invalidated by exact chatEntityKey",
		).not.toBe(true);

		// Chat list should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatListKey(toChatListParams()))?.isInvalidated,
			"chatListKey should NOT be invalidated by exact chatEntityKey",
		).not.toBe(true);
	});

	it("without exact: true, chatEntityKey invalidation cascades to messages and diff-contents (the old bug)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });

		// This is what the OLD (broken) handler did, no exact: true.
		await queryClient.invalidateQueries({
			queryKey: chatEntityKey(chatId),
		});

		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey IS invalidated without exact: true (old bug)",
		).toBe(true);

		expect(
			queryClient.getQueryState(chatDiffContentsKey(chatId))?.isInvalidated,
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

	it("keeps the repaired build_id when the event snapshot carries a stale binding", () => {
		const cachedChat = makeChat("chat-1", {
			workspace_id: "workspace-1",
			agent_id: "agent-new",
			build_id: "build-new",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			workspace_id: "workspace-1",
			agent_id: "agent-old",
			build_id: "build-old",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		const merged = mergeWatchedChatSummary(cachedChat, watchedChat, {
			eventKind: "diff_status_change",
		});
		expect(merged.build_id).toBe("build-new");
		expect(merged.agent_id).toBe("agent-new");
	});

	it("adopts a fresh build_id when the event snapshot agrees on the agent", () => {
		const cachedChat = makeChat("chat-1", {
			workspace_id: "workspace-1",
			agent_id: "agent-1",
			build_id: "build-old",
			updated_at: "2025-01-01T00:00:00.000Z",
		});
		const watchedChat = makeChat("chat-1", {
			workspace_id: "workspace-1",
			agent_id: "agent-1",
			build_id: "build-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});

		expect(
			mergeWatchedChatSummary(cachedChat, watchedChat, {
				eventKind: "status_change",
			}).build_id,
		).toBe("build-new");
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
		queryClient.setQueryData(chatEntityKey(chatId), cachedChat);

		mergeWatchedChatIntoCaches(queryClient, watchedChat, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId)),
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
		queryClient.setQueryData(chatEntityKey(childId), cachedChild);

		mergeWatchedChatIntoCaches(queryClient, watchedChild, {
			eventKind: "status_change",
		});

		expect(readInfiniteChats(queryClient)?.[0].children?.[0]).toMatchObject({
			status: "running",
			last_model_config_id: "model-new",
			updated_at: "2025-01-01T00:05:00.000Z",
		});
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(childId)),
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
		queryClient.setQueryData(chatEntityKey(chatId), cachedChat);

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
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId)),
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

		expect(chatACLKey(chatId)).toEqual(["chats", "entities", chatId, "acl"]);
		expect(query.queryKey).toEqual(chatACLKey(chatId));
		await expect(query.queryFn()).resolves.toEqual(acl);
		expect(API.experimental.getChatACL).toHaveBeenCalledWith(chatId);
	});

	it("sets one chat user role and invalidates the ACL", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatACLKey(chatId), { users: [], groups: [] });
		vi.mocked(API.experimental.updateChatACL).mockResolvedValue();

		const mutation = setChatUserRole(queryClient);
		const variables = { chatId, userId: "user-1", role: "read" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
			user_roles: { "user-1": "read" },
		});

		await mutation.onSuccess?.(undefined, variables);
		expect(queryClient.getQueryState(chatACLKey(chatId))?.isInvalidated).toBe(
			true,
		);
	});

	it("sets one chat group role and invalidates the ACL", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatACLKey(chatId), { users: [], groups: [] });
		vi.mocked(API.experimental.updateChatACL).mockResolvedValue();

		const mutation = setChatGroupRole(queryClient);
		const variables = { chatId, groupId: "group-1", role: "" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
			group_roles: { "group-1": "" },
		});

		await mutation.onSuccess?.(undefined, variables);
		expect(queryClient.getQueryState(chatACLKey(chatId))?.isInvalidated).toBe(
			true,
		);
	});
});

describe("semantic cache operations: exact invalidations", () => {
	it("invalidateChatEntity touches only the detail entry", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		queryClient.setQueryData(chatMessagesKey("chat-1"), []);
		queryClient.setQueryData(chatEntityKey("chat-2"), makeChat("chat-2"));

		await invalidateChatEntity(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey("chat-1"))?.isInvalidated,
			"messages entry should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-2"))?.isInvalidated,
			"other chat's detail entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidateChatDiffContents touches only the diff-contents entry", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatDiffContentsKey("chat-1"), { files: [] });
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		await invalidateChatDiffContents(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatDiffContentsKey("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
			"detail entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidateChatPrompts touches only the prompts entry", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatPromptsKey("chat-1"), { prompts: [] });
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		await invalidateChatPrompts(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatPromptsKey("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
			"detail entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidateChatMessages touches only the messages entry", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatMessagesKey("chat-1"), []);
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		await invalidateChatMessages(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatMessagesKey("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
			"detail entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidateChatACL touches only the ACL entry", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatACLKey("chat-1"), {});
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		await invalidateChatACL(queryClient, "chat-1");

		expect(queryClient.getQueryState(chatACLKey("chat-1"))?.isInvalidated).toBe(
			true,
		);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
			"detail entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidateChatCostTree touches only the matching cost tree entry", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatCostTreeKey("chat-1"), {});
		queryClient.setQueryData(chatCostTreeKey("chat-2"), {});
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		await invalidateChatCostTree(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatCostTreeKey("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatCostTreeKey("chat-2"))?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
			"detail entry should NOT be invalidated",
		).not.toBe(true);
	});
});

describe("semantic cache operations: prefix invalidations", () => {
	it("invalidateChatListQueries touches every list entry and nothing outside the family", async () => {
		const queryClient = createTestQueryClient();
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);
		seedInfiniteChats(queryClient, [makeChat("chat-1")], { archived: true });
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {});

		await invalidateChatListQueries(queryClient);

		expect(queryClient.getQueryState(infiniteChatsTestKey)?.isInvalidated).toBe(
			true,
		);
		expect(
			queryClient.getQueryState(
				chatListKey(toChatListParams({ archived: true })),
			)?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatsByWorkspace(["ws-1"]).queryKey)
				?.isInvalidated,
		).not.toBe(true);
	});

	it("invalidateChatsByWorkspace touches by-workspace entries only", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {});
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		await invalidateChatsByWorkspace(queryClient);

		expect(
			queryClient.getQueryState(chatsByWorkspace(["ws-1"]).queryKey)
				?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(infiniteChatsTestKey)?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
		).not.toBe(true);
	});

	describe(shouldInvalidateChatsByWorkspace.name, () => {
		// created/deleted have their own watch branches; title, summary,
		// diff, and context events do not move updated_at ordering.
		const expectedByKind: Record<TypesGen.ChatWatchEventKind, boolean> = {
			action_required: true,
			chat_summary_change: false,
			context_dirty: false,
			created: false,
			deleted: false,
			diff_status_change: false,
			status_change: true,
			summary_change: false,
			title_change: false,
		};

		it.each(ChatWatchEventKinds)("%s", (kind) => {
			expect(shouldInvalidateChatsByWorkspace(kind)).toBe(expectedByKind[kind]);
		});
	});

	it("invalidateChatDebugRuns touches the runs list and run details only", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatDebugRunsKey("chat-1"), []);
		queryClient.setQueryData(chatDebugRunKey("chat-1", "run-1"), {});
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		queryClient.setQueryData(chatMessagesKey("chat-1"), []);

		await invalidateChatDebugRuns(queryClient, "chat-1");

		expect(
			queryClient.getQueryState(chatDebugRunsKey("chat-1"))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatDebugRunKey("chat-1", "run-1"))
				?.isInvalidated,
			"run detail entry should be invalidated by the family prefix",
		).toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
			"detail entry should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey("chat-1"))?.isInvalidated,
			"messages entry should NOT be invalidated",
		).not.toBe(true);
	});

	it("invalidateChatSearches touches every search entry and nothing outside the family", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, []);
		queryClient.setQueryData(chatSearch({ q: "beta" }).queryKey, []);
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {});
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		queryClient.setQueryData(chatMessagesKey("chat-1"), []);
		queryClient.setQueryData(chatCostTreeKey("chat-1"), {});

		await invalidateChatSearches(queryClient);

		expect(
			queryClient.getQueryState(chatSearch({ q: "alpha" }).queryKey)
				?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatSearch({ q: "beta" }).queryKey)
				?.isInvalidated,
		).toBe(true);
		for (const [label, key] of [
			["chat list", infiniteChatsTestKey],
			["by-workspace", chatsByWorkspace(["ws-1"]).queryKey],
			["chat detail", chatEntityKey("chat-1")],
			["messages", chatMessagesKey("chat-1")],
			["cost tree", chatCostTreeKey("chat-1")],
		] as const) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} entry should NOT be invalidated`,
			).not.toBe(true);
		}
	});

	describe(shouldInvalidateChatSearches.name, () => {
		// Search results render title, status, diff status, and the
		// action-required badge. Summary and context events are excluded:
		// stale last_turn_summary subtitles are accepted until
		// reconciliation lands. The created and deleted kinds are handled
		// by their own watch branches before the merge path runs.
		const expectedByKind: Record<TypesGen.ChatWatchEventKind, boolean> = {
			action_required: true,
			chat_summary_change: false,
			context_dirty: false,
			created: false,
			deleted: false,
			diff_status_change: true,
			status_change: true,
			summary_change: false,
			title_change: true,
		};

		it.each(ChatWatchEventKinds)("%s", (kind) => {
			expect(shouldInvalidateChatSearches(kind)).toBe(expectedByKind[kind]);
		});
	});
});

describe("semantic cache operations: cancellation", () => {
	it("cancelChatListQueries cancels unconditionally across the list family", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await cancelChatListQueries(queryClient);

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatListFamilyKey,
		});
	});

	it("cancelChatEntity cancels the exact detail entry unconditionally", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await cancelChatEntity(queryClient, "chat-1");

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey("chat-1"),
			exact: true,
		});
	});

	it("cancelLoadedChatEntityRefetch is a no-op when detail data is absent", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await cancelLoadedChatEntityRefetch(queryClient, "chat-1");

		expect(cancelSpy).not.toHaveBeenCalled();
	});

	it("cancelLoadedChatEntityRefetch cancels exactly when detail data exists", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await cancelLoadedChatEntityRefetch(queryClient, "chat-1");

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey("chat-1"),
			exact: true,
		});
	});

	it("resetUnloadedChatEntity resets exactly when detail data is absent", async () => {
		const queryClient = createTestQueryClient();
		const resetSpy = vi.spyOn(queryClient, "resetQueries");

		await resetUnloadedChatEntity(queryClient, "chat-1");

		expect(resetSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey("chat-1"),
			exact: true,
		});
	});

	it("resetUnloadedChatEntity is a no-op when detail data exists", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		const resetSpy = vi.spyOn(queryClient, "resetQueries");

		await resetUnloadedChatEntity(queryClient, "chat-1");

		expect(resetSpy).not.toHaveBeenCalled();
	});

	it("cancelChatMessages cancels the exact messages entry", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await cancelChatMessages(queryClient, "chat-1");

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatMessagesKey("chat-1"),
			exact: true,
		});
	});
});

describe("semantic cache operations: removal and patching", () => {
	it("removeChatEntity removes only the exact detail entry", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		queryClient.setQueryData(chatMessagesKey("chat-1"), []);
		queryClient.setQueryData(chatPromptsKey("chat-1"), { prompts: [] });
		queryClient.setQueryData(chatACLKey("chat-1"), {});
		queryClient.setQueryData(chatDiffContentsKey("chat-1"), { files: [] });
		queryClient.setQueryData(chatDebugRunsKey("chat-1"), []);
		queryClient.setQueryData(chatDebugRunKey("chat-1", "run-1"), {});
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);

		removeChatEntity(queryClient, "chat-1");

		expect(queryClient.getQueryData(chatEntityKey("chat-1"))).toBeUndefined();
		for (const [label, key] of [
			["messages", chatMessagesKey("chat-1")],
			["prompts", chatPromptsKey("chat-1")],
			["acl", chatACLKey("chat-1")],
			["diff-contents", chatDiffContentsKey("chat-1")],
			["debug-runs", chatDebugRunsKey("chat-1")],
			["debug-run detail", chatDebugRunKey("chat-1", "run-1")],
		] as const) {
			expect(
				queryClient.getQueryData(key),
				`${label} entry should survive removeChatEntity`,
			).toBeDefined();
		}
		expect(
			queryClient.getQueryData(infiniteChatsTestKey),
			"list entry should survive removeChatEntity",
		).toBeDefined();
	});

	it("patchChatEntity applies the updater to the exact detail entry", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		patchChatEntity(queryClient, "chat-1", (chat) =>
			chat ? { ...chat, title: "Patched" } : chat,
		);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey("chat-1"))?.title,
		).toBe("Patched");
	});

	it("patchChatMessages preserves the previous reference when the updater is a no-op", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatMessagesKey("chat-1"), {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});
		const before = queryClient.getQueryData(chatMessagesKey("chat-1"));

		patchChatMessages(queryClient, "chat-1", (data) => data);

		expect(queryClient.getQueryData(chatMessagesKey("chat-1"))).toBe(before);
	});

	it("removeChatFromChatsByWorkspace removes only mappings pointing at the chat", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-1",
			"ws-2": "chat-2",
		});
		queryClient.setQueryData(chatsByWorkspace(["ws-3"]).queryKey, {
			"ws-3": "chat-1",
		});
		seedInfiniteChats(queryClient, [makeChat("chat-1")]);
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, []);

		removeChatFromChatsByWorkspace(queryClient, "chat-1");

		expect(
			queryClient.getQueryData(chatsByWorkspace(["ws-1"]).queryKey),
		).toEqual({ "ws-2": "chat-2" });
		expect(
			queryClient.getQueryData(chatsByWorkspace(["ws-3"]).queryKey),
		).toEqual({});
		for (const [label, key] of [
			["chat list", infiniteChatsTestKey],
			["chat detail", chatEntityKey("chat-1")],
			["chat search", chatSearch({ q: "alpha" }).queryKey],
		] as const) {
			expect(
				queryClient.getQueryData(key),
				`${label} entry should survive removeChatFromChatsByWorkspace`,
			).toBeDefined();
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} entry should NOT be invalidated`,
			).not.toBe(true);
		}
	});

	it("removeChatFromChatsByWorkspace preserves the previous reference when the chat is absent", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": "chat-2",
		});
		const before = queryClient.getQueryData(
			chatsByWorkspace(["ws-1"]).queryKey,
		);

		removeChatFromChatsByWorkspace(queryClient, "chat-1");

		expect(queryClient.getQueryData(chatsByWorkspace(["ws-1"]).queryKey)).toBe(
			before,
		);
	});
});

describe("message upsert fan-out and history replacement", () => {
	type InfMessages = {
		pages: TypesGen.ChatMessagesResponse[];
		pageParams: (number | undefined)[];
	};

	const mockChatMessage = (
		id: number,
		text = `msg ${id}`,
	): TypesGen.ChatMessage => ({
		...MockChatMessage,
		id,
		content: [{ type: "text", text }],
	});

	/** Seed and read back the canonical stored object so reference
	 *  assertions compare against what the cache actually holds. */
	const seedMessagePages = (
		queryClient: QueryClient,
		data: InfMessages,
	): InfMessages => {
		queryClient.setQueryData<InfMessages>(chatMessagesKey("chat-1"), data);
		const seeded = queryClient.getQueryData<InfMessages>(
			chatMessagesKey("chat-1"),
		);
		if (!seeded) {
			throw new Error("failed to seed messages cache");
		}
		return seeded;
	};

	const readMessagePages = (
		queryClient: QueryClient,
	): InfMessages | undefined =>
		queryClient.getQueryData<InfMessages>(chatMessagesKey("chat-1"));

	// pages[0] is the newest page and every page is DESC by ID,
	// matching chatMessagesForInfiniteScroll.
	const twoPageFixture = (): InfMessages => ({
		pages: [
			{
				messages: [mockChatMessage(60), mockChatMessage(55)],
				queued_messages: [],
				has_more: true,
			},
			{
				messages: [mockChatMessage(50), mockChatMessage(45)],
				queued_messages: [],
				has_more: false,
			},
		],
		pageParams: [undefined, 55],
	});

	it("upsertChatMessages replaces a message found only in an older page in place", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [mockChatMessage(45, "updated")]);

		const after = readMessagePages(queryClient);
		expect(after?.pages[0]).toBe(before.pages[0]);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([60, 55]);
		expect(after?.pages[1]?.messages.map((m) => m.id)).toEqual([50, 45]);
		expect(after?.pages[1]?.messages[1]?.content).toEqual([
			{ type: "text", text: "updated" },
		]);
	});

	it("upsertChatMessages gives every containing page the same fresh value for a duplicated ID", () => {
		const queryClient = createTestQueryClient();
		seedMessagePages(queryClient, {
			pages: [
				{
					messages: [mockChatMessage(60), mockChatMessage(45)],
					queued_messages: [],
					has_more: true,
				},
				{
					messages: [mockChatMessage(50), mockChatMessage(45)],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined, 45],
		});

		upsertChatMessages(queryClient, "chat-1", [mockChatMessage(45, "fresh")]);

		const after = readMessagePages(queryClient);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([60, 45]);
		expect(after?.pages[1]?.messages.map((m) => m.id)).toEqual([50, 45]);
		expect(after?.pages[0]?.messages[1]?.content).toEqual([
			{ type: "text", text: "fresh" },
		]);
		expect(after?.pages[1]?.messages[1]?.content).toEqual([
			{ type: "text", text: "fresh" },
		]);
	});

	it("upsertChatMessages preserves the previous reference for a found-but-equal ID in an older page", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [mockChatMessage(45)]);

		const after = readMessagePages(queryClient);
		expect(after).toBe(before);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([60, 55]);
	});

	it("upsertChatMessages prepends an unknown newest message to page 0 in descending order", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [mockChatMessage(70)]);

		const after = readMessagePages(queryClient);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([70, 60, 55]);
		expect(after?.pages[1]).toBe(before.pages[1]);
	});

	it("upsertChatMessages inserts an unknown interleaving message mid-page-0 in descending order", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [mockChatMessage(57)]);

		const after = readMessagePages(queryClient);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([60, 57, 55]);
		expect(after?.pages[1]).toBe(before.pages[1]);
	});

	it("upsertChatMessages inserts each unseen ID once when the batch carries two revisions of it", () => {
		const queryClient = createTestQueryClient();
		seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [
			mockChatMessage(70, "revision 1"),
			mockChatMessage(70, "revision 2"),
		]);

		const after = readMessagePages(queryClient);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([70, 60, 55]);
		expect(after?.pages[0]?.messages[0]?.content).toEqual([
			{ type: "text", text: "revision 2" },
		]);
	});

	it("upsertChatMessages applies a mixed replace-and-insert batch in a single call", () => {
		const queryClient = createTestQueryClient();
		seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [
			mockChatMessage(45, "updated"),
			mockChatMessage(70),
		]);

		const after = readMessagePages(queryClient);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([70, 60, 55]);
		expect(after?.pages[1]?.messages.map((m) => m.id)).toEqual([50, 45]);
		expect(after?.pages[1]?.messages[1]?.content).toEqual([
			{ type: "text", text: "updated" },
		]);
	});

	it("upsertChatMessages never changes the pageParams reference", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [
			mockChatMessage(45, "updated"),
			mockChatMessage(70),
		]);

		const after = readMessagePages(queryClient);
		expect(after?.pageParams).toBe(before.pageParams);
		expect(after?.pageParams).toEqual([undefined, 55]);
	});

	it("upsertChatMessages returns the previous reference for a same-value batch", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, twoPageFixture());

		upsertChatMessages(queryClient, "chat-1", [
			mockChatMessage(60),
			mockChatMessage(45),
		]);

		expect(readMessagePages(queryClient)).toBe(before);
	});

	it("upsertChatMessages is a no-op on an absent cache and creates no entry", () => {
		const queryClient = createTestQueryClient();

		upsertChatMessages(queryClient, "chat-1", [mockChatMessage(70)]);

		expect(
			queryClient.getQueryCache().find({ queryKey: chatMessagesKey("chat-1") }),
		).toBeUndefined();
	});

	it("replaceChatMessagesHistory collapses to one page and one pageParam, preserving queued messages", () => {
		const queryClient = createTestQueryClient();
		const queuedMessage: TypesGen.ChatQueuedMessage = {
			id: 1,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:10:00.000Z",
			content: [{ type: "text", text: "queued" }],
		};
		seedMessagePages(queryClient, {
			pages: [
				{
					messages: [mockChatMessage(60), mockChatMessage(55)],
					queued_messages: [queuedMessage],
					has_more: true,
				},
				{
					messages: [mockChatMessage(50), mockChatMessage(45)],
					queued_messages: [],
					has_more: true,
				},
				{
					messages: [mockChatMessage(40)],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined, 55, 45],
		});

		replaceChatMessagesHistory(queryClient, "chat-1", [
			mockChatMessage(55),
			mockChatMessage(60, "rewritten"),
		]);

		const after = readMessagePages(queryClient);
		expect(after?.pages).toHaveLength(1);
		expect(after?.pageParams).toHaveLength(1);
		expect(after?.pageParams).toEqual([undefined]);
		expect(after?.pages[0]?.messages.map((m) => m.id)).toEqual([60, 55]);
		expect(after?.pages[0]?.has_more).toBe(false);
		expect(after?.pages[0]?.queued_messages).toEqual([queuedMessage]);
	});

	it("replaceChatMessagesHistory preserves the previous reference when the replacement equals the current single page", () => {
		const queryClient = createTestQueryClient();
		const before = seedMessagePages(queryClient, {
			pages: [
				{
					messages: [mockChatMessage(60), mockChatMessage(55)],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		});

		replaceChatMessagesHistory(queryClient, "chat-1", [
			mockChatMessage(55),
			mockChatMessage(60),
		]);

		expect(readMessagePages(queryClient)).toBe(before);
	});

	it("replaceChatMessagesHistory is a no-op on an absent cache and creates no entry", () => {
		const queryClient = createTestQueryClient();

		replaceChatMessagesHistory(queryClient, "chat-1", [mockChatMessage(60)]);

		expect(
			queryClient.getQueryCache().find({ queryKey: chatMessagesKey("chat-1") }),
		).toBeUndefined();
	});
});

describe("chatEntitiesFamilyKey shape", () => {
	// chatEntityKey builds on this prefix, so every entity detail entry
	// shares the family root and can be addressed as a group.
	it("prefixes every chat entity key", () => {
		expect(chatEntityKey("chat-1")).toEqual([
			...chatEntitiesFamilyKey,
			"chat-1",
		]);
	});
});

describe("applyChatArchiveStateToCaches search rows", () => {
	it("removes matching rows from every cached search and preserves unrelated rows by reference", () => {
		const queryClient = createTestQueryClient();
		const unrelatedRow = makeChat("chat-2");
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, [
			makeChat("chat-1", { pin_order: 2 }),
			unrelatedRow,
		]);
		queryClient.setQueryData(chatSearch({ q: "archived:true" }).queryKey, [
			makeChat("chat-1", { archived: true }),
		]);

		applyChatArchiveStateToCaches(queryClient, "chat-1", true);

		expect(
			queryClient
				.getQueryData<TypesGen.Chat[]>(chatSearch({ q: "alpha" }).queryKey)
				?.find((row) => row.id === "chat-1"),
		).toBeUndefined();
		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(
				chatSearch({ q: "archived:true" }).queryKey,
			),
		).toEqual([]);
		expect(
			queryClient
				.getQueryData<TypesGen.Chat[]>(chatSearch({ q: "alpha" }).queryKey)
				?.find((row) => row.id === "chat-2"),
		).toEqual(unrelatedRow);
	});

	it("preserves the previous array reference when the row is not cached", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, [
			makeChat("chat-2"),
		]);
		const before = queryClient.getQueryData(
			chatSearch({ q: "alpha" }).queryKey,
		);

		applyChatArchiveStateToCaches(queryClient, "chat-1", true);

		expect(queryClient.getQueryData(chatSearch({ q: "alpha" }).queryKey)).toBe(
			before,
		);
	});

	it("leaves per-chat sub-resource entries untouched", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatPromptsKey(chatId), { prompts: [] });
		queryClient.setQueryData(chatACLKey(chatId), {});
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });

		applyChatArchiveStateToCaches(queryClient, chatId, true);

		for (const [label, key] of [
			["messages", chatMessagesKey(chatId)],
			["prompts", chatPromptsKey(chatId)],
			["acl", chatACLKey(chatId)],
			["diff-contents", chatDiffContentsKey(chatId)],
		] as const) {
			expect(
				queryClient.getQueryData(key),
				`${label} entry should survive`,
			).toBeDefined();
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} entry should NOT be invalidated`,
			).not.toBe(true);
		}
	});
});

describe("applyWatchedChatArchived", () => {
	it("restarts an active initial entity fetch so stale data cannot overwrite archive", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const staleChat = makeChat(chatId, { archived: false });
		const durableChat = makeChat(chatId, { archived: true });
		const fetch = observeChatWithDeferredFirstFetch(
			queryClient,
			staleChat,
			durableChat,
		);

		applyWatchedChatArchived(queryClient, durableChat);
		fetch.firstFetch.resolve(staleChat);
		await fetch.durableResult.promise;

		expect(fetch.fetchCount()).toBe(2);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
		).toBe(true);
		fetch.unsubscribe();
	});

	it("does not create an entity query when no observer is mounted", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		applyWatchedChatArchived(queryClient, makeChat(chatId, { archived: true }));

		expect(queryClient.getQueryState(chatEntityKey(chatId))).toBeUndefined();
	});

	it("patches the entity in place instead of removing it", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(
			chatEntityKey(chatId),
			makeChat(chatId, { pin_order: 2 }),
		);

		applyWatchedChatArchived(queryClient, makeChat(chatId, { archived: true }));

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId)),
		).toMatchObject({
			archived: true,
			pin_order: 0,
		});
	});

	it("drops the chat from active lists and patches it in archived lists", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId), makeChat("chat-2")], {
			archived: false,
		});
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })], {
			archived: true,
		});

		applyWatchedChatArchived(queryClient, makeChat(chatId, { archived: true }));

		expect(
			readInfiniteChats(queryClient, { archived: false })?.map(
				(chat) => chat.id,
			),
		).toEqual(["chat-2"]);
		expect(
			readInfiniteChats(queryClient, { archived: true })?.[0],
		).toMatchObject({
			id: chatId,
			archived: true,
			pin_order: 0,
		});
	});

	it("removes search rows and removes the by-workspace mapping", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, [
			makeChat(chatId),
		]);
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {
			"ws-1": chatId,
		});

		applyWatchedChatArchived(queryClient, makeChat(chatId, { archived: true }));

		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(
				chatSearch({ q: "alpha" }).queryKey,
			),
		).toEqual([]);
		expect(
			queryClient.getQueryData(chatsByWorkspace(["ws-1"]).queryKey),
		).toEqual({});
	});

	it("invalidates the list, by-workspace, and search families but not per-chat sub-resources", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		queryClient.setQueryData(chatEntityKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, []);
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {});

		applyWatchedChatArchived(queryClient, makeChat(chatId, { archived: true }));

		for (const [label, key] of [
			["chat list", infiniteChatsTestKey],
			["chat search", chatSearch({ q: "alpha" }).queryKey],
			["by-workspace", chatsByWorkspace(["ws-1"]).queryKey],
		] as const) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} entry should be invalidated`,
			).toBe(true);
		}
		expect(
			queryClient.getQueryData(chatMessagesKey(chatId)),
			"messages entry should survive",
		).toBeDefined();
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"messages entry should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryData(chatEntityKey(chatId)),
			"entity entry should survive",
		).toBeDefined();
	});
});

describe("applyWatchedChatCreatedOrUnarchived", () => {
	it("restarts an active initial entity fetch so stale data cannot overwrite unarchive", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const staleChat = makeChat(chatId, { archived: true });
		const durableChat = makeChat(chatId, { archived: false });
		const fetch = observeChatWithDeferredFirstFetch(
			queryClient,
			staleChat,
			durableChat,
		);

		applyWatchedChatCreatedOrUnarchived(queryClient, durableChat);
		fetch.firstFetch.resolve(staleChat);
		await fetch.durableResult.promise;

		expect(fetch.fetchCount()).toBe(2);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
		).toBe(false);
		fetch.unsubscribe();
	});

	it("restarts an active initial fetch for a genuinely new chat", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-new";
		const staleChat = makeChat(chatId, { title: "stale" });
		const durableChat = makeChat(chatId, { title: "durable" });
		const fetch = observeChatWithDeferredFirstFetch(
			queryClient,
			staleChat,
			durableChat,
		);

		applyWatchedChatCreatedOrUnarchived(queryClient, durableChat);
		fetch.firstFetch.resolve(staleChat);
		await fetch.durableResult.promise;

		expect(fetch.fetchCount()).toBe(2);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.title,
		).toBe("durable");
		fetch.unsubscribe();
	});

	it("flips a cached archived entity back to active and repairs list rows", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(
			chatEntityKey(chatId),
			makeChat(chatId, { archived: true }),
		);
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })], {
			archived: true,
		});
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })], {
			archived: false,
		});

		applyWatchedChatCreatedOrUnarchived(queryClient, makeChat(chatId));

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId))?.archived,
		).toBe(false);
		expect(readInfiniteChats(queryClient, { archived: true })).toEqual([]);
		expect(
			readInfiniteChats(queryClient, { archived: false })?.[0].archived,
		).toBe(false);
	});

	it("only invalidates the collection families for a truly new chat", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-new";
		seedInfiniteChats(queryClient, [makeChat("chat-2")]);
		queryClient.setQueryData(chatSearch({ q: "alpha" }).queryKey, []);
		queryClient.setQueryData(chatsByWorkspace(["ws-1"]).queryKey, {});

		applyWatchedChatCreatedOrUnarchived(queryClient, makeChat(chatId));

		expect(queryClient.getQueryData(chatEntityKey(chatId))).toBeUndefined();
		expect(queryClient.getQueryState(chatEntityKey(chatId))).toBeUndefined();
		for (const [label, key] of [
			["chat list", infiniteChatsTestKey],
			["chat search", chatSearch({ q: "alpha" }).queryKey],
			["by-workspace", chatsByWorkspace(["ws-1"]).queryKey],
		] as const) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${label} entry should be invalidated`,
			).toBe(true);
		}
	});
});

describe("archive mutation entity retention", () => {
	it.each([
		{ name: "archiveChat", factory: archiveChat, archived: true },
		{ name: "unarchiveChat", factory: unarchiveChat, archived: false },
	])("$name onSuccess never removes the entity family", ({
		factory,
		archived,
	}) => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(
			chatEntityKey(chatId),
			makeChat(chatId, { archived: !archived }),
		);
		queryClient.setQueryData(chatMessagesKey(chatId), []);

		const mutation = factory(queryClient);
		mutation.onSuccess(undefined, chatId);
		mutation.onSettled(undefined, undefined, chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(chatId)),
		).toMatchObject({ archived });
		expect(queryClient.getQueryData(chatMessagesKey(chatId))).toBeDefined();
	});
});
