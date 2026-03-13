import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import { fn } from "storybook/test";
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
