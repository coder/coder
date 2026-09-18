import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import {
	MockChatQueuedMessage,
	MockEditingChatQueuedMessage,
} from "#/testHelpers/chatEntities";
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
		onEdit: fn(),
		onEndEdit: fn(),
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

// Queued messages retain send and delete actions and expose edit.
export const ActionsIncludeEdit: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the linter"))],
	},
};

// Without edit handlers (a read-only viewer) only send and delete render.
export const ActionsWithoutEdit: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the linter"))],
		onEdit: undefined,
		onEndEdit: undefined,
	},
};

// A row under edit in the middle of a busy chat's queue: the row ahead of
// it is still sent, the row under edit offers Cancel edit, Edit, Send now
// and Remove, and the rows behind it wait.
export const RowUnderEditWithWaitingTail: Story = {
	args: {
		messages: [
			buildMessage(1, textContent("Install dependencies")),
			{
				...MockEditingChatQueuedMessage,
				id: 2,
				content: textContent("Run database migrations"),
			},
			buildMessage(3, textContent("Start the dev server")),
			buildMessage(4, textContent("Open the browser")),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByText("Run database migrations"));
	},
};

// The chat is paused: cancelling the head's edit sends it, and the row
// behind it shows Edit disabled with a reason.
export const PausedAtHead: Story = {
	args: {
		chatPaused: true,
		messages: [
			{
				...MockEditingChatQueuedMessage,
				id: 1,
				content: textContent("Run the test suite"),
			},
			buildMessage(2, textContent("Open the browser")),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByRole("button", { name: "Cancel edit" }));
	},
};

// The chat is paused: Edit on the row behind the head is disabled and its
// tooltip says why.
export const PausedEditBehindHead: Story = {
	args: {
		chatPaused: true,
		messages: [
			{
				...MockEditingChatQueuedMessage,
				id: 1,
				content: textContent("Run the test suite"),
			},
			buildMessage(2, textContent("Open the browser")),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const editBehind = canvas.getAllByRole("button", { name: "Edit" })[1];
		// The disabled button's wrapper span hosts the tooltip.
		await userEvent.hover(editBehind.parentElement ?? editBehind);
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

		if (!rejectQueuedDelete) {
			throw new Error("onDelete was not invoked");
		}
		rejectQueuedDelete(new Error("delete failed"));
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
		await userEvent.tab();
		expect(trigger).toHaveFocus();
		const tooltip = await within(document.body).findByRole("tooltip");
		expect(tooltip).toHaveTextContent("Deployment prompts are audited.");
	},
};
