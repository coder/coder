import type { Meta, StoryObj } from "@storybook/react-vite";
import { StreamingOutput } from "./ConversationTimeline";

// StreamingOutput renders inside a ConversationItem > Message > MessageContent
// chain, but it's self-contained enough to render standalone.

const meta: Meta<typeof StreamingOutput> = {
	title: "pages/AgentsPage/AgentDetail/StreamingOutput",
	component: StreamingOutput,
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6">
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof StreamingOutput>;

/** Default shimmer placeholder with no stream state. */
export const ThinkingPlaceholder: Story = {
	args: {
		streamState: null,
		streamTools: [],
		showInitialPlaceholder: true,
	},
};

/** First retry attempt. */
export const RetryAttempt1: Story = {
	args: {
		streamState: null,
		streamTools: [],
		showInitialPlaceholder: true,
		retryState: { attempt: 1, error: "service unavailable" },
	},
};

/** Third retry attempt. */
export const RetryAttempt3: Story = {
	args: {
		streamState: null,
		streamTools: [],
		showInitialPlaceholder: true,
		retryState: { attempt: 3, error: "rate limit exceeded" },
	},
};

/** Higher attempt number to see how it looks. */
export const RetryHighAttempt: Story = {
	args: {
		streamState: null,
		streamTools: [],
		showInitialPlaceholder: true,
		retryState: { attempt: 12, error: "overloaded" },
	},
};

/** Active streaming with partial text content. */
export const StreamingWithText: Story = {
	args: {
		streamState: {
			blocks: [
				{
					type: "response" as const,
					text: "Here is a partial response that is still being generated...",
				},
			],
			toolCalls: {},
			toolResults: {},
		},
		streamTools: [],
	},
};

/** Content arrived after retries (no retry indicator shown). */
export const StreamingAfterRetry: Story = {
	args: {
		streamState: {
			blocks: [
				{
					type: "response" as const,
					text: "Successfully connected after retry. Here is your answer...",
				},
			],
			toolCalls: {},
			toolResults: {},
		},
		streamTools: [],
		retryState: null,
	},
};
