import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor } from "storybook/test";
import type { AIBridgeThread } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockAIBridgeThread,
	MockSession,
} from "#/testHelpers/entities";
import { SessionTimeline } from "./SessionTimeline";

// The shared single-tool thread, with a prompt that includes the CRF-6
// negative-assertion string and a token-usage metadata variant.
const mockThread: AIBridgeThread = {
	...MockAIBridgeThread,
	prompt:
		"Can you check what files are in the project and summarize the structure?",
	token_usage: {
		...MockAIBridgeThread.token_usage,
		metadata: { cache_read_input_tokens: 900 },
	},
	agentic_actions: MockAIBridgeThread.agentic_actions.map((action) => ({
		...action,
		thinking: [
			{
				text: "The user wants to understand the project structure. I should start by listing the root directory, then drill into interesting sub-directories.",
			},
		],
	})),
};

// A second thread with a long prompt and multiple tool calls.
const mockThreadLong: AIBridgeThread = {
	...MockAIBridgeThread,
	id: "thread-2",
	prompt:
		"Please refactor the authentication module so that it uses the new token-based flow we discussed. Make sure to update all the related tests and add inline comments explaining the security rationale for each change.",
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

// A thread whose prompt is long enough to collapse past 200px, with the only
// match near the end so the preview must window around it.
const windowedFiller =
	"Review the deployment pipeline, inspect staging, confirm feature flags, validate the rollout plan, check dashboards, and review escalation paths. ";
const mockThreadWindowed: AIBridgeThread = {
	...MockAIBridgeThread,
	id: "thread-3",
	prompt:
		windowedFiller.repeat(16) +
		"Finally, coordinate the cutover using zebra-relay.",
	agentic_actions: [],
};

export const MultipleThreads: Story = {
	args: { threads: [mockThread, mockThreadLong] },
};

// A match past the 200px prompt cutoff windows the preview around the match,
// ellipsizing both ends, instead of hiding it behind the collapse.
export const SearchWindowsPromptOnMatch: Story = {
	args: {
		threads: [mockThreadWindowed],
		searchQuery: "zebra-relay",
	},
	play: async ({ canvas, canvasElement }) => {
		// The bolded match is revealed even though it sits past the cutoff.
		await expect(canvas.getByText("zebra-relay")).toBeInTheDocument();
		// Once the full height is measured, the preview windows around the
		// match: the paragraph leads with an ellipsis and no longer starts at
		// the prompt head.
		await waitFor(() => {
			const para = Array.from(canvasElement.querySelectorAll("p")).find((el) =>
				el.textContent?.includes("zebra-relay"),
			);
			expect(para?.textContent?.trimStart().startsWith("…")).toBe(true);
		});
	},
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

// A tool-name search hides non-matching tool calls in the multi-tool loop
// and expands the match.
export const SearchFiltersToolCalls: Story = {
	args: {
		threads: [mockThreadLong],
		searchQuery: "read_file",
	},
	play: async ({ canvas }) => {
		await expect(
			(await canvas.findAllByText("read_file")).length,
		).toBeGreaterThan(0);
		await expect(canvas.queryByText("write_file")).not.toBeInTheDocument();
	},
};

// A prompt-only search leaves tool calls alone: the empty match set must not
// filter them out when the user opens the loop by hand.
export const SearchPromptMatchKeepsToolCalls: Story = {
	args: {
		threads: [mockThreadLong],
		// "token-based" appears only in the prompt, never in a tool name or
		// input, so no tool call matches and all of them must still render.
		searchQuery: "token-based",
	},
	play: async ({ canvas }) => {
		await userEvent.click(
			canvas.getByRole("button", { name: /agentic loop/i }),
		);
		await expect(canvas.getByText("read_file")).toBeInTheDocument();
		await expect(canvas.getByText("write_file")).toBeInTheDocument();
	},
};

// While searching, the network panel header and rows reflect matches only.
export const SearchFiltersNetworkCalls: Story = {
	args: {
		networkCallSummary: { total: 4, blocked: 2 },
		networkCalls: MockAIBridgeSessionNetworkCalls,
		searchQuery: "npmjs.org",
	},
	play: async ({ canvas }) => {
		await canvas.findByText("1 match");
		// Highlighting splits the URL into <strong>/<span> children, so match
		// the row by its combined text rather than a single text node.
		const rowByText = (text: string) =>
			canvas.queryAllByText((_c, el) => el?.textContent === text);
		await expect(
			rowByText("https://registry.npmjs.org/lodash").length,
		).toBeGreaterThan(0);
		await expect(
			rowByText("https://api.github.com/repos/coder/coder"),
		).toHaveLength(0);
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
			canvas.getByText(/No events match your search in the loaded events/),
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
