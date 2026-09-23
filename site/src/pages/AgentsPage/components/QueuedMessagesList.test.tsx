import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	MockChatQueuedMessage,
	MockChatQueuedMessageUnderEdit,
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
				| "onDelete"
				| "onPromote"
				| "onEdit"
				| "onEndEdit"
				| "chatPaused"
				| "queuedMessageUnderEditID"
			>
		> = {},
	) => {
		const onDelete = vi.fn();
		const onPromote = vi.fn();
		const onEdit = vi.fn();
		const onEndEdit = vi.fn();
		render(
			<TooltipProvider>
				<QueuedMessagesList
					messages={messages}
					onDelete={onDelete}
					onPromote={onPromote}
					onEdit={onEdit}
					onEndEdit={onEndEdit}
					{...handlers}
				/>
			</TooltipProvider>,
		);
		return { onDelete, onPromote, onEdit, onEndEdit };
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

	it("forwards Edit and Cancel edit with the row id", async () => {
		const user = userEvent.setup();
		const { onEdit, onEndEdit } = renderList([
			{ ...MockChatQueuedMessageUnderEdit, id: 9 },
			{ ...MockChatQueuedMessage, id: 10 },
		]);

		await user.click(screen.getByRole("button", { name: "Cancel edit" }));
		expect(onEndEdit).toHaveBeenCalledWith(9);

		const editButtons = screen.getAllByRole("button", { name: "Edit" });
		await user.click(editButtons[0]);
		await user.click(editButtons[1]);
		expect(onEdit).toHaveBeenNthCalledWith(1, 9);
		expect(onEdit).toHaveBeenNthCalledWith(2, 10);
	});

	it("while the chat is paused, only the row under edit accepts Edit, and focusing another row's Edit shows why", async () => {
		const user = userEvent.setup();
		const { onEdit } = renderList(
			[
				{ ...MockChatQueuedMessageUnderEdit, id: 9 },
				{ ...MockChatQueuedMessage, id: 10 },
			],
			{ chatPaused: true },
		);

		const [editUnderEdit, editBehind] = screen.getAllByRole("button", {
			name: "Edit",
		});
		act(() => editBehind.focus());
		expect(editBehind).toHaveFocus();
		expect(await screen.findByRole("tooltip")).toHaveTextContent(
			"Finish the current edit first.",
		);
		expect(editBehind).toHaveAccessibleDescription(
			"Finish the current edit first.",
		);

		await user.click(editBehind);
		expect(onEdit).not.toHaveBeenCalled();

		await user.click(editUnderEdit);
		expect(onEdit).toHaveBeenCalledWith(9);
	});

	it("offers Cancel edit on the row under edit before the server marks it", async () => {
		const user = userEvent.setup();
		const { onEndEdit } = renderList(
			[
				{ ...MockChatQueuedMessage, id: 9 },
				{ ...MockChatQueuedMessage, id: 10 },
			],
			{ queuedMessageUnderEditID: 9 },
		);

		await user.click(screen.getByRole("button", { name: "Cancel edit" }));
		expect(onEndEdit).toHaveBeenCalledWith(9);
	});

	it("keeps the row and blocks its other actions while Edit is pending, and unblocks them after the Edit fails", async () => {
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
		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).not.toHaveBeenCalled();

		const failEdit = rejectEdit;
		if (!failEdit) {
			throw new Error("onEdit was not invoked");
		}
		await act(async () => {
			failEdit(new Error("begin failed"));
		});

		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).toHaveBeenCalledWith(7);
	});
});
