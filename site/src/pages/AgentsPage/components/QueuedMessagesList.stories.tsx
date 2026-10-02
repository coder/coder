import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { ChatQueuedMessage } from "#/api/typesGenerated";
import {
	MockChatAutomation,
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
		automationNames: { names: new Map(), status: "settled" },
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

const mockLongNameAutomation = {
	...MockChatAutomation,
	id: "5c4b3a29-1807-4f6e-9d5c-4b3a29180706",
	name: "Nightly regression triage for the payments service across every staging region and canary cluster, with a summary of flaky tests",
};

export const AutomationMessages: Story = {
	decorators: [
		(Story) => (
			<div className="max-w-[390px]">
				<Story />
			</div>
		),
	],
	args: {
		messages: [
			{
				...buildMessage(1, textContent("Check the nightly build.")),
				automation_id: MockChatAutomation.id,
				input_id: "0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e",
			},
			{
				...buildMessage(2, textContent("Summarize open issues.")),
				automation_id: "3e9d8c7b-6a5f-4e3d-8c2b-1a0f9e8d7c6b",
				input_id: "9a8b7c6d-5e4f-4a3b-9c2d-1e0f2a3b4c5d",
			},
			{
				...buildMessage(3, textContent("Triage last night's failures.")),
				automation_id: mockLongNameAutomation.id,
				input_id: "1d2e3f4a-5b6c-4d7e-8f9a-0b1c2d3e4f5a",
			},
			buildMessage(4, textContent("Run the test suite")),
		],
		automationNames: {
			names: new Map([
				[MockChatAutomation.id, MockChatAutomation.name],
				[mockLongNameAutomation.id, mockLongNameAutomation.name],
			]),
			status: "settled",
		},
	},
};

export const AutomationMessagesLoading: Story = {
	...AutomationMessages,
	args: {
		...AutomationMessages.args,
		automationNames: {
			names: new Map([[MockChatAutomation.id, MockChatAutomation.name]]),
			status: "loading",
		},
	},
};

// Opens the first label's tooltip to capture the named state.
export const AutomationLabelTooltip: Story = {
	...AutomationMessages,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const [label] = await canvas.findAllByRole("button", {
			name: /^Automation run/,
		});
		await userEvent.hover(label);
	},
};

// Opens the first label's tooltip after the automations list failed.
export const AutomationLabelTooltipError: Story = {
	...AutomationLabelTooltip,
	args: {
		...AutomationMessages.args,
		automationNames: { names: new Map(), status: "error" },
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
// hidden, and the Cancel edit tooltip says cancelling sends the head.
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
