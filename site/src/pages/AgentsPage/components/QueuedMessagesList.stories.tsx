import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ChatQueuedMessage } from "api/typesGenerated";
import { expect, fn, userEvent, within } from "storybook/test";
import { QueuedMessagesList } from "./QueuedMessagesList";

// Helper to build a ChatQueuedMessage with minimal boilerplate.
function makeMessage(
	id: number,
	content: ChatQueuedMessage["content"],
): ChatQueuedMessage {
	return {
		id,
		chat_id: "test-chat-id",
		content,
		created_at: new Date().toISOString(),
	};
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
		messages: [makeMessage(1, textContent("Run the test suite"))],
	},
};

// Several messages queued up at once.
export const SeveralMessages: Story = {
	args: {
		messages: [
			makeMessage(1, textContent("Install dependencies")),
			makeMessage(2, textContent("Run database migrations")),
			makeMessage(3, textContent("Start the dev server")),
		],
	},
};

// Messages with different content shapes to exercise the parsing logic.
export const MixedContentTypes: Story = {
	args: {
		messages: [
			// Typed text content.
			makeMessage(1, textContent("Plain text content")),
			// Attachment-only message falls back to the generic label.
			makeMessage(2, [
				{ type: "file", file_id: "img-1", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
			// Empty content falls back to the generic label.
			makeMessage(3, [] as ChatQueuedMessage["content"]),
		],
	},
};

// A longer queue to verify scrolling and layout with many items.
export const LongQueue: Story = {
	args: {
		messages: Array.from({ length: 10 }, (_, i) =>
			makeMessage(i + 1, textContent(`Queued task number ${i + 1}`)),
		),
	},
};

// A message whose content is a long string to test truncation.
export const LongMessageText: Story = {
	args: {
		messages: [
			makeMessage(
				1,
				textContent(
					"This is an extremely long queued message that should be truncated by the component layout because it exceeds the available horizontal space in the queue list container",
				),
			),
			makeMessage(2, textContent("Short follow-up")),
		],
	},
};

// Multi-line text is truncated to the first line with an ellipsis appended.
export const MultiLineTextTruncation: Story = {
	args: {
		messages: [
			makeMessage(
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

// A message with both text and a file attachment shows the ImageIcon badge.
export const WithAttachments: Story = {
	args: {
		messages: [
			makeMessage(1, [
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
			makeMessage(1, [
				{ type: "file", file_id: "img-1", media_type: "image/png" },
				{ type: "file", file_id: "img-2", media_type: "image/jpeg" },
			] as ChatQueuedMessage["content"]),
		],
	},
};

// Clicking Edit on a message with attachments passes file blocks to onEdit.
export const EditPassesFileBlocks: Story = {
	args: {
		onEdit: fn(),
		messages: [
			makeMessage(1, [
				{ type: "text", text: "Check this screenshot" },
				{ type: "file", file_id: "abc-123", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const editButton = canvas.getByRole("button", { name: "Edit" });
		await userEvent.click(editButton);
		expect(args.onEdit).toHaveBeenCalledWith(1, "Check this screenshot", [
			{ type: "file", file_id: "abc-123", media_type: "image/png" },
		]);
	},
};

// Clicking Edit on an attachment-only message passes file blocks with empty text.
export const EditAttachmentOnlyMessage: Story = {
	args: {
		onEdit: fn(),
		messages: [
			makeMessage(1, [
				{ type: "file", file_id: "img-1", media_type: "image/png" },
				{ type: "file", file_id: "img-2", media_type: "image/jpeg" },
			] as ChatQueuedMessage["content"]),
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const editButton = canvas.getByRole("button", { name: "Edit" });
		await userEvent.click(editButton);
		expect(args.onEdit).toHaveBeenCalledWith(1, "", [
			{ type: "file", file_id: "img-1", media_type: "image/png" },
			{ type: "file", file_id: "img-2", media_type: "image/jpeg" },
		]);
	},
};

// A mixed queue with text-only, text+attachment, and attachment-only messages.
export const MixedQueueWithAttachments: Story = {
	args: {
		messages: [
			makeMessage(1, textContent("Run the linter")),
			makeMessage(2, [
				{ type: "text", text: "Fix this layout bug" },
				{ type: "file", file_id: "img-a", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
			makeMessage(3, [
				{ type: "file", file_id: "img-b", media_type: "image/png" },
			] as ChatQueuedMessage["content"]),
		],
	},
};
