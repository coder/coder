import { QueryClient } from "react-query";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	chatACLKey,
	chatCache,
	chatCostTreeKey,
	chatDebugRunKey,
	chatDebugRunsKey,
	chatDiffContentsKey,
	chatEntityKey,
	chatListKey,
	chatMessagesKey,
	chatPromptsKey,
	toChatListParams,
} from "./chats";

type InfiniteChatsData = {
	pages: TypesGen.Chat[][];
	pageParams: unknown[];
};

type InfiniteMessagesData = {
	pages: TypesGen.ChatMessagesResponse[];
	pageParams: (number | undefined)[];
};

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

const makeChat = (id: string): TypesGen.Chat => ({
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
});

const makeMessages = (): InfiniteMessagesData => ({
	pages: [{ messages: [], queued_messages: [], has_more: false }],
	pageParams: [undefined],
});

// Every per-chat entry used to probe family boundaries.
const entitySubKeys = (chatId: string) => ({
	detail: chatEntityKey(chatId),
	messages: chatMessagesKey(chatId),
	prompts: chatPromptsKey(chatId),
	acl: chatACLKey(chatId),
	diffContents: chatDiffContentsKey(chatId),
	debugRuns: chatDebugRunsKey(chatId),
	debugRun: chatDebugRunKey(chatId, "run-1"),
});

// Collection entries must never be touched by entity operations.
const collectionKeys = () => ({
	list: chatListKey(toChatListParams()),
	byWorkspace: ["chats", "collections", "by-workspace", ["ws-1"]] as const,
});

const seedAll = (queryClient: QueryClient, chatId: string) => {
	queryClient.setQueryData(entitySubKeys(chatId).detail, makeChat(chatId));
	queryClient.setQueryData(entitySubKeys(chatId).messages, makeMessages());
	queryClient.setQueryData(entitySubKeys(chatId).prompts, { prompts: [] });
	queryClient.setQueryData(entitySubKeys(chatId).acl, {});
	queryClient.setQueryData(entitySubKeys(chatId).diffContents, { files: [] });
	queryClient.setQueryData(entitySubKeys(chatId).debugRuns, []);
	queryClient.setQueryData(entitySubKeys(chatId).debugRun, {});
	queryClient.setQueryData(collectionKeys().list, {
		pages: [[makeChat(chatId)]],
		pageParams: [0],
	} satisfies InfiniteChatsData);
	queryClient.setQueryData(collectionKeys().byWorkspace, {});
};

type ExactOp = {
	name: string;
	call: (queryClient: QueryClient, chatId: string) => unknown;
	target: (chatId: string) => readonly unknown[];
	untouched: readonly (keyof ReturnType<typeof entitySubKeys>)[];
};

describe("chatCache exact invalidations", () => {
	const exactOps: readonly ExactOp[] = [
		{
			name: "invalidateDetail",
			call: chatCache.invalidateDetail,
			target: (id) => chatEntityKey(id),
			untouched: ["messages", "prompts", "acl", "diffContents", "debugRuns"],
		},
		{
			name: "invalidateDiffContents",
			call: chatCache.invalidateDiffContents,
			target: (id) => chatDiffContentsKey(id),
			untouched: ["detail", "messages", "prompts", "acl", "debugRuns"],
		},
		{
			name: "invalidatePrompts",
			call: chatCache.invalidatePrompts,
			target: (id) => chatPromptsKey(id),
			untouched: ["detail", "messages", "acl", "diffContents", "debugRuns"],
		},
		{
			name: "invalidateMessages",
			call: chatCache.invalidateMessages,
			target: (id) => chatMessagesKey(id),
			untouched: ["detail", "prompts", "acl", "diffContents", "debugRuns"],
		},
		{
			name: "invalidateACL",
			call: chatCache.invalidateACL,
			target: (id) => chatACLKey(id),
			untouched: ["detail", "messages", "prompts", "diffContents", "debugRuns"],
		},
	];

	for (const { name, call, target, untouched } of exactOps) {
		it(`${name} touches only the exact entry`, async () => {
			const queryClient = createTestQueryClient();
			seedAll(queryClient, "chat-1");
			const keys = entitySubKeys("chat-1");

			await call(queryClient, "chat-1");

			expect(queryClient.getQueryState(target("chat-1"))?.isInvalidated).toBe(
				true,
			);
			for (const label of untouched) {
				expect(
					queryClient.getQueryState(keys[label])?.isInvalidated,
					`${label} entry should NOT be invalidated by ${name}`,
				).not.toBe(true);
			}
			expect(
				queryClient.getQueryState(collectionKeys().list)?.isInvalidated,
				"list entry should NOT be invalidated",
			).not.toBe(true);
		});
	}

	it("invalidateCostTree touches only the matching cost tree entry", async () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");
		queryClient.setQueryData(chatCostTreeKey("chat-1"), {});
		queryClient.setQueryData(chatCostTreeKey("chat-2"), {});

		await chatCache.invalidateCostTree(queryClient, "chat-1");

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

describe("chatCache prefix invalidations", () => {
	it("invalidateEntityFamily touches the entity family and nothing else", async () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");
		queryClient.setQueryData(chatCostTreeKey("chat-1"), {});
		const keys = entitySubKeys("chat-1");

		await chatCache.invalidateEntityFamily(queryClient, "chat-1");

		for (const label of Object.keys(keys) as (keyof ReturnType<
			typeof entitySubKeys
		>)[]) {
			expect(
				queryClient.getQueryState(keys[label])?.isInvalidated,
				`${label} entry should be invalidated`,
			).toBe(true);
		}
		expect(
			queryClient.getQueryState(collectionKeys().list)?.isInvalidated,
			"list entry should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(collectionKeys().byWorkspace)?.isInvalidated,
			"by-workspace entry should NOT be invalidated",
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatCostTreeKey("chat-1"))?.isInvalidated,
			"cost tree is analytics, not an entity descendant",
		).not.toBe(true);
	});

	it("invalidateLists touches every list entry and nothing outside the family", async () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");
		queryClient.setQueryData(
			chatListKey(toChatListParams({ archived: true })),
			{
				pages: [[makeChat("chat-1")]],
				pageParams: [0],
			},
		);

		await chatCache.invalidateLists(queryClient);

		expect(
			queryClient.getQueryState(chatListKey(toChatListParams()))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(
				chatListKey(toChatListParams({ archived: true })),
			)?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(collectionKeys().byWorkspace)?.isInvalidated,
		).not.toBe(true);
	});

	it("invalidateSearches touches search entries and nothing outside the family", async () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");
		queryClient.setQueryData(
			["chats", "collections", "search", { q: "a" }],
			[],
		);

		await chatCache.invalidateSearches(queryClient);

		expect(
			queryClient.getQueryState(["chats", "collections", "search", { q: "a" }])
				?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(collectionKeys().list)?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(collectionKeys().byWorkspace)?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
		).not.toBe(true);
	});

	it("invalidateByWorkspace touches by-workspace entries only", async () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");

		await chatCache.invalidateByWorkspace(queryClient);

		expect(
			queryClient.getQueryState(collectionKeys().byWorkspace)?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(collectionKeys().list)?.isInvalidated,
		).not.toBe(true);
		expect(
			queryClient.getQueryState(chatEntityKey("chat-1"))?.isInvalidated,
		).not.toBe(true);
	});

	it("invalidateDebugRuns touches the runs list and run details only", async () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");
		const keys = entitySubKeys("chat-1");

		await chatCache.invalidateDebugRuns(queryClient, "chat-1");

		expect(queryClient.getQueryState(keys.debugRuns)?.isInvalidated).toBe(true);
		expect(queryClient.getQueryState(keys.debugRun)?.isInvalidated).toBe(true);
		for (const label of [
			"detail",
			"messages",
			"prompts",
			"acl",
			"diffContents",
		] as const) {
			expect(
				queryClient.getQueryState(keys[label])?.isInvalidated,
				`${label} entry should NOT be invalidated`,
			).not.toBe(true);
		}
	});
});

