import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { StreamingOutput } from "./ConversationTimeline";
import type { MergedTool, StreamState } from "./types";

const streamingToolSources: StreamState["sources"] = [
	{
		url: "https://coder.com/docs/admin/setup",
		title: "Coder Admin Setup Guide",
	},
	{
		url: "https://coder.com/docs/api/general",
		title: "Coder API Reference",
	},
	{
		url: "https://github.com/coder/coder/wiki/Configuration",
		title: "Configuration Wiki",
	},
];

const streamingReadFileTool: MergedTool = {
	id: "stream_tc_1",
	name: "read_file",
	args: { path: "src/index.ts" },
	status: "running",
	isError: false,
};

const streamingWithToolCallsState: StreamState = {
	blocks: [
		{ type: "response", text: "Let me check the file structure first." },
		{ type: "tool", id: "stream_tc_1" },
		{ type: "response", text: "Based on what I see..." },
	],
	toolCalls: {
		stream_tc_1: {
			id: "stream_tc_1",
			name: "read_file",
			args: { path: "src/index.ts" },
		},
	},
	toolResults: {},
	sources: [],
};

const streamingWithThinkingState: StreamState = {
	blocks: [
		{
			type: "thinking",
			text: "I need to analyze the error log and determine the root cause. The stack trace suggests a null pointer in the auth middleware. Let me trace through the request lifecycle to identify where the session object becomes undefined.",
		},
		{
			type: "response",
			text: "I've identified the issue. The auth middleware...",
		},
	],
	toolCalls: {},
	toolResults: {},
	sources: [],
};

const streamingWithSourcesState: StreamState = {
	blocks: [
		{
			type: "response",
			text: "Based on the documentation, the configuration requires...",
		},
		{ type: "sources", sources: streamingToolSources },
	],
	toolCalls: {},
	toolResults: {},
	sources: streamingToolSources,
};

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
					type: "response",
					text: "Here is a partial response that is still being generated...",
				},
			],
			toolCalls: {},
			toolResults: {},
			sources: [],
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
					type: "response",
					text: "Successfully connected after retry. Here is your answer...",
				},
			],
			toolCalls: {},
			toolResults: {},
			sources: [],
		},
		streamTools: [],
		retryState: null,
	},
};

/** Active streaming with an in-progress tool call in the transcript. */
export const StreamingWithToolCalls: Story = {
	args: {
		streamState: streamingWithToolCallsState,
		streamTools: [streamingReadFileTool],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toolHeader = await canvas.findByText(/Read index\.ts/i);
		await expect(toolHeader).toBeInTheDocument();
	},
};

/** Active streaming that includes reasoning before the response text. */
export const StreamingWithThinking: Story = {
	args: {
		streamState: streamingWithThinkingState,
		streamTools: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const thinkingText = await canvas.findByText(/analyze the error log/i);
		const responseText = await canvas.findByText(/identified the issue/i);
		await expect(thinkingText).toBeInTheDocument();
		await expect(responseText).toBeInTheDocument();
	},
};

/** Active streaming that includes sources before the response finalizes. */
export const StreamingWithSources: Story = {
	args: {
		streamState: streamingWithSourcesState,
		streamTools: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sourcesToggle = await canvas.findByRole("button", {
			name: /searched 3 results/i,
		});
		await userEvent.click(sourcesToggle);
		const sourceTitle = await canvas.findByText(/Coder Admin Setup Guide/i);
		await expect(sourceTitle).toBeInTheDocument();
	},
};
