import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	MockChatQueuedMessage,
	MockEditingChatQueuedMessage,
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
				| "queuedEditOverride"
				| "enterSendsHead"
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

	it("forwards Edit and Cancel edit with the row id", async () => {
		const user = userEvent.setup();
		const { onEdit, onEndEdit } = renderList([
			{ ...MockEditingChatQueuedMessage, id: 9 },
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

	it("disables Edit on rows behind the edit while the chat is paused and says why", async () => {
		const user = userEvent.setup();
		const { onEdit } = renderList(
			[
				{ ...MockEditingChatQueuedMessage, id: 9 },
				{ ...MockChatQueuedMessage, id: 10 },
			],
			{ chatPaused: true },
		);

		const [editUnderEdit, editBehind] = screen.getAllByRole("button", {
			name: "Edit",
		});
		expect(editBehind).toBeDisabled();
		await user.hover(editBehind);
		expect(
			await screen.findByText("Finish the current edit first."),
		).toBeInTheDocument();

		await user.click(editUnderEdit);
		expect(onEdit).toHaveBeenCalledWith(9);
	});

	it("lets the local edit state override the row's marker", () => {
		renderList(
			[
				{ ...MockChatQueuedMessage, id: 9 },
				{ ...MockChatQueuedMessage, id: 10 },
			],
			{ queuedEditOverride: { id: 9, editing: true } },
		);
		expect(screen.getByText("Editing")).toBeInTheDocument();
		expect(screen.getByText("Waiting")).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "Cancel edit" }),
		).toBeInTheDocument();
		cleanup();

		renderList([{ ...MockEditingChatQueuedMessage, id: 9 }], {
			queuedEditOverride: { id: 9, editing: false },
		});
		expect(screen.queryByText("Editing")).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Cancel edit" }),
		).not.toBeInTheDocument();
	});

	it("hides the Enter hint on the head while Enter saves an edit", () => {
		renderList([{ ...MockChatQueuedMessage, id: 9 }]);
		expect(screen.getByText("to send")).toBeInTheDocument();
		cleanup();

		renderList([{ ...MockChatQueuedMessage, id: 9 }], {
			enterSendsHead: false,
		});
		expect(screen.queryByText("to send")).not.toBeInTheDocument();
	});

	it("still offers Send now and Remove on a row under edit", async () => {
		const user = userEvent.setup();
		const row = { ...MockEditingChatQueuedMessage, id: 9 };

		const { onPromote } = renderList([row]);
		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).toHaveBeenCalledWith(9);
		cleanup();

		const { onDelete } = renderList([row]);
		await user.click(screen.getByRole("button", { name: "Remove from queue" }));
		expect(onDelete).toHaveBeenCalledWith(9);
	});

	it("disables row actions while onEdit is pending and re-enables them after it fails", async () => {
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

		if (!rejectEdit) {
			throw new Error("onEdit was not invoked");
		}
		rejectEdit(new Error("begin failed"));

		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).toHaveBeenCalledWith(7);
	});
});
