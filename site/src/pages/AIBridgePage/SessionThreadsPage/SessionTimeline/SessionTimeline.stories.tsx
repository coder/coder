import type { Meta, StoryObj } from "@storybook/react-vite";
import type { AIBridgeThread } from "#/api/typesGenerated";
import { MockSession } from "#/testHelpers/entities";
import { SessionTimeline } from "./SessionTimeline";

// A thread with one thinking block and one tool call.
const mockThread: AIBridgeThread = {
	id: "thread-1",
	prompt:
		"Can you check what files are in the project and summarize the structure?",
	model: "claude-opus-4-6",
	provider: "anthropic",
	started_at: "2026-03-09T09:28:15.000Z",
	ended_at: "2026-03-09T09:28:47.000Z",
	token_usage: {
		input_tokens: 1240,
		output_tokens: 320,
		cache_read_input_tokens: 900,
		cache_write_input_tokens: 140,
		metadata: { cache_read_input_tokens: 900 },
	},
	agentic_actions: [
		{
			model: "claude-opus-4-6",
			token_usage: {
				input_tokens: 620,
				output_tokens: 160,
				cache_read_input_tokens: 450,
				cache_write_input_tokens: 70,
				metadata: {},
			},
			thinking: [
				{
					text: "The user wants to understand the project structure. I should start by listing the root directory, then drill into interesting sub-directories.",
				},
			],
			tool_calls: [
				{
					id: "tool-1",
					interception_id: "interception-1",
					provider_response_id: "resp-1",
					server_url: "http://localhost:3000/mcp",
					tool: "list_directory",
					injected: false,
					input: JSON.stringify({ path: "." }),
					metadata: {},
					created_at: "2026-03-09T09:28:20.000Z",
				},
			],
		},
	],
};

// A second thread with a long prompt and multiple tool calls.
const mockThreadLong: AIBridgeThread = {
	id: "thread-2",
	prompt:
		"Please refactor the authentication module so that it uses the new token-based flow we discussed. Make sure to update all the related tests and add inline comments explaining the security rationale for each change.",
	model: "claude-opus-4-6",
	provider: "anthropic",
	started_at: "2026-03-09T10:00:00.000Z",
	ended_at: "2026-03-09T10:05:30.000Z",
	token_usage: {
		input_tokens: 8500,
		output_tokens: 3200,
		cache_read_input_tokens: 6000,
		cache_write_input_tokens: 2000,
		metadata: {
			cache_read_input_tokens: 6000,
			cache_creation_input_tokens: 2000,
		},
	},
	agentic_actions: [
		{
			model: "claude-opus-4-6",
			token_usage: {
				input_tokens: 2800,
				output_tokens: 1100,
				cache_read_input_tokens: 1800,
				cache_write_input_tokens: 500,
				metadata: {},
			},
			thinking: [],
			tool_calls: [
				{
					id: "tool-2a",
					interception_id: "interception-2",
					provider_response_id: "resp-2",
					server_url: "http://localhost:3000/mcp",
					tool: "read_file",
					injected: false,
					input: JSON.stringify({ path: "src/auth/index.ts" }),
					metadata: {},
					created_at: "2026-03-09T10:00:15.000Z",
				},
				{
					id: "tool-2b",
					interception_id: "interception-3",
					provider_response_id: "resp-3",
					server_url: "http://localhost:3000/mcp",
					tool: "write_file",
					injected: false,
					input: JSON.stringify({
						path: "src/auth/index.ts",
						content: "// refactored auth module\n...",
					}),
					metadata: {},
					created_at: "2026-03-09T10:01:00.000Z",
				},
			],
		},
	],
};

const noop = () => {};

const meta: Meta<typeof SessionTimeline> = {
	title: "pages/AIBridgePage/SessionTimeline",
	component: SessionTimeline,
	args: {
		initiator: MockSession.initiator,
		threads: [mockThread],
		hasNextPage: false,
		isFetchingNextPage: false,
		onFetchNextPage: noop,
	},
};

export default meta;
type Story = StoryObj<typeof SessionTimeline>;

export const OneThread: Story = {};

export const MultipleThreads: Story = {
	args: { threads: [mockThread, mockThreadLong] },
};

export const FetchingNextPage: Story = {
	args: { hasNextPage: true, isFetchingNextPage: true },
};
