import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor } from "storybook/test";
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

// Highlighted network URLs also split, so match by combined text.
const rowByText = (root: HTMLElement, text: string) =>
	Array.from(root.querySelectorAll("span")).find(
		(el) => el.textContent === text,
	);

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
		await waitFor(() => {
			expect(
				promptByText(canvasElement, "Deploy the service to production"),
			).toBeTruthy();
			expect(
				promptByText(canvasElement, "Summarize the project structure"),
			).toBeUndefined();
		});

		await userEvent.clear(input);
		await userEvent.type(input, "npmjs.org");
		await canvas.findByText("1 match");
		await expect(
			rowByText(canvasElement, "https://registry.npmjs.org/lodash"),
		).toBeTruthy();
		await expect(
			rowByText(canvasElement, "https://api.github.com/repos/coder/coder"),
		).toBeUndefined();

		await userEvent.clear(input);
		await waitFor(() => {
			expect(canvas.getByText("Network calls (4)")).toBeInTheDocument();
			expect(
				promptByText(canvasElement, "Summarize the project structure"),
			).toBeTruthy();
		});
	},
};

export const SearchNoMatches: Story = {
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});
		await userEvent.type(input, "no-such-event");
		await canvas.findByText(
			"No events match your search in the loaded events.",
			undefined,
			{ timeout: 2000 },
		);
	},
};

export const SearchMatchCount: Story = {
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});

		const status = canvas.getByRole("status", { hidden: true });
		await expect(status).not.toBeVisible();

		await userEvent.type(input, "list_directory");
		await waitFor(() => expect(status).toHaveTextContent("1 match"));
		await expect(status).toBeVisible();
	},
};

// Opens the agentic loop before searching so ToolCallBlock mounts with
// expandedByDefault=false. The query matches only the tool input JSON, not
// the tool name, so the <pre> must become visible via auto-expand for the
// input's "path" key to appear.
export const SearchToolMatchAfterMount: Story = {
	play: async ({ canvas }) => {
		await userEvent.click(
			canvas.getByRole("button", { name: /agentic loop/i }),
		);
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});
		await userEvent.type(input, ".");
		await waitFor(() =>
			expect(
				canvas.queryAllByText((_c, el) => el?.textContent === '"path"').length,
			).toBeGreaterThan(0),
		);
	},
};
