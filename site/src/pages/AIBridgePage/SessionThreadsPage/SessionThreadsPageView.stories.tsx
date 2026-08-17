import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import type { AIBridgeSessionThreadsResponse } from "#/api/typesGenerated";
import {
	MockAIBridgeSessionNetworkCalls,
	MockAIBridgeThread,
	MockSession,
} from "#/testHelpers/entities";
import { SessionThreadsPageView } from "./SessionThreadsPageView";

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
	threads: [MockAIBridgeThread],
};

const noop = () => {};

// Highlighted prompts split into <strong>/<span> children, so match the
// paragraph by its combined text rather than a single text node.
const promptByText = (root: HTMLElement, text: string) =>
	Array.from(root.querySelectorAll("p")).find((el) => el.textContent === text);

const meta: Meta<typeof SessionThreadsPageView> = {
	title: "pages/AIBridgePage/SessionThreadsPageView",
	component: SessionThreadsPageView,
	args: {
		session: mockSession,
		threads: [MockAIBridgeThread],
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
			MockAIBridgeThread,
			{
				...MockAIBridgeThread,
				id: "thread-2",
				prompt: "Deploy the service to production",
				agentic_actions: [],
			},
		],
	},
	play: async ({ canvas, canvasElement }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});

		await expect(
			promptByText(canvasElement, "Summarize the project structure"),
		).toBeTruthy();
		await expect(
			promptByText(canvasElement, "Deploy the service to production"),
		).toBeTruthy();

		await userEvent.type(input, "deploy");
		await expect(
			promptByText(canvasElement, "Deploy the service to production"),
		).toBeTruthy();
		await expect(
			promptByText(canvasElement, "Summarize the project structure"),
		).toBeUndefined();

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
			promptByText(canvasElement, "Summarize the project structure"),
		).toBeTruthy();
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

// The header count reports occurrences across the loaded session and names
// the scope honestly.
export const SearchResultsCount: Story = {
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});
		await userEvent.type(input, "list_directory");
		await expect(
			canvas.getByText((_content, element) => {
				return element?.textContent === "1 result";
			}),
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
