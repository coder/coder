import type { InfiniteData } from "react-query";
import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { mergeMessagesIntoInfiniteCache } from "./messageCache";

type ChatMessagesInfiniteData = InfiniteData<TypesGen.ChatMessagesResponse>;

const makeMessage = (
	id: number,
	text: string,
	overrides: Partial<TypesGen.ChatMessage> = {},
): TypesGen.ChatMessage => {
	const base: TypesGen.ChatMessage = {
		id,
		chat_id: "chat-1",
		created_at: `2025-01-01T00:00:00.${String(id).padStart(3, "0")}Z`,
		role: "assistant",
		content: [{ type: "text", text }],
	};
	return { ...base, ...overrides };
};

const makeQueuedMessage = (id: number): TypesGen.ChatQueuedMessage => ({
	id,
	chat_id: "chat-1",
	created_at: `2025-01-01T00:00:00.${String(id).padStart(3, "0")}Z`,
	content: [{ type: "text", text: "queued" }],
});

const makePage = (
	messages: readonly TypesGen.ChatMessage[],
	overrides: Partial<TypesGen.ChatMessagesResponse> = {},
): TypesGen.ChatMessagesResponse => ({
	messages,
	queued_messages: [],
	has_more: false,
	...overrides,
});

const makeCache = (
	pages: readonly TypesGen.ChatMessagesResponse[],
	pageParams: readonly unknown[] = pages.map((_, index) => index),
): ChatMessagesInfiniteData => ({
	pages: [...pages],
	pageParams: [...pageParams],
});

describe("mergeMessagesIntoInfiniteCache", () => {
	it("synthesizes a first page for an empty cache with incoming messages", () => {
		const older = makeMessage(1, "older");
		const newer = makeMessage(2, "newer");
		const updatedOlder = makeMessage(1, "updated older");

		const result = mergeMessagesIntoInfiniteCache(undefined, [
			older,
			newer,
			updatedOlder,
		]);

		expect(result.pageParams).toEqual([undefined]);
		expect(result.pages).toHaveLength(1);
		expect(result.pages[0]).toEqual({
			messages: [newer, updatedOlder],
			queued_messages: [],
			has_more: true,
		});
	});

	it("synthesizes an empty first page for an empty cache with no incoming messages", () => {
		const result = mergeMessagesIntoInfiniteCache(undefined, []);

		expect(result).toEqual({
			pageParams: [undefined],
			pages: [
				{
					messages: [],
					queued_messages: [],
					has_more: true,
				},
			],
		});
	});

	it("merges incoming messages into page zero and leaves later pages untouched", () => {
		const newest = makeMessage(3, "newest");
		const oldest = makeMessage(1, "oldest");
		const incoming = makeMessage(2, "incoming");
		const firstPage = makePage([newest, oldest], {
			queued_messages: [makeQueuedMessage(1)],
			has_more: true,
		});
		const secondPage = makePage([makeMessage(0, "previous page")]);
		const prev = makeCache([firstPage, secondPage], [undefined, 1]);

		const result = mergeMessagesIntoInfiniteCache(prev, [incoming]);

		expect(result).not.toBe(prev);
		expect(result.pageParams).toBe(prev.pageParams);
		expect(result.pages[0]).toEqual({
			...firstPage,
			messages: [newest, incoming, oldest],
		});
		expect(result.pages[1]).toBe(secondPage);
	});

	it("lets incoming messages replace existing messages with the same id", () => {
		const stale = makeMessage(2, "stale");
		const updated = makeMessage(2, "updated");
		const older = makeMessage(1, "older");
		const prev = makeCache([makePage([stale, older])]);

		const result = mergeMessagesIntoInfiniteCache(prev, [updated]);

		expect(result.pages[0].messages).toEqual([updated, older]);
	});
});
