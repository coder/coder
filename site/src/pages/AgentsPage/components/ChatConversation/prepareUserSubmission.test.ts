import { describe, expect, it } from "vitest";
import { prepareUserSubmission } from "./prepareUserSubmission";

describe("prepareUserSubmission", () => {
	it("builds matching request and optimistic payloads for text and file references", () => {
		const submission = prepareUserSubmission({
			editorParts: [
				{ type: "text", text: "Review this" },
				{
					type: "file-reference",
					reference: {
						fileName: "main.go",
						startLine: 1,
						endLine: 10,
						content: 'fmt.Println("hi")',
					},
				},
			],
			attachments: [],
			uploadStates: new Map(),
		});

		expect(submission.requestContent).toEqual([
			{ type: "text", text: "Review this" },
			{
				type: "file-reference",
				file_name: "main.go",
				start_line: 1,
				end_line: 10,
				content: 'fmt.Println("hi")',
			},
		]);
		expect(submission.optimisticContent).toEqual(submission.requestContent);
		expect(submission.skippedAttachmentErrors).toBe(0);
	});

	it("preserves uploaded attachment metadata in optimistic content", () => {
		const image = new File(["image"], "diagram.png", { type: "image/png" });
		const uploadStates = new Map([
			[image, { status: "uploaded" as const, fileId: "file-123" }],
		]);

		const submission = prepareUserSubmission({
			editorParts: [{ type: "text", text: "See attached" }],
			attachments: [image],
			uploadStates,
		});

		expect(submission.requestContent).toEqual([
			{ type: "text", text: "See attached" },
			{ type: "file", file_id: "file-123" },
		]);
		expect(submission.optimisticContent).toEqual([
			{ type: "text", text: "See attached" },
			{ type: "file", file_id: "file-123", media_type: "image/png" },
		]);
	});

	it("preserves edit-mode inline file blocks when no uploaded file id exists", () => {
		const textAttachment = new File([], "notes.txt", { type: "text/plain" });
		const inlineFileBlock = {
			type: "file" as const,
			media_type: "text/plain",
			data: "notes",
		};

		const submission = prepareUserSubmission({
			editorParts: [{ type: "text", text: "Updated notes" }],
			attachments: [textAttachment],
			uploadStates: new Map(),
			editingFileBlocks: [inlineFileBlock],
		});

		expect(submission.requestContent).toEqual([
			{ type: "text", text: "Updated notes" },
		]);
		expect(submission.optimisticContent).toEqual([
			{ type: "text", text: "Updated notes" },
			inlineFileBlock,
		]);
	});

	it("keeps inline fallback attachments aligned after earlier removals", () => {
		const remainingAttachment = new File([], "attachment-1.txt", {
			type: "text/plain",
		});
		const firstInlineFileBlock = {
			type: "file" as const,
			media_type: "text/plain",
			data: "first notes",
		};
		const secondInlineFileBlock = {
			type: "file" as const,
			media_type: "text/plain",
			data: "second notes",
		};

		const submission = prepareUserSubmission({
			editorParts: [{ type: "text", text: "Updated notes" }],
			attachments: [remainingAttachment],
			uploadStates: new Map(),
			editingFileBlocks: [firstInlineFileBlock, secondInlineFileBlock],
		});

		expect(submission.optimisticContent).toEqual([
			{ type: "text", text: "Updated notes" },
			secondInlineFileBlock,
		]);
	});

	it("counts failed attachments without adding them to either payload", () => {
		const broken = new File(["bad"], "broken.png", { type: "image/png" });
		const uploadStates = new Map([
			[broken, { status: "error" as const, error: "failed" }],
		]);

		const submission = prepareUserSubmission({
			editorParts: [{ type: "text", text: "hello" }],
			attachments: [broken],
			uploadStates,
		});

		expect(submission.requestContent).toEqual([
			{ type: "text", text: "hello" },
		]);
		expect(submission.optimisticContent).toEqual([
			{ type: "text", text: "hello" },
		]);
		expect(submission.skippedAttachmentErrors).toBe(1);
	});
});
