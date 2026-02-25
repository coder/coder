import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { ChatQueuedMessage } from "api/typesGenerated";
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

// A single queued message with plain object content.
export const SingleMessage: Story = {
	args: {
		messages: [makeMessage(1, { text: "Run the test suite" })],
	},
};

// Several messages queued up at once.
export const SeveralMessages: Story = {
	args: {
		messages: [
			makeMessage(1, { text: "Install dependencies" }),
			makeMessage(2, { text: "Run database migrations" }),
			makeMessage(3, { text: "Start the dev server" }),
		],
	},
};

// Messages with different content shapes to exercise the parsing logic.
export const MixedContentTypes: Story = {
	args: {
		messages: [
			// Object with a text field — extracted by the component.
			makeMessage(1, { text: "Plain text content" }),
			// Object without a text field — falls through to JSON.stringify.
			makeMessage(2, { action: "deploy", target: "staging" }),
			// Empty object — stringified as "{}".
			makeMessage(3, {} as ChatQueuedMessage["content"]),
		],
	},
};

// A longer queue to verify scrolling and layout with many items.
export const LongQueue: Story = {
	args: {
		messages: Array.from({ length: 10 }, (_, i) =>
			makeMessage(i + 1, { text: `Queued task number ${i + 1}` }),
		),
	},
};

// A message whose content is a long string to test truncation.
export const LongMessageText: Story = {
	args: {
		messages: [
			makeMessage(1, {
				text: "This is an extremely long queued message that should be truncated by the component layout because it exceeds the available horizontal space in the queue list container",
			}),
			makeMessage(2, { text: "Short follow-up" }),
		],
	},
};
