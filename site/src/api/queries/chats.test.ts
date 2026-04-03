import { QueryClient } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	archiveChat,
	cancelChatListRefetches,
	chatCostSummary,
	chatCostSummaryKey,
	chatCostUsers,
	chatCostUsersKey,
	chatDiffContentsKey,
	chatKey,
	chatMessagesKey,
	chatsKey,
	createChat,
	createChatMessage,
	deleteChatQueuedMessage,
	editChatMessage,
	infiniteChats,
	interruptChat,
	invalidateChatListQueries,
	pinChat,
	promoteChatQueuedMessage,
	regenerateChatTitle,
	reorderPinnedChat,
	unarchiveChat,
	unpinChat,
	updateInfiniteChatsCache,
} from "./chats";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			updateChat: vi.fn(),
			createChat: vi.fn(),
			deleteChatQueuedMessage: vi.fn(),
			getChats: vi.fn(),
			getChatCostSummary: vi.fn(),
			getChatCostUsers: vi.fn(),
			createChatMessage: vi.fn(),
			editChatMessage: vi.fn(),
			interruptChat: vi.fn(),
			promoteChatQueuedMessage: vi.fn(),
			regenerateChatTitle: vi.fn(),
		},
	},
}));

// The infinite query key used by useInfiniteQuery(infiniteChats())
// is [...chatsKey, undefined] = ["chats", undefined].
const infiniteChatsTestKey = [...chatsKey, undefined];

type InfiniteData = {
	pages: TypesGen.Chat[][];
	pageParams: unknown[];
};

/** Seed the infinite chats cache in the format TanStack Query expects. */
const seedInfiniteChats = (
	queryClient: QueryClient,
	chats: TypesGen.Chat[],
) => {
	queryClient.setQueryData<InfiniteData>(infiniteChatsTestKey, {
		pages: [chats],
		pageParams: [0],
	});
};

/** Read chats back from the infinite query cache. */
const readInfiniteChats = (
	queryClient: QueryClient,
): TypesGen.Chat[] | undefined => {
	const data = queryClient.getQueryData<InfiniteData>(infiniteChatsTestKey);
	return data?.pages.flat();
};

const makeChat = (
	id: string,
	overrides?: Partial<TypesGen.Chat>,
): TypesGen.Chat => ({
	id,
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	title: `Chat ${id}`,
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	last_error: null,
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

describe("invalidateChatListQueries", () => {
	it("invalidates flat and infinite chat list queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Sidebar queries.
		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);
		queryClient.setQueryData([...chatsKey, { archived: false }], {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		// Per-chat queries that should NOT be touched.
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), {});
		queryClient.setQueryData(
			chatCostSummaryKey("me", undefined),
			{} as TypesGen.ChatCostSummary,
		);

		await invalidateChatListQueries(queryClient);

		// Sidebar queries should be invalidated.
		expect(
			queryClient.getQueryState(chatsKey)?.isInvalidated,
			"flat chats should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState([...chatsKey, { archived: false }])
				?.isInvalidated,
			"infinite chats should be invalidated",
		).toBe(true);

		// Per-chat queries should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatKey(chatId))?.isInvalidated,
			"chatKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatDiffContentsKey(chatId))?.isInvalidated,
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

		queryClient.setQueryData([...chatsKey, undefined], {
			pages: [[makeChat("chat-1")]],
			pageParams: [0],
		});

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState([...chatsKey, undefined])?.isInvalidated,
			"infinite chats with undefined opts should be invalidated",
		).toBe(true);
	});

	it("does not invalidate chatCostUsersKey", async () => {
		const queryClient = createTestQueryClient();

		queryClient.setQueryData(chatCostUsersKey(undefined), {});
		queryClient.setQueryData(chatsKey, [makeChat("chat-1")]);

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatCostUsersKey(undefined))?.isInvalidated,
			"chatCostUsersKey should NOT be invalidated",
		).not.toBe(true);
	});

	it("does not invalidate a different chat's queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const otherChatId = "chat-2";

		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);
		queryClient.setQueryData(chatKey(otherChatId), makeChat(otherChatId));
		queryClient.setQueryData(chatMessagesKey(otherChatId), []);

		await invalidateChatListQueries(queryClient);

		expect(
			queryClient.getQueryState(chatKey(otherChatId))?.isInvalidated,
			"other chat's chatKey should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatMessagesKey(otherChatId))?.isInvalidated,
			"other chat's chatMessagesKey should NOT be invalidated",
		).not.toBe(true);
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
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));

		vi.mocked(API.experimental.updateChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const cachedChat = queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId));
		expect(cachedChat?.archived).toBe(true);
	});

	it("rolls back the chats list on error by invalidating", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialChats = [makeChat(chatId)];
		seedInfiniteChats(queryClient, initialChats);
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify the optimistic update took effect.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);

		// Simulate an error — the onError handler invalidates the
		// cache so a re-fetch restores the correct state.
		mutation.onError(new Error("server error"), chatId, context);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
	});

	it("rolls back the individual chat cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.archived,
		).toBe(true);

		mutation.onError(new Error("server error"), chatId, context);

		const rolledBack = queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId));
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
			expect.objectContaining({ queryKey: chatsKey }),
		);
	});

	it("handles onMutate when no individual chat cache exists", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId)]);
		// Deliberately do NOT set chatKey(chatId) data.

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// The list should still be optimistically updated.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(true);
		// previousChat should be undefined.
		expect(context?.previousChat).toBeUndefined();
	});

	it("invalidates queries on settled regardless of outcome", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
			exact: true,
		});
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
			chatKey(chatId),
			makeChat(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.archived,
		).toBe(false);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chatKey(chatId),
			makeChat(chatId, { archived: true }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unarchiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].archived).toBe(false);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.archived,
		).toBe(false);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// The chats list is rolled back via invalidation.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
		// The individual chat cache is restored directly.
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.archived,
		).toBe(true);
	});

	it("invalidates queries on settled", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unarchiveChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
			exact: true,
		});
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
		queryClient.setQueryData([...chatsKey, { archived: true }], {
			pages: [[makeChat("chat-pinned-archived", { pin_order: 4 })]],
			pageParams: [0],
		});
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));

		const mutation = pinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			readInfiniteChats(queryClient)?.find((chat) => chat.id === chatId)
				?.pin_order,
		).toBe(5);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.pin_order,
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
			chatKey(chatId),
			makeChat(chatId, { pin_order: 2 }),
		);

		const mutation = unpinChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.pin_order,
		).toBe(0);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedInfiniteChats(queryClient, [makeChat(chatId, { pin_order: 3 })]);
		queryClient.setQueryData(
			chatKey(chatId),
			makeChat(chatId, { pin_order: 3 }),
		);
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(readInfiniteChats(queryClient)?.[0].pin_order).toBe(0);
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.pin_order,
		).toBe(0);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		// The chats list is rolled back via invalidation.
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
		// The individual chat cache is restored directly.
		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId))?.pin_order,
		).toBe(3);
	});

	it("invalidates queries on settled", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unpinChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
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
			expect.objectContaining({ queryKey: chatsKey }),
		);
		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
			exact: true,
		});
		expect(API.experimental.updateChat).toHaveBeenCalledWith(chatId, {
			pin_order: 2,
		});
		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({ queryKey: chatsKey }),
		);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
			exact: true,
		});
	});
});

