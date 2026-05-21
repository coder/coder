import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatCostSummaryView } from "./ChatCostSummaryView";

const buildSummary = (
	overrides: Partial<TypesGen.ChatCostSummary> = {},
): TypesGen.ChatCostSummary => ({
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	total_cost_micros: 1_500_000,
	priced_message_count: 12,
	unpriced_message_count: 0,
	total_input_tokens: 123_456,
	total_output_tokens: 654_321,
	total_cache_read_tokens: 9_876,
	total_cache_creation_tokens: 5_432,
	total_runtime_ms: 0,
	by_model: [
		{
			model_config_id: "model-config-1",
			display_name: "GPT-4.1",
			provider: "OpenAI",
			model: "gpt-4.1",
			total_cost_micros: 1_250_000,
			message_count: 9,
			total_input_tokens: 100_000,
			total_output_tokens: 200_000,
			total_cache_read_tokens: 7_654,
			total_cache_creation_tokens: 3_210,
			total_runtime_ms: 0,
		},
	],
	by_chat: [
		{
			root_chat_id: "chat-1",
			chat_title: "Quarterly review",
			total_cost_micros: 750_000,
			message_count: 5,
			total_input_tokens: 60_000,
			total_output_tokens: 80_000,
			total_cache_read_tokens: 4_321,
			total_cache_creation_tokens: 1_234,
			total_runtime_ms: 0,
		},
	],
	...overrides,
});

const emptySummary = buildSummary({
	total_cost_micros: 0,
	priced_message_count: 0,
	unpriced_message_count: 0,
	total_input_tokens: 0,
	total_output_tokens: 0,
	by_model: [],
	by_chat: [],
});

const meta: Meta<typeof ChatCostSummaryView> = {
	title: "pages/AgentsPage/ChatCostSummaryView",
	component: ChatCostSummaryView,
	args: {
		summary: undefined,
		isLoading: false,
		error: undefined,
		onRetry: fn(),
		loadingLabel: "Loading usage details",
		emptyMessage: "No usage details available.",
	},
};

export default meta;
type Story = StoryObj<typeof ChatCostSummaryView>;

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

const ErrorState: Story = {
	args: {
		error: new globalThis.Error("Failed to fetch"),
		onRetry: fn(),
	},
};

export { ErrorState as Error };

export const ErrorNonError: Story = {
	args: {
		error: "string error",
		onRetry: fn(),
	},
};

export const Empty: Story = {
	args: {
		summary: emptySummary,
	},
};

export const WithData: Story = {
	args: {
		summary: buildSummary(),
	},
};

export const UnpricedWarning: Story = {
	args: {
		summary: buildSummary({
			unpriced_message_count: 2,
		}),
	},
};

const manyModels = Array.from({ length: 12 }, (_, i) => ({
	model_config_id: `model-${i + 1}`,
	display_name: `Model ${i + 1}`,
	provider: "TestProvider",
	model: `test-model-${i + 1}`,
	total_cost_micros: 100_000 * (i + 1),
	message_count: i + 1,
	total_input_tokens: 10_000 * (i + 1),
	total_output_tokens: 20_000 * (i + 1),
	total_cache_read_tokens: 1_000,
	total_cache_creation_tokens: 500,
	total_runtime_ms: 0,
}));

const manyChats = Array.from({ length: 12 }, (_, i) => ({
	root_chat_id: `chat-${i + 1}`,
	chat_title: `Agent ${i + 1}`,
	total_cost_micros: 50_000 * (i + 1),
	message_count: i + 1,
	total_input_tokens: 5_000 * (i + 1),
	total_output_tokens: 10_000 * (i + 1),
	total_cache_read_tokens: 500,
	total_cache_creation_tokens: 250,
	total_runtime_ms: 0,
}));

export const PaginatedChats: Story = {
	args: {
		summary: buildSummary({ by_chat: manyChats }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// First page shows agents 1–10, agent 11 is on page 2.
		await canvas.findByText("Agent 1");
		await expect(canvas.queryByText("Agent 11")).not.toBeInTheDocument();

		// Navigate to page 2 (second pagination widget on the page).
		const nextButtons = canvas.getAllByRole("button", { name: /next/i });
		await userEvent.click(nextButtons[nextButtons.length - 1]);

		await expect(canvas.getByText("Agent 11")).toBeInTheDocument();
		await expect(canvas.getByText("Agent 12")).toBeInTheDocument();
		await expect(canvas.queryByText("Agent 1")).not.toBeInTheDocument();
	},
};

export const PaginatedModels: Story = {
	args: {
		summary: buildSummary({ by_model: manyModels }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// First page shows models 1–10, model 11 is on page 2.
		await canvas.findByText("Model 1");
		await expect(canvas.queryByText("Model 11")).not.toBeInTheDocument();

		// Navigate to page 2.
		const nextButton = canvas.getByRole("button", { name: /next/i });
		await userEvent.click(nextButton);

		await expect(canvas.getByText("Model 11")).toBeInTheDocument();
		await expect(canvas.getByText("Model 12")).toBeInTheDocument();
		await expect(canvas.queryByText("Model 1")).not.toBeInTheDocument();
	},
};
