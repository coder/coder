import { MutationObserver, QueryClient } from "react-query";
import { afterEach, describe, expect, it, type MockInstance, vi } from "vitest";
import { API } from "#/api/api";
import {
	applyWatchedChatArchived,
	chatListFamilyKey,
	chatListKey,
	toChatListParams,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { boardChats, boardChatsKey, updateChatLabels } from "./boardChats";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	labels,
});

const boardPage = (chats: Chat[]) => ({ pages: [chats], pageParams: [0] });

/** Runs the mutation the way useMutation does, without a component. */
const write = (
	queryClient: QueryClient,
	chatId: string,
	after?: Promise<unknown>,
) =>
	new MutationObserver(queryClient, {
		...updateChatLabels(queryClient),
		retry: false,
	}).mutate({ chatId, labels: { "board/column": "Doing" }, after });

const listInvalidations = (
	invalidate: MockInstance<QueryClient["invalidateQueries"]>,
) =>
	invalidate.mock.calls.filter(
		([filters]) => filters?.queryKey === chatListFamilyKey,
	).length;

describe("boardChats", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("pages through every unarchived chat into one page", async () => {
		const full = Array.from({ length: 200 }, (_, i) => chat(`a${i}`));
		const spy = vi
			.spyOn(API.experimental, "getChats")
			.mockResolvedValueOnce(full)
			.mockResolvedValueOnce([chat("last")]);
		const data = await new QueryClient().fetchInfiniteQuery(boardChats());
		expect(data.pages).toHaveLength(1);
		expect(data.pages[0]).toHaveLength(201);
		expect(spy).toHaveBeenNthCalledWith(1, {
			limit: 200,
			offset: 0,
			q: "archived:false",
		});
		expect(spy).toHaveBeenNthCalledWith(2, {
			limit: 200,
			offset: 200,
			q: "archived:false",
		});
	});

	it("lives under the chat list family so list helpers reach it", () => {
		expect(boardChatsKey.slice(0, chatListFamilyKey.length)).toEqual(
			chatListFamilyKey,
		);
		expect(boardChatsKey).not.toEqual(chatListKey(toChatListParams()));
	});

	it("drops an archived chat like the sidebar's unarchived list does", () => {
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a"), chat("b")]));
		applyWatchedChatArchived(queryClient, { ...chat("a"), archived: true });
		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("b")]),
		);
	});

	it("relabels the board cache before the request", async () => {
		const request = createDeferred<void>();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a"), chat("b")]));

		const pending = write(queryClient, "b");
		await vi.waitFor(() =>
			expect(queryClient.getQueryData(boardChatsKey)).toEqual(
				boardPage([chat("a"), chat("b", { "board/column": "Doing" })]),
			),
		);

		request.resolve();
		await pending;
	});

	it("invalidates the list once, after the last of two overlapping writes settles", async () => {
		const first = createDeferred<void>();
		const second = createDeferred<void>();
		vi.spyOn(API.experimental, "updateChat").mockImplementation((chatId) =>
			chatId === "a" ? first.promise : second.promise,
		);
		const queryClient = new QueryClient();
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a"), chat("b")]));

		const writes = [write(queryClient, "a"), write(queryClient, "b")];
		first.resolve();
		await writes[0];
		expect(listInvalidations(invalidate)).toBe(0);

		second.resolve();
		await Promise.all(writes);
		expect(listInvalidations(invalidate)).toBe(1);
		expect(invalidate).toHaveBeenCalledTimes(3);
	});

	it("invalidates the list when two writes settle in the same tick", async () => {
		const first = createDeferred<void>();
		const second = createDeferred<void>();
		vi.spyOn(API.experimental, "updateChat").mockImplementation((chatId) =>
			chatId === "a" ? first.promise : second.promise,
		);
		const queryClient = new QueryClient();
		const invalidate = vi.spyOn(queryClient, "invalidateQueries");

		const writes = [write(queryClient, "a"), write(queryClient, "b")];
		first.resolve();
		second.resolve();
		await Promise.all(writes);

		expect(listInvalidations(invalidate)).toBe(1);
	});

	it("sends no request for a write whose gate rejected", async () => {
		const spy = vi.spyOn(API.experimental, "updateChat");
		const queryClient = new QueryClient();

		await expect(
			write(queryClient, "a", Promise.reject(new Error("receiver failed"))),
		).resolves.toBeUndefined();

		expect(spy).not.toHaveBeenCalled();
	});
});