describe("regenerateChatTitle cache updates", () => {
	it("preserves existing chat detail fields when the response is partial", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const cachedChat = makeChat(chatId, {
			diff_status: {
				chat_id: chatId,
				url: "https://example.com/pr/1",
				pull_request_state: "open",
				pull_request_title: "",
				pull_request_draft: false,
				changes_requested: false,
				additions: 1,
				deletions: 2,
				changed_files: 3,
				refreshed_at: "2025-01-01T00:00:00.000Z",
				stale_at: "2025-01-01T01:00:00.000Z",
			},
		});
		queryClient.setQueryData(chatKey(chatId), cachedChat);
		seedInfiniteChats(queryClient, [cachedChat]);

		const mutation = regenerateChatTitle(queryClient);
		const updatedChat = {
			id: chatId,
			title: "New title",
		} satisfies Partial<TypesGen.Chat>;

		mutation.onSuccess(updatedChat as TypesGen.Chat);

		const cachedDetail = queryClient.getQueryData<TypesGen.Chat>(
			chatKey(chatId),
		);
		expect(cachedDetail).toEqual({
			...cachedChat,
			title: "New title",
		});
		expect(cachedDetail?.diff_status).toEqual(cachedChat.diff_status);
		expect(readInfiniteChats(queryClient)?.[0]).toMatchObject({
			id: chatId,
			title: "New title",
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

	it("builds a distinct users query key and forwards snake_case params", async () => {
		const params = {
			start_date: "2025-01-01",
			end_date: "2025-01-31",
			username: "alice",
			limit: 10,
			offset: 20,
		};
		vi.mocked(API.experimental.getChatCostUsers).mockResolvedValue(
			{} as TypesGen.ChatCostUsersResponse,
		);

		const query = chatCostUsers(params);

		expect(chatCostUsersKey(params)).toEqual(["chats", "costUsers", params]);
		expect(query.queryKey).toEqual(["chats", "costUsers", params]);
		expect(query.queryKey).not.toEqual(chatCostSummaryKey("me", params));
		await query.queryFn();
		expect(API.experimental.getChatCostUsers).toHaveBeenCalledWith(params);
	});
});

describe("mutation invalidation scope", () => {
	// These tests assert the CORRECT (narrow) invalidation behaviour.
	// Each mutation should only invalidate the queries it actually
	// needs to refresh — not the entire ["chats"] prefix tree. The
	// WebSocket stream already delivers real-time updates for
	// messages, status changes, and sidebar ordering, so broad
	// prefix invalidation causes a burst of redundant HTTP requests
	// on the /agents page.

	/** Populate the QueryClient with every query key that is actively
	 *  observed on the /agents/:id detail page. */
	const seedAllActiveQueries = (queryClient: QueryClient, chatId: string) => {
		// Infinite sidebar list: ["chats", { archived: false }]
		queryClient.setQueryData([...chatsKey, { archived: false }], {
			pages: [[makeChat(chatId)]],
			pageParams: [0],
		});
		// Flat chats list: ["chats"]
		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);
		// Individual chat: ["chats", chatId]
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));
		// Messages: ["chats", chatId, "messages"]
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		// Diff contents: ["chats", chatId, "diff-contents"]
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });
		// Cost summary: ["chats", "costSummary", "me", undefined]
		queryClient.setQueryData(
			chatCostSummaryKey("me", undefined),
			{} as TypesGen.ChatCostSummary,
		);
	};

	/** Keys that should NEVER be invalidated by chat message mutations
	 *  because they are completely unrelated to the message flow. */
	const unrelatedKeys = (chatId: string) => [
		{ label: "diff-contents", key: chatDiffContentsKey(chatId) },
		{ label: "cost-summary", key: chatCostSummaryKey("me", undefined) },
	];

	it("createChatMessage does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		// createChatMessage has no onSuccess handler — the WebSocket
		// stream covers all real-time updates. Verify that constructing
		// the mutation config does not define one.
		const mutation = createChatMessage(queryClient, chatId);
		expect(mutation).not.toHaveProperty("onSuccess");

		// Since there is no onSuccess, no queries should be invalidated.
		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by createChatMessage`,
			).not.toBe(true);
		}
	});

	it("createChatMessage does not invalidate chat detail or messages (WebSocket handles these)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		// No onSuccess handler exists.
		const mutation = createChatMessage(queryClient, chatId);
		expect(mutation).not.toHaveProperty("onSuccess");

		const chatState = queryClient.getQueryState(chatKey(chatId));
		expect(
			chatState?.isInvalidated,
			"chatKey should NOT be invalidated",
		).not.toBe(true);

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

	it("editChatMessage invalidates only chat detail and messages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = editChatMessage(queryClient, chatId);
		mutation.onSettled();

		await new Promise((r) => setTimeout(r, 0));

		// These two should still be invalidated — editing changes
		// message content and potentially the chat's updated_at.
		const chatState = queryClient.getQueryState(chatKey(chatId));
		expect(chatState?.isInvalidated, "chatKey should be invalidated").toBe(
			true,
		);

		const messagesState = queryClient.getQueryState(chatMessagesKey(chatId));
		expect(
			messagesState?.isInvalidated,
			"chatMessagesKey should be invalidated",
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

	const editReq = {
		content: [{ type: "text" as const, text: "edited" }],
	};

	it("editChatMessage optimistically removes truncated messages from cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			req: editReq,
		});

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([2, 1]);
		expect(context?.previousData?.pages[0]?.messages).toHaveLength(5);
	});

	it("editChatMessage restores cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		const context = await mutation.onMutate({
			messageId: 3,
			req: editReq,
		});

		expect(
			queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId))?.pages[0]
				?.messages,
		).toHaveLength(2);

		mutation.onError(
			new Error("network failure"),
			{ messageId: 3, req: editReq },
			context,
		);

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([5, 4, 3, 2, 1]);
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

		// Pass undefined context — simulates onMutate throwing before
		// it could return a snapshot.
		mutation.onError(
			new Error("fail"),
			{ messageId: 2, req: editReq },
			undefined,
		);

		// Cache should be untouched — no crash, no corruption.
		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([3, 2, 1]);
	});

	it("editChatMessage onMutate filters across multiple pages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Page 0 (newest): IDs 10–6. Page 1 (older): IDs 5–1.
		const page0 = [10, 9, 8, 7, 6].map((id) => makeMsg(chatId, id));
		const page1 = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [
				{ messages: page0, queued_messages: [], has_more: true },
				{ messages: page1, queued_messages: [], has_more: false },
			],
			pageParams: [undefined, 6],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({ messageId: 7, req: editReq });

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		// Page 0: only ID 6 survives (< 7).
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([6]);
		// Page 1: all survive (all < 7).
		expect(data?.pages[1]?.messages.map((m) => m.id)).toEqual([5, 4, 3, 2, 1]);
	});

	it("editChatMessage onMutate editing the first message empties all pages", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({ messageId: 1, req: editReq });

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		// All messages have id >= 1, so the page is empty.
		expect(data?.pages[0]?.messages).toHaveLength(0);
		// Sibling fields survive the spread.
		expect(data?.pages[0]?.queued_messages).toEqual([]);
		expect(data?.pages[0]?.has_more).toBe(false);
	});

	it("editChatMessage onMutate editing the latest message keeps earlier ones", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const messages = [5, 4, 3, 2, 1].map((id) => makeMsg(chatId, id));

		queryClient.setQueryData<InfMessages>(chatMessagesKey(chatId), {
			pages: [{ messages, queued_messages: [], has_more: false }],
			pageParams: [undefined],
		});

		const mutation = editChatMessage(queryClient, chatId);
		await mutation.onMutate({ messageId: 5, req: editReq });

		const data = queryClient.getQueryData<InfMessages>(chatMessagesKey(chatId));
		expect(data?.pages[0]?.messages.map((m) => m.id)).toEqual([4, 3, 2, 1]);
	});

	it("interruptChat does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		// interruptChat has no onSuccess handler — the WebSocket
		// delivers status changes in real-time.
		const mutation = interruptChat(queryClient, chatId);
		expect(mutation).not.toHaveProperty("onSuccess");

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by interruptChat`,
			).not.toBe(true);
		}
	});

	it("promoteChatQueuedMessage does not invalidate unrelated queries", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = promoteChatQueuedMessage(queryClient, chatId);
		expect(mutation).not.toHaveProperty("onSuccess");

		for (const { label, key } of unrelatedKeys(chatId)) {
			const state = queryClient.getQueryState(key);
			expect(
				state?.isInvalidated,
				`${label} should NOT be invalidated by promoteChatQueuedMessage`,
			).not.toBe(true);
		}
	});

	it("createChat invalidates only sidebar queries on success", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		seedAllActiveQueries(queryClient, chatId);

		const mutation = createChat(queryClient);
		mutation.onSuccess();

		await new Promise((r) => setTimeout(r, 0));

		// Sidebar lists SHOULD be invalidated.
		expect(
			queryClient.getQueryState(chatsKey)?.isInvalidated,
			"flat chats should be invalidated",
		).toBe(true);
		expect(
			queryClient.getQueryState([...chatsKey, { archived: false }])
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
			queryClient.getQueryState(chatKey(chatId))?.isInvalidated,
			"chatKey should NOT be invalidated",
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
			queryClient.getQueryState(chatKey(chatId))?.isInvalidated,
			"chatKey should be invalidated",
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
			queryClient.getQueryState(chatsKey)?.isInvalidated,
			"flat chats should NOT be invalidated",
		).not.toBe(true);
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

		it("throws when pageParam is not a number", () => {
			const { queryFn } = infiniteChats();
			expect(() => queryFn({ pageParam: "bad" })).toThrow(
				"pageParam must be a number",
			);
		});
	});
});

