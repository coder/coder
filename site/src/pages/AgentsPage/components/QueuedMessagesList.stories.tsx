import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import {
	MockChatQueuedMessage,
	MockChatQueuedMessageUnderEdit,
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
		queuedMessageUnderEditID: null,
		composerQueuedMessageID: null,
		showEnterToSendHint: true,
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

// Several messages queued up at once. The phone viewport hides the
// Enter-to-send hint below the sm breakpoint.
export const SeveralMessages: Story = {
	args: {
		messages: [
			buildMessage(1, textContent("Install dependencies")),
			buildMessage(2, textContent("Run database migrations")),
			buildMessage(3, textContent("Start the dev server")),
		],
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
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

export const WithSkillChips: Story = {
	args: {
		messages: [
			buildMessage(1, [
				{ type: "text", text: "Draft start" },
				{
					type: "skill-invocation",
					skill_source: "personal",
					skill_name: "zebra-notes",
				},
				{ type: "text", text: "draft end" },
			]),
			buildMessage(2, [
				{
					type: "skill-invocation",
					skill_source: "project",
					skill_name: "triage",
				},
			]),
			buildMessage(3, [
				{
					type: "skill-invocation",
					skill_source: "org",
					skill_name: "release-notes",
				},
				{ type: "text", text: "First line\nSecond line" },
				{
					type: "skill-invocation",
					skill_source: "project",
					skill_name: "triage",
				},
			]),
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

// One row with every action.
export const HeadRowActions: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the linter"))],
	},
};

// A read-only viewer gets no edit handlers, so only Send now and Remove render.
export const ReadOnlyViewerActions: Story = {
	args: {
		messages: [buildMessage(1, textContent("Run the linter"))],
		onEdit: undefined,
		onEndEdit: undefined,
	},
};

// A row under edit behind the head: the head stays sendable; rows behind the
// edit wait. The row, under edit in another client, offers Cancel edit, Edit,
// Send now and Remove, and its Editing badge shows its tooltip.
export const RowUnderEditWithWaitingTail: Story = {
	args: {
		queuedMessageUnderEditID: 2,
		messages: [
			buildMessage(1, textContent("Install dependencies")),
			{
				...MockChatQueuedMessageUnderEdit,
				id: 2,
				content: textContent("Run database migrations"),
			},
			buildMessage(3, textContent("Start the dev server")),
			buildMessage(4, textContent("Open the browser")),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByRole("button", { name: "Editing" }));
	},
};

// The queue head is under edit, so the Enter-to-send hint is hidden. The
// Waiting badge on the row behind it shows its tooltip.
export const WaitingBehindHeadUnderEdit: Story = {
	args: {
		queuedMessageUnderEditID: 1,
		showEnterToSendHint: false,
		messages: [
			{
				...MockChatQueuedMessageUnderEdit,
				id: 1,
				content: textContent("Run the test suite"),
			},
			buildMessage(2, textContent("Open the browser")),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByRole("button", { name: "Waiting" }));
	},
};

// The queue head is under edit on a paused chat, so the Enter-to-send hint is
// hidden, Cancel edit sends the head, and the row behind it shows Edit
// disabled with a reason.
export const HeadUnderEdit: Story = {
	args: {
		queuedMessageUnderEditID: 1,
		isChatPaused: true,
		showEnterToSendHint: false,
		messages: [
			{
				...MockChatQueuedMessageUnderEdit,
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
		queuedMessageUnderEditID: 1,
		isChatPaused: true,
		showEnterToSendHint: false,
		messages: [
			{
				...MockChatQueuedMessageUnderEdit,
				id: 1,
				content: textContent("Run the test suite"),
			},
			buildMessage(2, textContent("Open the browser")),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const editBehind = canvas.getAllByRole("button", { name: "Edit" })[1];
		await userEvent.hover(editBehind);
	},
};

// The begin request is in flight: the row shows as under edit and the Enter
// hint is hidden because Enter saves the edit.
export const BeginRequestPending: Story = {
	args: {
		queuedMessageUnderEditID: 1,
		showEnterToSendHint: false,
		messages: [
			buildMessage(1, textContent("Run the test suite")),
			buildMessage(2, textContent("Open the browser")),
		],
	},
};

// The end request is in flight while the row still carries the server
// marker: the row shows no edit state.
export const EndRequestPending: Story = {
	args: {
		queuedMessageUnderEditID: null,
		messages: [
			{
				...MockChatQueuedMessageUnderEdit,
				id: 1,
				content: textContent("Run the test suite"),
			},
			buildMessage(2, textContent("Open the browser")),
		],
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
