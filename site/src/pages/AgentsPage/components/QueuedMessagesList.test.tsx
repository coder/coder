import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatQueuedMessage } from "#/testHelpers/chatEntities";
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
			Pick<ComponentProps<typeof QueuedMessagesList>, "onDelete" | "onPromote">
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

	it("forwards Send now and Remove with the row id", async () => {
		const user = userEvent.setup();
		const row = { ...MockChatQueuedMessage, id: 9 };

		const { onPromote } = renderList([row]);
		await user.click(screen.getByRole("button", { name: "Send now" }));
		expect(onPromote).toHaveBeenCalledWith(9);
		cleanup();

		const { onDelete } = renderList([row]);
		await user.click(screen.getByRole("button", { name: "Remove from queue" }));
		expect(onDelete).toHaveBeenCalledWith(9);
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
		const sendBehind = screen.getByRole("button", { name: "Send now" });
		expect(sendBehind).toBeDisabled();

		if (!rejectPromote) {
			throw new Error("onPromote was not invoked");
		}
		rejectPromote(new Error("promote failed"));

		await waitFor(() =>
			expect(
				screen.getAllByRole("button", { name: "Remove from queue" }),
			).toHaveLength(2),
		);
		await user.click(
			screen.getAllByRole("button", { name: "Remove from queue" })[0],
		);
		expect(onDelete).toHaveBeenCalledWith(7);
	});
});
