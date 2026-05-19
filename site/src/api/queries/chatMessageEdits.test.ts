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

	it("projects workspace-file parts with provided metadata", () => {
		const message = buildOptimisticEditedMessage({
			requestContent: [
				{
					type: "workspace-file-reference",
					workspace_file_path: "/home/coder/.coder/chats/abc/files/data.csv",
					workspace_file_name: "data.csv",
					workspace_file_size: 1024,
					workspace_file_media_type: "text/csv",
				},
			],
			originalMessage: makeUserMessage(),
		});

		expect(message.content).toEqual([
			{
				type: "workspace-file-reference",
				workspace_file_path: "/home/coder/.coder/chats/abc/files/data.csv",
				workspace_file_name: "data.csv",
				workspace_file_size: 1024,
				workspace_file_media_type: "text/csv",
			},
		]);
	});

	it("defaults workspace-file fields when omitted on the request part", () => {
		const message = buildOptimisticEditedMessage({
			requestContent: [{ type: "workspace-file-reference" }],
			originalMessage: makeUserMessage(),
		});

		expect(message.content).toEqual([
			{
				type: "workspace-file-reference",
				workspace_file_path: "",
				workspace_file_name: "",
				workspace_file_size: 0,
				workspace_file_media_type: undefined,
			},
		]);
	});
});
