import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn } from "storybook/test";
import type { AIBridgeThread } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockSession,
} from "#/testHelpers/entities";
import { SessionTimeline } from "./SessionTimeline";

// A thread with one thinking block and one tool call.
const mockThread: AIBridgeThread = {
	id: "thread-1",
	prompt:
		"Can you check what files are in the project and summarize the structure?",
	model: "claude-opus-4-6",
	provider: "anthropic",
	credential_kind: "centralized",
	credential_hint: "sk-a...efgh",
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
	credential_kind: "centralized",
	credential_hint: "sk-a...efgh",
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
		networkCalls: [],
		searchQuery: "",
		hasNextPage: false,
		isFetchingNextPage: false,
		onFetchNextPage: noop,
	},
};

export default meta;
type Story = StoryObj<typeof SessionTimeline>;

export const OneThread: Story = {};

// A summary is present only for sessions that passed through Agent Firewall.
// The panel sits above the threads because its counts are session-scoped
// rather than tied to any one thread.
export const WithNetworkCalls: Story = {
	args: {
		networkCallSummary: { total: 4, blocked: 2 },
		networkCalls: MockAIBridgeSessionNetworkCalls,
	},
	play: async ({ canvas }) => {
		await expect(canvas.getByText("Network calls (4)")).toBeInTheDocument();
		await expect(
			canvas.getByText("https://api.github.com/repos/coder/coder"),
		).toBeInTheDocument();
	},
};

export const MultipleThreads: Story = {
	args: { threads: [mockThread, mockThreadLong] },
};

// Filtering to a tool name keeps only the thread whose agentic loop contains
// a matching tool call; the loop auto-expands so the tool name is revealed.
export const SearchFiltersThreads: Story = {
	args: {
		threads: [mockThread, mockThreadLong],
		searchQuery: "read_file",
	},
	play: async ({ canvas }) => {
		await expect(
			canvas.getByText(
				"Please refactor the authentication module so that it uses the new token-based flow we discussed. Make sure to update all the related tests and add inline comments explaining the security rationale for each change.",
			),
		).toBeInTheDocument();
		await expect(
			canvas.queryByText(
				"Can you check what files are in the project and summarize the structure?",
			),
		).not.toBeInTheDocument();
		expect((await canvas.findAllByText("read_file")).length).toBeGreaterThan(0);
	},
};

// While searching, the network panel header and rows reflect matches only.
// "npmjs.org" appears in one of the mock calls, so the header shows one match.
export const SearchFiltersNetworkCalls: Story = {
	args: {
		networkCallSummary: { total: 4, blocked: 2 },
		networkCalls: MockAIBridgeSessionNetworkCalls,
		searchQuery: "npmjs.org",
	},
	play: async ({ canvas }) => {
		await canvas.findByText("1 match");
		await expect(
			canvas.getByText("https://registry.npmjs.org/lodash"),
		).toBeInTheDocument();
		await expect(
			canvas.queryByText("https://api.github.com/repos/coder/coder"),
		).not.toBeInTheDocument();
	},
};

// A query that matches nothing shows a dedicated empty state while keeping the
// session start/end markers.
export const SearchNoMatches: Story = {
	args: {
		threads: [mockThread, mockThreadLong],
		networkCallSummary: { total: 4, blocked: 2 },
		networkCalls: MockAIBridgeSessionNetworkCalls,
		searchQuery: "no-such-event",
	},
	play: async ({ canvas }) => {
		await expect(
			canvas.getByText("No events match your search in the loaded events."),
		).toBeInTheDocument();
		await expect(canvas.queryByText("Prompt")).not.toBeInTheDocument();
	},
};

// A zero-match query on a session that still has pages to load must not
// trigger the infinite-scroll cascade, so the empty state is stable.
export const SearchNoMatchesWithNextPage: Story = {
	args: {
		threads: [mockThread, mockThreadLong],
		networkCallSummary: { total: 4, blocked: 2 },
		networkCalls: MockAIBridgeSessionNetworkCalls,
		searchQuery: "no-such-event",
		hasNextPage: true,
		isFetchingNextPage: false,
		onFetchNextPage: fn(),
	},
	play: async ({ args, canvas }) => {
		await expect(
			canvas.getByText("No events match your search in the loaded events."),
		).toBeInTheDocument();
		await expect(args.onFetchNextPage).not.toHaveBeenCalled();
	},
};

export const FetchingNextPage: Story = {
	args: { hasNextPage: true, isFetchingNextPage: true },
};

// The "Session completed" marker should stay hidden while more threads
// remain to be loaded so it cannot be misread as the end of the session.
export const HasMoreThreadsToLoad: Story = {
	args: {
		threads: [mockThread, mockThreadLong],
		hasNextPage: true,
		isFetchingNextPage: false,
	},
};