describe("chatCache cancellation", () => {
	it("cancelLists cancels unconditionally across the list family", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await chatCache.cancelLists(queryClient);

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: ["chats", "collections", "list"],
		});
	});

	it("cancelListRefetches cancels with the loaded-refetch predicate", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await chatCache.cancelListRefetches(queryClient);

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: ["chats", "collections", "list"],
			predicate: expect.any(Function),
		});
	});

	it("cancelDetail cancels the exact detail entry unconditionally", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await chatCache.cancelDetail(queryClient, "chat-1");

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey("chat-1"),
			exact: true,
		});
	});

	it("cancelLoadedDetailRefetch is a no-op when detail data is absent", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await chatCache.cancelLoadedDetailRefetch(queryClient, "chat-1");

		expect(cancelSpy).not.toHaveBeenCalled();
	});

	it("cancelLoadedDetailRefetch cancels exactly when detail data exists", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await chatCache.cancelLoadedDetailRefetch(queryClient, "chat-1");

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatEntityKey("chat-1"),
			exact: true,
		});
	});

	it("cancelMessages cancels the exact messages entry", async () => {
		const queryClient = createTestQueryClient();
		const cancelSpy = vi.spyOn(queryClient, "cancelQueries");

		await chatCache.cancelMessages(queryClient, "chat-1");

		expect(cancelSpy).toHaveBeenCalledWith({
			queryKey: chatMessagesKey("chat-1"),
			exact: true,
		});
	});
});

describe("chatCache removal and patching", () => {
	it("removeDetail removes only the exact detail entry", () => {
		const queryClient = createTestQueryClient();
		seedAll(queryClient, "chat-1");
		const keys = entitySubKeys("chat-1");

		chatCache.removeDetail(queryClient, "chat-1");

		expect(queryClient.getQueryData(keys.detail)).toBeUndefined();
		for (const label of [
			"messages",
			"prompts",
			"acl",
			"diffContents",
			"debugRuns",
			"debugRun",
		] as const) {
			expect(
				queryClient.getQueryData(keys[label]),
				`${label} entry should survive removeDetail`,
			).toBeDefined();
		}
		expect(
			queryClient.getQueryData(collectionKeys().list),
			"list entry should survive removeDetail",
		).toBeDefined();
	});

	it("patchDetail applies the updater to the exact detail entry", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatEntityKey("chat-1"), makeChat("chat-1"));

		chatCache.patchDetail(queryClient, "chat-1", (chat) =>
			chat ? { ...chat, title: "Patched" } : chat,
		);

		expect(
			queryClient.getQueryData<TypesGen.Chat>(chatEntityKey("chat-1"))?.title,
		).toBe("Patched");
	});

	it("patchMessages preserves the previous reference when the updater is a no-op", () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(chatMessagesKey("chat-1"), makeMessages());
		const before = queryClient.getQueryData(chatMessagesKey("chat-1"));

		chatCache.patchMessages(queryClient, "chat-1", (data) => data);

		expect(queryClient.getQueryData(chatMessagesKey("chat-1"))).toBe(before);
	});
});
