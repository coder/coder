import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	applyWatchedChatArchived,
	chatListFamilyKey,
	chatListKey,
	toChatListParams,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { boardChats, boardChatsKey, updateChatLabels } from "./boardChats";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	labels,
});

const boardPage = (chats: Chat[]) => ({ pages: [chats], pageParams: [0] });

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

	it("is patched by the shared infinite list updater", () => {
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a"), chat("b")]));
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((c) => (c.id === "a" ? { ...c, labels: { k: "v" } } : c)),
		);
		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("a", { k: "v" }), chat("b")]),
		);
	});

	it("relabels the board cache before the request", async () => {
		const queryClient = new QueryClient();
		queryClient.setQueryData(boardChatsKey, boardPage([chat("a"), chat("b")]));
		await updateChatLabels(queryClient).onMutate({
			chatId: "b",
			labels: { "board/column": "Doing" },
		});
		expect(queryClient.getQueryData(boardChatsKey)).toEqual(
			boardPage([chat("a"), chat("b", { "board/column": "Doing" })]),
		);
	});
});
