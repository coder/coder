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
	play: async ({ canvas }) => {
		const input = canvas.getByRole("textbox", {
			name: /search session events/i,
		});
		const promptByText = (text: string) =>
			canvas
				.getAllByRole("paragraph")
				.find((element) => element.textContent === text);

		await expect(promptByText("Summarize the project structure")).toBeVisible();
		await expect(
			promptByText("Deploy the service to production"),
		).toBeVisible();

		await userEvent.type(input, "deploy");
		await waitFor(() => {
			expect(promptByText("Deploy the service to production")).toBeVisible();
			expect(promptByText("Summarize the project structure")).toBeUndefined();
		});

		await userEvent.clear(input);
		await userEvent.type(input, "npmjs.org");
		await canvas.findByRole("button", { name: "1 match" });
		await expect(
			canvas.getByRole("button", {
				name: /registry\.npmjs\.org\/lodash/i,
			}),
		).toBeVisible();
		await expect(
			canvas.queryByRole("button", {
				name: /api\.github\.com\/repos\/coder\/coder/i,
			}),
		).not.toBeInTheDocument();

		await userEvent.clear(input);
		await waitFor(() => {
			expect(canvas.getByText("Network calls (4)")).toBeInTheDocument();
			expect(promptByText("Summarize the project structure")).toBeVisible();
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

		const count = canvas.getByTestId("search-match-count");
		const status = canvas.getByRole("status");
		await expect(count).not.toBeVisible();
		await expect(status).toBeEmptyDOMElement();

		await userEvent.type(input, "list_directory");
		await waitFor(() => expect(status).toHaveTextContent("1 match"));
		await expect(count).toHaveTextContent("1 match");
		await expect(count).toBeVisible();
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
