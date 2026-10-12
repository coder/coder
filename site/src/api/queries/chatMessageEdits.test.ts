import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { buildOptimisticEditedMessage } from "./chatMessageEdits";

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
