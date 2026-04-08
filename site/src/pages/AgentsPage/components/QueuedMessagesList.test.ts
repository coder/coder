import { describe, expect, it } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { getQueuedMessageInfo } from "./QueuedMessagesList";

const makeMessage = (
	content: ChatQueuedMessage["content"],
): ChatQueuedMessage => ({
	id: 1,
	chat_id: "c",
	content,
	created_at: "",
});

describe("getQueuedMessageInfo", () => {
	it("returns text for a text-only message", () => {
		const result = getQueuedMessageInfo(
			makeMessage([{ type: "text", text: "hello" }]),
		);
		expect(result).toEqual({
			displayText: "hello",
			rawText: "hello",
			attachmentCount: 0,
			fileBlocks: [],
		});
	});

	it("preserves multi-line text", () => {
		const result = getQueuedMessageInfo(
			makeMessage([{ type: "text", text: "line1\nline2" }]),
		);
		expect(result).toEqual({
			displayText: "line1\nline2",
			rawText: "line1\nline2",
			attachmentCount: 0,
			fileBlocks: [],
		});
	});

	it("returns attachment label for a single file", () => {
		const result = getQueuedMessageInfo(
			makeMessage([{ type: "file", file_id: "a", media_type: "image/png" }]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			rawText: "",
			attachmentCount: 1,
			fileBlocks: [{ type: "file", file_id: "a", media_type: "image/png" }],
		});
	});

	it("returns attachment label for multiple files", () => {
		const result = getQueuedMessageInfo(
			makeMessage([
				{ type: "file", file_id: "a", media_type: "image/png" },
				{ type: "file", file_id: "b", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			rawText: "",
			attachmentCount: 2,
			fileBlocks: [
				{ type: "file", file_id: "a", media_type: "image/png" },
				{ type: "file", file_id: "b", media_type: "image/png" },
			],
		});
	});

	it("returns text with attachment count for text + file", () => {
		const result = getQueuedMessageInfo(
			makeMessage([
				{ type: "text", text: "look" },
				{ type: "file", file_id: "a", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			displayText: "look",
			rawText: "look",
			attachmentCount: 1,
			fileBlocks: [{ type: "file", file_id: "a", media_type: "image/png" }],
		});
	});

	it("returns fallback for empty content", () => {
		const result = getQueuedMessageInfo(makeMessage([]));
		expect(result).toEqual({
			displayText: "[Queued message]",
			rawText: "",
			attachmentCount: 0,
			fileBlocks: [],
		});
	});

	it("returns fallback for whitespace-only text", () => {
		const result = getQueuedMessageInfo(
			makeMessage([{ type: "text", text: "  " }]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			rawText: "",
			attachmentCount: 0,
			fileBlocks: [],
		});
	});

	it("returns attachment label for whitespace text + file", () => {
		const result = getQueuedMessageInfo(
			makeMessage([
				{ type: "text", text: " " },
				{ type: "file", file_id: "a", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			rawText: "",
			attachmentCount: 1,
			fileBlocks: [{ type: "file", file_id: "a", media_type: "image/png" }],
		});
	});

	it("joins multiple text parts with a space", () => {
		const result = getQueuedMessageInfo(
			makeMessage([
				{ type: "text", text: "a" },
				{ type: "text", text: "b" },
			]),
		);
		expect(result).toEqual({
			displayText: "a b",
			rawText: "a b",
			attachmentCount: 0,
			fileBlocks: [],
		});
	});

	it("preserves media_type from file parts", () => {
		const result = getQueuedMessageInfo(
			makeMessage([
				{ type: "text", text: "check this" },
				{ type: "file", file_id: "img-1", media_type: "image/png" },
				{ type: "file", file_id: "doc-2", media_type: "application/pdf" },
			]),
		);
		expect(result).toEqual({
			displayText: "check this",
			rawText: "check this",
			attachmentCount: 2,
			fileBlocks: [
				{ type: "file", file_id: "img-1", media_type: "image/png" },
				{ type: "file", file_id: "doc-2", media_type: "application/pdf" },
			],
		});
	});
});
