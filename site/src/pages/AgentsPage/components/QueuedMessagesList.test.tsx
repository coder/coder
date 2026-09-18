import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import {
	getFirstLinePreview,
	getQueuedMessageInfo,
	QueuedMessagesList,
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

describe("QueuedMessagesList", () => {
	const renderList = (
		messages: readonly ChatQueuedMessage[],
		handlers: Partial<
			Pick<
				React.ComponentProps<typeof QueuedMessagesList>,
				"onDelete" | "onPromote"
			>
		> = {},
	) => {
		const onDelete = vi.fn();
		const onPromote = vi.fn();
		render(
			<TooltipProvider>
				<QueuedMessagesList
					messages={messages}
					onDelete={onDelete}
					onPromote={onPromote}
					{...handlers}
				/>
			</TooltipProvider>,
		);
		return { onDelete, onPromote };
	};

	it.each([
		["Send now", "onPromote"],
		["Remove from queue", "onDelete"],
	] as const)("forwards %s with the row id", async (name, handler) => {
		const user = userEvent.setup();
		const handlers = renderList([{ ...MockChatQueuedMessage, id: 9 }]);
		await user.click(screen.getByRole("button", { name }));
		expect(handlers[handler]).toHaveBeenCalledWith(9);
	});

	it("hides the row and disables sibling actions while onPromote is pending, and restores them after it fails", async () => {
		const user = userEvent.setup();
		let rejectPromote: ((error: Error) => void) | undefined;
		const onPromote = vi.fn(
			() =>
				new Promise<void>((_, reject) => {
					rejectPromote = reject;
				}),
		);
		const { onDelete } = renderList(
			[
				{ ...MockChatQueuedMessage, id: 7 },
				{ ...MockChatQueuedMessage, id: 8 },
			],
			{ onPromote },
		);

		const [sendHead] = screen.getAllByRole("button", { name: "Send now" });
		await user.click(sendHead);
		expect(onPromote).toHaveBeenCalledWith(7);
		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).toHaveBeenCalledTimes(1);

		const failPromote = rejectPromote;
		if (!failPromote) {
			throw new Error("onPromote was not invoked");
		}
		await act(async () => {
			failPromote(new Error("promote failed"));
		});

		const [removeHead] = screen.getAllByRole("button", {
			name: "Remove from queue",
		});
		await user.click(removeHead);
		expect(onDelete).toHaveBeenCalledWith(7);
	});
});
