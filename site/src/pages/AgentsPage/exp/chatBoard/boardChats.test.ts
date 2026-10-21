import { MutationObserver, QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	applyWatchedChatArchived,
	chatListFamilyKey,
	chatListKey,
	toChatListParams,
	updateChatTitle,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import {
	boardChats,
	boardChatsKey,
	boardWriteScope,
	updateChatLabels,
} from "./boardChats";

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
	labels: Record<string, string> = { "board/column": "Doing" },
) =>
	new MutationObserver(queryClient, {
		...updateChatLabels(queryClient),
		retry: false,
	}).mutate({ chatId, labels, after });

const rename = (queryClient: QueryClient, chatId: string, title: string) =>
	new MutationObserver(queryClient, {
		...updateChatTitle(queryClient),
		scope: boardWriteScope,
		retry: false,
	}).mutate({ chatId, title });

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
		expect(spy).toHaveBeenNthCalledWith(
			1,
			{ limit: 200, offset: 0, q: "archived:false" },
			expect.any(AbortSignal),
		);
		expect(spy).toHaveBeenNthCalledWith(
			2,
			{ limit: 200, offset: 200, q: "archived:false" },
			expect.any(AbortSignal),
		);
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
		const request: Deferred<void> = createDeferred();
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

	it("keeps the board's labels on a refetch while a write is queued", async () => {
		const request: Deferred<void> = createDeferred();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const staleList = createDeferred<Chat[]>();
		vi.spyOn(API.experimental, "getChats")
			.mockReturnValueOnce(staleList.promise)
			.mockResolvedValueOnce([chat("a", { "board/column": "Done" })]);
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a")]));
		const refetch = () =>
			queryClient.fetchInfiniteQuery({ ...boardChats(), staleTime: 0 });

		const pending = write(queryClient, "a");
		await vi.waitFor(() =>
			expect(queryClient.getQueryData(boardChatsKey)).toEqual(
				boardPage([chat("a", { "board/column": "Doing" })]),
			),
		);
		const midQueue = refetch();
		staleList.resolve([chat("a"), chat("new")]);
		await midQueue;
		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("a", { "board/column": "Doing" }), chat("new")]),
		);

		request.resolve();
		await pending;
		await refetch();
		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("a", { "board/column": "Done" })]),
		);
	});

	it("aborts a board refetch in flight when a write starts", async () => {
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
		let signal: AbortSignal | undefined;
		vi.spyOn(API.experimental, "getChats").mockImplementationOnce((_req, s) => {
			signal = s;
			return new Promise(() => {});
		});
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a")]));

		void queryClient
			.fetchInfiniteQuery({ ...boardChats(), staleTime: 0 })
			.catch(() => undefined);
		await vi.waitFor(() => expect(signal).toBeDefined());
		await write(queryClient, "a");

		expect(signal?.aborted).toBe(true);
		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("a", { "board/column": "Doing" })]),
		);
	});

	it("keeps the board's labels from a response requested before a write settled", async () => {
		const request: Deferred<void> = createDeferred();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const staleList = createDeferred<Chat[]>();
		vi.spyOn(API.experimental, "getChats").mockReturnValueOnce(
			staleList.promise,
		);
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a")]));

		const pending = write(queryClient, "a");
		await vi.waitFor(() =>
			expect(queryClient.getQueryData(boardChatsKey)).toEqual(
				boardPage([chat("a", { "board/column": "Doing" })]),
			),
		);
		const inFlight = queryClient.fetchInfiniteQuery({
			...boardChats(),
			staleTime: 0,
		});
		request.resolve();
		await pending;
		staleList.resolve([chat("a")]);
		await inFlight;

		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("a", { "board/column": "Doing" })]),
		);
	});

	it("sends board writes to the server one at a time in call order", async () => {
		const requests: Array<Deferred<void>> = [];
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockImplementation(() => {
				const request: Deferred<void> = createDeferred();
				requests.push(request);
				return request.promise;
			});
		const queryClient = new QueryClient();

		// A transfer (source gated on the receiver), a later source write,
		// and a rename.
		const received = write(queryClient, "r", undefined, { "board/n": "1" });
		const writes = [
			received,
			write(queryClient, "s", received, {}),
			write(queryClient, "s", undefined, { "board/column": "Done" }),
			rename(queryClient, "s", "Renamed"),
		];
		const expected = [
			["r", { labels: { "board/n": "1" } }],
			["s", { labels: {} }],
			["s", { labels: { "board/column": "Done" } }],
			["s", { title: "Renamed" }],
		];
		for (let i = 0; i < expected.length; i++) {
			await vi.waitFor(() => expect(spy).toHaveBeenCalledTimes(i + 1));
			expect(spy.mock.calls).toEqual(expected.slice(0, i + 1));
			requests[i].resolve();
		}
		await Promise.all(writes);
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
