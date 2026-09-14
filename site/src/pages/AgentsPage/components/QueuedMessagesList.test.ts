import { describe, expect, it } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import { getQueuedMessageInfo } from "./QueuedMessagesList";

const buildMessage = (
	content: ChatQueuedMessage["content"],
): ChatQueuedMessage => ({ ...MockChatQueuedMessage, content });

describe("getQueuedMessageInfo", () => {
	it("returns text for a text-only message", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "text", text: "hello" }]),
		);
		expect(result).toEqual({
			displayText: "hello",
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("collects hook notices without polluting the preview text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "hello" },
				{ type: "hook-notice", text: "policy notice" },
			]),
		);
		expect(result).toEqual({
			displayText: "hello",
			attachmentCount: 0,
			hookNotices: ["policy notice"],
		});
	});

	it("preserves multi-line text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "text", text: "line1\nline2" }]),
		);
		expect(result).toEqual({
			displayText: "line1\nline2",
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("returns attachment label for a single file", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "file", file_id: "a", media_type: "image/png" }]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			attachmentCount: 1,
			hookNotices: [],
		});
	});

	it("returns attachment label for multiple files", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "file", file_id: "a", media_type: "image/png" },
				{ type: "file", file_id: "b", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			attachmentCount: 2,
			hookNotices: [],
		});
	});

	it("returns text with attachment count for text + file", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "look" },
				{ type: "file", file_id: "a", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			displayText: "look",
			attachmentCount: 1,
			hookNotices: [],
		});
	});

	it("returns fallback for empty content", () => {
		const result = getQueuedMessageInfo(buildMessage([]));
		expect(result).toEqual({
			displayText: "[Queued message]",
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("returns fallback for whitespace-only text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "text", text: "  " }]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("returns attachment label for whitespace text + file", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: " " },
				{ type: "file", file_id: "a", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			displayText: "[Queued message]",
			attachmentCount: 1,
			hookNotices: [],
		});
	});

	it("joins multiple text parts with a space", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "a" },
				{ type: "text", text: "b" },
			]),
		);
		expect(result).toEqual({
			displayText: "a b",
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("counts multiple file attachments alongside text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "check this" },
				{ type: "file", file_id: "img-1", media_type: "image/png" },
				{ type: "file", file_id: "doc-2", media_type: "application/pdf" },
			]),
		);
		expect(result).toEqual({
			displayText: "check this",
			attachmentCount: 2,
			hookNotices: [],
		});
	});
});
