import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { QueryClient } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { archiveChat, chatKey, chatsKey, unarchiveChat } from "./chats";

vi.mock("api/api", () => ({
	API: {
		archiveChat: vi.fn(),
		unarchiveChat: vi.fn(),
	},
}));

const makeChat = (
	id: string,
	overrides?: Partial<TypesGen.Chat>,
): TypesGen.Chat => ({
	id,
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	title: `Chat ${id}`,
	status: "running",
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	last_error: null,
	...overrides,
});

const makeChatWithMessages = (
	chatId: string,
	overrides?: Partial<TypesGen.Chat>,
): TypesGen.ChatWithMessages => ({
	chat: makeChat(chatId, overrides),
	messages: [],
	queued_messages: [],
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

describe("archiveChat optimistic update", () => {
	it("optimistically sets archived to true in the chats list", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialChats = [makeChat(chatId), makeChat("chat-2")];
		queryClient.setQueryData(chatsKey, initialChats);

		vi.mocked(API.archiveChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const updatedChats = queryClient.getQueryData<TypesGen.Chat[]>(chatsKey);
		expect(updatedChats).toHaveLength(2);
		expect(updatedChats?.find((c) => c.id === chatId)?.archived).toBe(true);
		// Other chats are unchanged.
		expect(updatedChats?.find((c) => c.id === "chat-2")?.archived).toBe(false);
	});

	it("optimistically sets archived to true in the individual chat cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);
		queryClient.setQueryData(chatKey(chatId), makeChatWithMessages(chatId));

		vi.mocked(API.archiveChat).mockResolvedValue();

		const mutation = archiveChat(queryClient);
		await mutation.onMutate(chatId);

		const cachedChat = queryClient.getQueryData<TypesGen.ChatWithMessages>(
			chatKey(chatId),
		);
		expect(cachedChat?.chat.archived).toBe(true);
	});

	it("rolls back the chats list on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const initialChats = [makeChat(chatId)];
		queryClient.setQueryData(chatsKey, initialChats);
		queryClient.setQueryData(chatKey(chatId), makeChatWithMessages(chatId));

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify the optimistic update took effect.
		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(chatsKey)?.[0].archived,
		).toBe(true);

		// Simulate an error — the onError handler should restore original
		// data.
		mutation.onError(new Error("server error"), chatId, context);

		const rolledBack = queryClient.getQueryData<TypesGen.Chat[]>(chatsKey);
		expect(rolledBack?.[0].archived).toBe(false);
	});

	it("rolls back the individual chat cache on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);
		queryClient.setQueryData(chatKey(chatId), makeChatWithMessages(chatId));

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.ChatWithMessages>(chatKey(chatId))?.chat
				.archived,
		).toBe(true);

		mutation.onError(new Error("server error"), chatId, context);

		const rolledBack = queryClient.getQueryData<TypesGen.ChatWithMessages>(
			chatKey(chatId),
		);
		expect(rolledBack?.chat.archived).toBe(false);
	});

	it("handles error rollback gracefully when context is undefined", () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId, { archived: true })]);

		const mutation = archiveChat(queryClient);

		// Calling onError with undefined context should not throw.
		expect(() => {
			mutation.onError(new Error("fail"), chatId, undefined);
		}).not.toThrow();

		// Data should remain unchanged since there was nothing to roll
		// back to.
		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(chatsKey)?.[0].archived,
		).toBe(true);
	});

	it("handles onMutate when no individual chat cache exists", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId)]);
		// Deliberately do NOT set chatKey(chatId) data.

		const mutation = archiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// The list should still be optimistically updated.
		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(chatsKey)?.[0].archived,
		).toBe(true);
		// previousChat should be undefined.
		expect(context?.previousChat).toBeUndefined();
	});

	it("invalidates queries on settled regardless of outcome", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = archiveChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatsKey,
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
		});
	});
});

describe("unarchiveChat optimistic update", () => {
	it("optimistically sets archived to false in the chats list", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId, { archived: true })]);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(chatsKey)?.[0].archived,
		).toBe(false);
	});

	it("optimistically sets archived to false in the individual chat cache", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chatKey(chatId),
			makeChatWithMessages(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		await mutation.onMutate(chatId);

		expect(
			queryClient.getQueryData<TypesGen.ChatWithMessages>(chatKey(chatId))?.chat
				.archived,
		).toBe(false);
	});

	it("rolls back both caches on error", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		queryClient.setQueryData(chatsKey, [makeChat(chatId, { archived: true })]);
		queryClient.setQueryData(
			chatKey(chatId),
			makeChatWithMessages(chatId, { archived: true }),
		);

		const mutation = unarchiveChat(queryClient);
		const context = await mutation.onMutate(chatId);

		// Verify optimistic update.
		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(chatsKey)?.[0].archived,
		).toBe(false);
		expect(
			queryClient.getQueryData<TypesGen.ChatWithMessages>(chatKey(chatId))?.chat
				.archived,
		).toBe(false);

		// Roll back.
		mutation.onError(new Error("server error"), chatId, context);

		expect(
			queryClient.getQueryData<TypesGen.Chat[]>(chatsKey)?.[0].archived,
		).toBe(true);
		expect(
			queryClient.getQueryData<TypesGen.ChatWithMessages>(chatKey(chatId))?.chat
				.archived,
		).toBe(true);
	});

	it("invalidates queries on settled", async () => {
		const queryClient = createTestQueryClient();
		const chatId = "chat-1";
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		const mutation = unarchiveChat(queryClient);
		await mutation.onSettled(undefined, undefined, chatId);

		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatsKey,
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: chatKey(chatId),
		});
	});
});
