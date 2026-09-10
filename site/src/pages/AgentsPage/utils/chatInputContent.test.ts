import { describe, expect, it } from "vitest";
import {
	buildAttachmentMediaTypes,
	buildChatInputContent,
} from "./chatInputContent";

describe("buildAttachmentMediaTypes", () => {
	it("returns undefined when attachments are missing or empty", () => {
		expect(buildAttachmentMediaTypes()).toBeUndefined();
		expect(buildAttachmentMediaTypes([])).toBeUndefined();
	});

	it("maps file IDs to media types", () => {
		expect(
			buildAttachmentMediaTypes([
				{ fileId: "a", mediaType: "image/png" },
				{ fileId: "b", mediaType: "text/plain" },
			]),
		).toEqual(
			new Map([
				["a", "image/png"],
				["b", "text/plain"],
			]),
		);
	});
});

describe("buildChatInputContent", () => {
	it("has no content when message, composer, and attachments are empty", () => {
		expect(buildChatInputContent({ message: "   " })).toEqual({
			content: [],
			hasContent: false,
		});
	});

	it("sends trimmed message text when composer parts are omitted", () => {
		expect(buildChatInputContent({ message: "  hello  " })).toEqual({
			content: [{ type: "text", text: "  hello  " }],
			hasContent: true,
		});
	});

	it("walks composer parts in document order and skips blank text", () => {
		expect(
			buildChatInputContent({
				message: "fallback",
				composerParts: [
					{ type: "text", text: "before" },
					{ type: "text", text: "   " },
					{
						type: "file-reference",
						reference: {
							fileName: "a.ts",
							startLine: 2,
							endLine: 4,
							content: "code",
						},
					},
					{ type: "text", text: "after" },
				],
			}),
		).toEqual({
			content: [
				{ type: "text", text: "before" },
				{
					type: "file-reference",
					file_name: "a.ts",
					start_line: 2,
					end_line: 4,
					content: "code",
				},
				{ type: "text", text: "after" },
			],
			hasContent: true,
		});
	});

	it("falls back to message text when composer parts yield nothing", () => {
		expect(
			buildChatInputContent({
				message: "typed",
				composerParts: [{ type: "text", text: "   " }],
			}),
		).toEqual({
			content: [{ type: "text", text: "typed" }],
			hasContent: true,
		});
	});

	it("does not mix composer file references into a message-only send", () => {
		expect(
			buildChatInputContent({
				message: "Implement the plan.",
			}),
		).toEqual({
			content: [{ type: "text", text: "Implement the plan." }],
			hasContent: true,
		});
	});

	it("appends attachment file parts after text", () => {
		expect(
			buildChatInputContent({
				message: "see files",
				attachments: [
					{ fileId: "file-1", mediaType: "image/png" },
					{ fileId: "file-2", mediaType: "text/plain" },
				],
			}),
		).toEqual({
			content: [
				{ type: "text", text: "see files" },
				{ type: "file", file_id: "file-1" },
				{ type: "file", file_id: "file-2" },
			],
			hasContent: true,
		});
	});

	it("treats attachments alone as content", () => {
		expect(
			buildChatInputContent({
				message: "  ",
				attachments: [{ fileId: "file-1", mediaType: "image/png" }],
			}),
		).toEqual({
			content: [{ type: "file", file_id: "file-1" }],
			hasContent: true,
		});
	});
});