describe("diff_status_change invalidation scope", () => {
	// These tests verify the CORRECT invalidation pattern for
	// diff_status_change WebSocket events. The handler should
	// invalidate only the individual chat detail and diff-contents
	// queries — NOT the chat list (sidebar) or messages.

	it("exact chatKey invalidation does not cascade to messages or diff-contents", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		// Seed all the queries that are active on the /agents/:id page.
		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });
		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);

		// This is what the fixed handler does — exact: true.
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});

		// chatKey itself should be invalidated.
		expect(
			queryClient.getQueryState(chatKey(chatId))?.isInvalidated,
			"chatKey should be invalidated",
		).toBe(true);

		// Messages should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatMessagesKey(chatId))?.isInvalidated,
			"chatMessagesKey should NOT be invalidated by exact chatKey",
		).not.toBe(true);

		// Diff-contents should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatDiffContentsKey(chatId))?.isInvalidated,
			"chatDiffContentsKey should NOT be invalidated by exact chatKey",
		).not.toBe(true);

		// Chat list should NOT be invalidated.
		expect(
			queryClient.getQueryState(chatsKey)?.isInvalidated,
			"chatsKey should NOT be invalidated by exact chatKey",
		).not.toBe(true);
	});

	it("without exact: true, chatKey invalidation cascades to messages and diff-contents (the old bug)", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";

		queryClient.setQueryData(chatKey(chatId), makeChat(chatId));
		queryClient.setQueryData(chatMessagesKey(chatId), []);
		queryClient.setQueryData(chatDiffContentsKey(chatId), { files: [] });

		// This is what the OLD (broken) handler did — no exact: true.
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
		});

		// Without exact: true, ALL queries starting with ["chats", chatId]
		// get invalidated, including messages and diff-contents.
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

		// Start an in-flight refetch (no fetchMeta — simulates a
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

		// The fetch was NOT cancelled — the new data landed.
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

		// Do NOT seed the cache — simulate the very first fetch
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
