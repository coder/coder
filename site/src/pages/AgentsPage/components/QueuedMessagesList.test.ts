import { describe, expect, it } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import {
	getFirstLinePreview,
	getQueuedMessageInfo,
} from "./QueuedMessagesList";

const buildMessage = (
	content: ChatQueuedMessage["content"],
): ChatQueuedMessage => ({ ...MockChatQueuedMessage, content });

describe("getQueuedMessageInfo", () => {
	it("returns text for a text-only message", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "text", text: "hello" }]),
		);
		expect(result).toEqual({
			previewParts: [{ type: "text", text: "hello" }],
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("keeps invoked skills in document order", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{
					type: "skill-invocation",
					skill_source: "project",
					skill_name: "triage",
				},
				{ type: "text", text: "the flaky test" },
			]),
		);
		expect(result.previewParts).toEqual([
			{
				type: "skill-invocation",
				skill_source: "project",
				skill_name: "triage",
			},
			{ type: "text", text: "the flaky test" },
		]);
	});

	it("collects hook notices without polluting the preview text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "hello" },
				{ type: "hook-notice", text: "policy notice" },
			]),
		);
		expect(result).toEqual({
			previewParts: [{ type: "text", text: "hello" }],
			attachmentCount: 0,
			hookNotices: ["policy notice"],
		});
	});

	it("preserves multi-line text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "text", text: "line1\nline2" }]),
		);
		expect(result).toEqual({
			previewParts: [{ type: "text", text: "line1\nline2" }],
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("returns no preview for a single file", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "file", file_id: "a", media_type: "image/png" }]),
		);
		expect(result).toEqual({
			previewParts: [],
			attachmentCount: 1,
			hookNotices: [],
		});
	});

	it("returns no preview for multiple files", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "file", file_id: "a", media_type: "image/png" },
				{ type: "file", file_id: "b", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			previewParts: [],
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
			previewParts: [{ type: "text", text: "look" }],
			attachmentCount: 1,
			hookNotices: [],
		});
	});

	it("returns no preview for empty content", () => {
		const result = getQueuedMessageInfo(buildMessage([]));
		expect(result).toEqual({
			previewParts: [],
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("returns no preview for whitespace-only text", () => {
		const result = getQueuedMessageInfo(
			buildMessage([{ type: "text", text: "  " }]),
		);
		expect(result).toEqual({
			previewParts: [],
			attachmentCount: 0,
			hookNotices: [],
		});
	});

	it("returns no preview for whitespace text + file", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: " " },
				{ type: "file", file_id: "a", media_type: "image/png" },
			]),
		);
		expect(result).toEqual({
			previewParts: [],
			attachmentCount: 1,
			hookNotices: [],
		});
	});

	it("keeps each text part", () => {
		const result = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "a" },
				{ type: "text", text: "b" },
			]),
		);
		expect(result).toEqual({
			previewParts: [
				{ type: "text", text: "a" },
				{ type: "text", text: "b" },
			],
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
			previewParts: [{ type: "text", text: "check this" }],
			attachmentCount: 2,
			hookNotices: [],
		});
	});
});

describe("getFirstLinePreview", () => {
	it("ends the first line at a break before a skill chip", () => {
		const { previewParts } = getQueuedMessageInfo(
			buildMessage([
				{ type: "text", text: "Fix the build\n" },
				{
					type: "skill-invocation",
					skill_source: "project",
					skill_name: "triage",
				},
			]),
		);
		expect(getFirstLinePreview(previewParts)).toEqual({
			parts: [{ type: "text", text: "Fix the build" }],
			isTruncated: true,
		});
	});

	it("ends the first line at a line break between skill chips", () => {
		const skillPart = (name: string) =>
			({
				type: "skill-invocation",
				skill_source: "project",
				skill_name: name,
			}) as const;
		const { previewParts } = getQueuedMessageInfo(
			buildMessage([
				skillPart("triage"),
				{ type: "text", text: "\n" },
				skillPart("review"),
			]),
		);
		expect(getFirstLinePreview(previewParts)).toEqual({
			parts: [skillPart("triage")],
			isTruncated: true,
		});
	});
});
