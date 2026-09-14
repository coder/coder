import type { InfiniteData } from "react-query";
import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	buildOptimisticEditedMessage,
	reconcileEditedMessageInCache,
} from "./chatMessageEdits";

const makeUserMessage = (
	content: readonly TypesGen.ChatMessagePart[] = [
		{ type: "text", text: "original" },
	],
): TypesGen.ChatMessage => ({
	id: 1,
	chat_id: "chat-1",
	created_at: "2025-01-01T00:00:00.000Z",
	role: "user",
	content,
});

describe("buildOptimisticEditedMessage", () => {
	it("preserves image MIME types for newly attached files", () => {
		const message = buildOptimisticEditedMessage({
			requestContent: [{ type: "file", file_id: "image-1" }],
			originalMessage: makeUserMessage(),
			attachmentMediaTypes: new Map([["image-1", "image/png"]]),
		});

		expect(message.content).toEqual([
			{ type: "file", file_id: "image-1", media_type: "image/png" },
		]);
	});

	it("reuses existing file parts before local attachment metadata", () => {
		const existingFilePart: TypesGen.ChatFilePart = {
			type: "file",
			file_id: "existing-1",
			media_type: "image/jpeg",
		};
		const message = buildOptimisticEditedMessage({
			requestContent: [{ type: "file", file_id: "existing-1" }],
			originalMessage: makeUserMessage([existingFilePart]),
			attachmentMediaTypes: new Map([["existing-1", "text/plain"]]),
		});

		expect(message.content).toEqual([existingFilePart]);
	});
});

describe("reconcileEditedMessageInCache", () => {
	it("drops messages the edit deleted, such as stale hook notices", () => {
		const staleNotice: TypesGen.ChatMessage = {
			id: 2,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:00:00.000Z",
			role: "system",
			content: [{ type: "text", text: "old hook notice" }],
		};
		const newNotice: TypesGen.ChatMessage = {
			id: 5,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:01:00.000Z",
			role: "system",
			content: [{ type: "text", text: "new hook notice" }],
		};
		const replacement: TypesGen.ChatMessage = {
			id: 6,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:01:00.000Z",
			role: "user",
			content: [{ type: "text", text: "edited prompt" }],
		};
		const currentData: InfiniteData<TypesGen.ChatMessagesResponse> = {
			pages: [
				{
					messages: [staleNotice, makeUserMessage()],
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		};

		const reconciled = reconcileEditedMessageInCache({
			currentData,
			optimisticMessageId: 1,
			responseMessages: [newNotice, replacement],
			deletedMessageIds: [staleNotice.id, 1],
		});

		const ids = reconciled?.pages[0]?.messages.map((message) => message.id);
		// Reversed from responseMessages: the first page is newest first.
		expect(ids).toEqual([replacement.id, newNotice.id]);
	});
});
