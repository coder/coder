import { describe, expect, it } from "vitest";
import type { PendingAttachment } from "../components/ChatPageContent";
import {
	buildChatInputContent,
	buildPendingAttachmentsByID,
} from "./chatInputContent";

const mockImageAttachment: PendingAttachment = {
	fileId: "file-1",
	mediaType: "image/png",
	name: "image.png",
	size: 1,
};
const mockTextAttachment: PendingAttachment = {
	fileId: "file-2",
	mediaType: "text/plain",
	name: "notes.txt",
	size: 1,
};

describe("buildPendingAttachmentsByID", () => {
	it("returns undefined when attachments are missing or empty", () => {
		expect(buildPendingAttachmentsByID()).toBeUndefined();
		expect(buildPendingAttachmentsByID([])).toBeUndefined();
	});

	it("keys attachments by file ID", () => {
		expect(
			buildPendingAttachmentsByID([mockImageAttachment, mockTextAttachment]),
		).toEqual(
			new Map([
				["file-1", mockImageAttachment],
				["file-2", mockTextAttachment],
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

	it("sends the untrimmed message text when composer parts are omitted", () => {
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

	it("appends attachment file parts after text", () => {
		expect(
			buildChatInputContent({
				message: "see files",
				attachments: [mockImageAttachment, mockTextAttachment],
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
				attachments: [mockImageAttachment],
			}),
		).toEqual({
			content: [{ type: "file", file_id: "file-1" }],
			hasContent: true,
		});
	});

	it("sends staged workspace files as workspace-attachment parts", () => {
		expect(
			buildChatInputContent({
				message: "",
				attachments: [
					mockImageAttachment,
					{
						fileId: "file-3",
						mediaType: "application/zip",
						name: "bundle.zip",
						size: 1,
						purpose: "workspace",
					},
				],
			}).content,
		).toEqual([
			{ type: "file", file_id: "file-1" },
			{ type: "workspace-attachment", file_id: "file-3" },
		]);
	});

	it("appends workspace file references after attachments", () => {
		expect(
			buildChatInputContent({
				message: "",
				attachments: [mockImageAttachment],
				workspaceUploads: [
					{
						path: "/home/coder/uploads/data.zip",
						name: "data.zip",
						size: 42,
						mediaType: "",
						workspaceId: "ws-1",
					},
				],
			}),
		).toEqual({
			content: [
				{ type: "file", file_id: "file-1" },
				{
					type: "workspace-file-reference",
					workspace_file_path: "/home/coder/uploads/data.zip",
					workspace_file_name: "data.zip",
					workspace_file_size: 42,
					workspace_file_media_type: "application/octet-stream",
					workspace_file_workspace_id: "ws-1",
				},
			],
			hasContent: true,
		});
	});
});
