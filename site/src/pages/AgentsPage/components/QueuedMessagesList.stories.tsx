import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import { MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import { QueuedMessagesList } from "./QueuedMessagesList";

// Helper to build a ChatQueuedMessage with minimal boilerplate.
function buildMessage(
	id: number,
	content: ChatQueuedMessage["content"],
): ChatQueuedMessage {
	return { ...MockChatQueuedMessage, id, content };
}

const meta: Meta<typeof QueuedMessagesList> = {
	title: "pages/AgentsPage/QueuedMessagesList",
	component: QueuedMessagesList,
	args: {
		onDelete: fn(),
		onPromote: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof QueuedMessagesList>;

// When the messages array is empty the component renders nothing.
export const Empty: Story = {
	args: {
		messages: [],
	},
};

const textContent = (text: string): ChatQueuedMessage["content"] =>
	[
		{
			type: "text",
			text,
		},
	] as ChatQueuedMessage["content"];

// A single queued message with text-part content.
export const SingleMessage: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the test suite"))],
	},
};

// Several messages queued up at once.
export const SeveralMessages: Story = {
	args: {
		messages: [
			buildMessage(1, textContent("Install dependencies")),
			buildMessage(2, textContent("Run database migrations")),
			buildMessage(3, textContent("Start the dev server")),
		],
	},
};

// Messages with different content shapes to exercise the parsing logic.
export const MixedContentTypes: Story = {
	args: {
		messages: [
			// Typed text content.
			buildMessage(1, textContent("Plain text content")),
			// Attachment-only message falls back to the generic label.
			buildMessage(2, [
				{ type: "file", file_id: "img-1", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
			// Empty content falls back to the generic label.
			buildMessage(3, [] as ChatQueuedMessage["content"]),
		],
	},
};

// A longer queue to verify scrolling and layout with many items.
export const LongQueue: Story = {
	args: {
		messages: Array.from({ length: 10 }, (_, i) =>
			buildMessage(i + 1, textContent(`Queued task number ${i + 1}`)),
		),
	},
};

// A message whose content is a long string to test truncation.
export const LongMessageText: Story = {
	args: {
		messages: [
			buildMessage(
				1,
				textContent(
					"This is an extremely long queued message that should be truncated by the component layout because it exceeds the available horizontal space in the queue list container",
				),
			),
			buildMessage(2, textContent("Short follow-up")),
		],
	},
};

// Multi-line text is truncated to the first line with an ellipsis appended.
export const MultiLineTextTruncation: Story = {
	args: {
		messages: [
			buildMessage(
				1,
				textContent(
					"First line of the message\nSecond line that should be hidden",
				),
			),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The first line and ellipsis should be visible in the same span.
		const textSpan = canvas.getByText(/First line of the message…/);
		expect(textSpan).toBeInTheDocument();
		// The second line should not appear anywhere.
		expect(canvas.queryByText(/Second line/)).not.toBeInTheDocument();
	},
};

export const ExpandAndCollapseText: Story = {
	args: {
		messages: [
			buildMessage(
				1,
				textContent(
					"First line of the message\nSecond line that should be hidden",
				),
			),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", {
			name: /First line of the message/,
		});
		expect(toggle).toHaveAttribute("aria-expanded", "false");

		await userEvent.click(toggle);
		await waitFor(() =>
			expect(
				canvas.getByRole("button", { name: /Second line/ }),
			).toHaveAttribute("aria-expanded", "true"),
		);

		await userEvent.click(canvas.getByRole("button", { name: /Second line/ }));
		await waitFor(() =>
			expect(canvas.queryByText(/Second line/)).not.toBeInTheDocument(),
		);
	},
};

// A message with both text and a file attachment shows the ImageIcon badge.
export const WithAttachments: Story = {
	args: {
		messages: [
			buildMessage(1, [
				{ type: "text", text: "Check this screenshot" },
				{ type: "file", file_id: "abc-123", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
		],
	},
};

// A message with only file attachments and no text displays a count label.
export const AttachmentsOnly: Story = {
	args: {
		messages: [
			buildMessage(1, [
				{ type: "file", file_id: "img-1", media_type: "image/png" },
				{ type: "file", file_id: "img-2", media_type: "image/jpeg" },
			] as ChatQueuedMessage["content"]),
		],
	},
};

// Queued messages retain send and delete actions without exposing edit.
export const ActionsExcludeEdit: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the linter"))],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("button", { name: "Send now" })).toBeVisible();
		expect(
			canvas.getByRole("button", { name: "Remove from queue" }),
		).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: "Edit queued message" }),
		).not.toBeInTheDocument();
	},
};

export const EditAction: Story = {
	args: {
		messages: [
			buildMessage(1, textContent("Run the linter")),
			buildMessage(2, textContent("Run the formatter")),
		],
		onEdit: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const editButtons = canvas.getAllByRole("button", {
			name: "Edit queued message",
		});
		await userEvent.click(editButtons[1]);
		expect(args.onEdit).toHaveBeenCalledWith(2);
	},
};

export const EditDisabledWhileComposerHasDraft: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the linter"))],
		onEdit: fn(),
		editDisabledReason: "Send or clear the composer to edit a queued message.",
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Edit queued message" }),
		).toBeDisabled();
		expect(args.onEdit).not.toHaveBeenCalled();
	},
};

