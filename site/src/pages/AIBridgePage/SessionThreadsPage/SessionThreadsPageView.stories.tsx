import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import type {
	AIBridgeSessionThreadsResponse,
	AIBridgeThread,
} from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockSession,
} from "#/testHelpers/entities";
import { SessionThreadsPageView } from "./SessionThreadsPageView";

// A thread with a prompt and one tool call.
const mockThread: AIBridgeThread = {
	id: "thread-1",
	prompt: "Summarize the project structure",
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
		metadata: {},
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
			thinking: [],
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

const mockSession: AIBridgeSessionThreadsResponse = {
	id: MockSession.id,
	initiator: MockSession.initiator,
	providers: MockSession.providers,
	models: MockSession.models,
	metadata: MockSession.metadata,
	started_at: MockSession.started_at,
	ended_at: MockSession.ended_at,
	token_usage_summary: {
		input_tokens: 1234,
		output_tokens: 4321,
		cache_read_input_tokens: 980,
		cache_write_input_tokens: 120,
		metadata: {},
	},
	network_calls: { total: 4, blocked: 2 },
	network_call_logs: MockAIBridgeSessionNetworkCalls,
	threads: [mockThread],
};

const noop = () => {};

const meta: Meta<typeof SessionThreadsPageView> = {
	title: "pages/AIBridgePage/SessionThreadsPageView",
	component: SessionThreadsPageView,
	args: {
		session: mockSession,
		threads: [mockThread],
		loading: false,
		hasNextPage: false,
		isFetchingNextPage: false,
		onFetchNextPage: noop,
		isAISessionsEnabled: true,
		isAISessionsEntitled: true,
		onBackClicked: noop,
	},
};

export default meta;
type Story = StoryObj<typeof SessionThreadsPageView>;

export const Default: Story = {};

export const SearchFiltersEvents: Story = {
	args: {
		threads: [
			mockThread,
			{
				...mockThread,
				id: "thread-2",
				prompt: "Deploy the service to production",
				agentic_actions: [],
			},
		],
	},
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});

		await expect(
			canvas.getByText("Summarize the project structure"),
		).toBeVisible();
		await expect(
			canvas.getByText("Deploy the service to production"),
		).toBeVisible();

		await userEvent.type(input, "deploy");
		await expect(
			canvas.getByText("Deploy the service to production"),
		).toBeVisible();
		await expect(
			canvas.queryByText("Summarize the project structure"),
		).not.toBeInTheDocument();

		await userEvent.clear(input);
		await userEvent.type(input, "npmjs.org");
		await canvas.findByText("1 match");
		await expect(
			canvas.getByText("https://registry.npmjs.org/lodash"),
		).toBeVisible();
		await expect(
			canvas.queryByText("https://api.github.com/repos/coder/coder"),
		).not.toBeInTheDocument();

		await userEvent.clear(input);
		await canvas.findByText("Network calls (4)");
		await expect(
			canvas.getByText("https://registry.npmjs.org/lodash"),
		).toBeVisible();
		await expect(
			canvas.getByText("Summarize the project structure"),
		).toBeVisible();
	},
};

export const SearchNoMatches: Story = {
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});
		await userEvent.type(input, "no-such-event");
		await expect(
			canvas.getByText("No events match your search in the loaded events."),
		).toBeInTheDocument();
	},
};

// Types the tool query after mount: the auto-expand must react to the search
// prop on an already-rendered thread, not just on mount.
export const SearchToolMatchAfterMount: Story = {
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});
		await userEvent.type(input, "list_directory");
		expect(
			(await canvas.findAllByText("list_directory")).length,
		).toBeGreaterThan(0);
	},
};
