import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	MockChatQueuedMessage,
	MockHeldChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { getQueuedMessageInfo, QueuedMessagesList } from "./QueuedMessagesList";

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

describe("QueuedMessagesList", () => {
	const renderList = (
		messages: readonly ChatQueuedMessage[],
		handlers: Partial<
			Pick<
				ComponentProps<typeof QueuedMessagesList>,
				"onDelete" | "onPromote" | "onEdit" | "onResume"
			>
		> = {},
	) => {
		const onDelete = vi.fn();
		const onPromote = vi.fn();
		const onEdit = vi.fn();
		const onResume = vi.fn();
		render(
			<TooltipProvider>
				<QueuedMessagesList
					messages={messages}
					onDelete={onDelete}
					onPromote={onPromote}
					onEdit={onEdit}
					onResume={onResume}
					{...handlers}
				/>
			</TooltipProvider>,
		);
		return { onDelete, onPromote, onEdit, onResume };
	};

	it("calls onEdit with the row id for an unheld row", async () => {
		const user = userEvent.setup();
		const { onEdit, onResume } = renderList([
			{ ...MockChatQueuedMessage, id: 7 },
		]);

		await user.click(screen.getByRole("button", { name: "Edit" }));

		expect(onEdit).toHaveBeenCalledWith(7);
		expect(onResume).not.toHaveBeenCalled();
	});

	it("calls onResume and onEdit with the row id for a held row", async () => {
		const user = userEvent.setup();
		const { onEdit, onResume } = renderList([
			{ ...MockHeldChatQueuedMessage, id: 9 },
		]);

		await user.click(screen.getByRole("button", { name: "Resume" }));
		await user.click(screen.getByRole("button", { name: "Edit" }));

		expect(onResume).toHaveBeenCalledWith(9);
		expect(onEdit).toHaveBeenCalledWith(9);
	});

	it("keeps Send now and Remove on a held row", async () => {
		const user = userEvent.setup();
		const { onPromote, onDelete } = renderList([
			{ ...MockHeldChatQueuedMessage, id: 9 },
			{ ...MockChatQueuedMessage, id: 10 },
		]);

		await user.click(screen.getAllByRole("button", { name: "Send now" })[0]);
		expect(onPromote).toHaveBeenCalledWith(9);

		await user.click(
			screen.getAllByRole("button", { name: "Remove from queue" })[0],
		);
		expect(onDelete).toHaveBeenCalledWith(10);
	});

	it("keeps the row visible while onEdit is pending and after it fails", async () => {
		const user = userEvent.setup();
		let rejectEdit: ((error: Error) => void) | undefined;
		const onEdit = vi.fn(
			() =>
				new Promise<void>((_, reject) => {
					rejectEdit = reject;
				}),
		);
		const { onPromote } = renderList([{ ...MockChatQueuedMessage, id: 7 }], {
			onEdit,
		});

		await user.click(screen.getByRole("button", { name: "Edit" }));
		expect(onEdit).toHaveBeenCalledWith(7);
		// Other actions are disabled while the edit hold is in flight.
		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).not.toHaveBeenCalled();

		if (!rejectEdit) {
			throw new Error("onEdit was not invoked");
		}
		rejectEdit(new Error("hold failed"));

		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).toHaveBeenCalledWith(7);
	});
});