export const EditDisabledForAttachments: Story = {
	args: {
		messages: [
			buildMessage(1, [
				{ type: "text", text: "Look at this" },
				{ type: "file", file_id: "file-1", media_type: "image/png" },
			]),
			buildMessage(2, textContent("Plain text prompt")),
		],
		onEdit: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const editButtons = canvas.getAllByRole("button", {
			name: "Edit queued message",
		});
		expect(editButtons[0]).toBeDisabled();
		expect(editButtons[1]).toBeEnabled();
	},
};

let rejectQueuedEdit: ((error: Error) => void) | undefined;

export const EditRejectionRestoresRow: Story = {
	args: {
		messages: [buildMessage(1, textContent("First queued"))],
		onEdit: () =>
			new Promise<void>((_, reject) => {
				rejectQueuedEdit = reject;
			}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Edit queued message" }),
		);
		await waitFor(() =>
			expect(canvas.queryByText("First queued")).not.toBeInTheDocument(),
		);

		rejectQueuedEdit?.(new Error("nope"));
		await waitFor(() => expect(canvas.getByText("First queued")).toBeVisible());
	},
};

let rejectQueuedDelete: ((error: Error) => void) | undefined;

// Deleting hides the row optimistically and disables sibling actions while
// pending; a rejected delete restores the row and re-enables actions.
export const DeleteRejectionRestoresRow: Story = {
	args: {
		messages: [
			buildMessage(1, textContent("First queued")),
			buildMessage(2, textContent("Second queued")),
		],
		onDelete: () =>
			new Promise<void>((_, reject) => {
				rejectQueuedDelete = reject;
			}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const removeButtons = canvas.getAllByRole("button", {
			name: "Remove from queue",
		});
		await userEvent.click(removeButtons[0]);

		expect(canvas.queryByText("First queued")).not.toBeInTheDocument();
		expect(canvas.getByText("Second queued")).toBeVisible();
		expect(canvas.getByRole("button", { name: "Send now" })).toBeDisabled();

		if (!rejectQueuedDelete) {
			throw new Error("onDelete was not invoked");
		}
		rejectQueuedDelete(new Error("delete failed"));

		await waitFor(() => expect(canvas.getByText("First queued")).toBeVisible());
		for (const button of canvas.getAllByRole("button", {
			name: "Send now",
		})) {
			expect(button).toBeEnabled();
		}
	},
};

// A mixed queue with text-only, text+attachment, and attachment-only messages.
export const MixedQueueWithAttachments: Story = {
	args: {
		messages: [
			buildMessage(1, textContent("Run the linter")),
			buildMessage(2, [
				{ type: "text", text: "Fix this layout bug" },
				{ type: "file", file_id: "img-a", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
			buildMessage(3, [
				{ type: "file", file_id: "img-b", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
		],
	},
};

export const HookNotice: Story = {
	args: {
		messages: [
			buildMessage(1, [
				{ type: "text", text: "Deploy to production" },
				{ type: "hook-notice", text: "Deployment prompts are audited." },
			]),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", {
			name: "Lifecycle hook notice: Deployment prompts are audited.",
		});
		// The expand/collapse control takes the first tab stop.
		await userEvent.tab();
		await userEvent.tab();
		expect(trigger).toHaveFocus();
		const tooltip = await within(document.body).findByRole("tooltip");
		expect(tooltip).toHaveTextContent("Deployment prompts are audited.");
	},
};
