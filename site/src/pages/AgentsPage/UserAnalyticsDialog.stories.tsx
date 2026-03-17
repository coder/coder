import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import dayjs from "dayjs";
import { expect, fn, screen, spyOn, waitFor } from "storybook/test";
import { UserAnalyticsDialog } from "./UserAnalyticsDialog";

const mockSummary: TypesGen.ChatCostSummary = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	total_cost_micros: 1_500_000,
	priced_message_count: 12,
	unpriced_message_count: 1,
	total_input_tokens: 123_456,
	total_output_tokens: 654_321,
	total_cache_read_tokens: 9_876,
	total_cache_creation_tokens: 5_432,
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
		},
	],
};

const meta: Meta<typeof UserAnalyticsDialog> = {
	title: "pages/AgentsPage/UserAnalyticsDialog",
	component: UserAnalyticsDialog,
	decorators: [withAuthProvider],
	parameters: {
		user: MockUserOwner,
	},
	beforeEach: () => {
		spyOn(API, "getChatCostSummary").mockResolvedValue(mockSummary);
	},
};

export default meta;
type Story = StoryObj<typeof UserAnalyticsDialog>;

export const Default: Story = {
	args: {
		open: true,
		onOpenChange: fn(),
		now: dayjs("2026-03-12T12:00:00Z"),
	},
	play: async () => {
		await waitFor(() => {
			expect(screen.getByText(/Feb 10\s*–\s*Mar 12, 2026/)).toBeInTheDocument();
		});
	},
};
