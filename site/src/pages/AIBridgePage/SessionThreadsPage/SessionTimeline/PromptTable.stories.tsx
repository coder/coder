import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent } from "storybook/test";
import { PromptTable } from "./PromptTable";

const meta: Meta<typeof PromptTable> = {
	title: "pages/AIBridgePage/SessionTimeline/PromptTable",
	component: PromptTable,
};

export default meta;
type Story = StoryObj<typeof PromptTable>;

export const Anthropic: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		model: "claude-sonnet-4-5",
		pricedModel: {
			model: "claude-sonnet-4-5",
			input_price: 3_000_000,
			output_price: 15_000_000,
			cache_read_price: 300_000,
			cache_write_price: 3_750_000,
		},
		inputTokens: 1234,
		outputTokens: 567,
		cacheReadTokens: 980,
		cacheWriteTokens: 120,
		costMicros: 12_550,
		hasUnpricedUsage: false,
	},
};

export const OpenAI: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		model: "gpt-4o",
		pricedModel: {
			model: "gpt-4o",
			input_price: 2_500_000,
			output_price: 10_000_000,
			cache_read_price: 1_250_000,
			cache_write_price: null,
		},
		inputTokens: 8192,
		outputTokens: 1024,
		cacheReadTokens: 4096,
		cacheWriteTokens: 0,
		costMicros: 35_840,
		hasUnpricedUsage: false,
	},
};

export const WithTokenMetadata: Story = {
	args: {
		...Anthropic.args,
		inputTokens: 5000,
		outputTokens: 2500,
		cacheReadTokens: 3200,
		cacheWriteTokens: 800,
		tokenUsageMetadata: {
			cache_read_input_tokens: 3200,
			cache_creation_input_tokens: 800,
		},
	},
};

export const LargeTokenCounts: Story = {
	args: {
		...Anthropic.args,
		model: "claude-opus-4-5",
		pricedModel: {
			model: "claude-opus-4-5",
			input_price: 5_000_000,
			output_price: 25_000_000,
			cache_read_price: 500_000,
			cache_write_price: 6_250_000,
		},
		inputTokens: 198_000,
		outputTokens: 8_000,
		cacheReadTokens: 1_635_778,
		cacheWriteTokens: 130_734,
		costMicros: 2_824_476,
	},
};

// Opens the model tooltip so the screenshot captures the unit prices.
export const ModelPrices: Story = {
	args: Anthropic.args,
	play: async ({ canvas }) => {
		await userEvent.hover(
			canvas.getByRole("button", { name: "claude-sonnet-4-5" }),
		);
	},
};

// The price lookup fell back to the model reported by the provider.
export const PricedAsProviderModel: Story = {
	args: {
		...Anthropic.args,
		pricedModel: {
			model: "claude-sonnet-4-5-20250929",
			input_price: 3_000_000,
			output_price: 15_000_000,
			cache_read_price: 300_000,
			cache_write_price: 3_750_000,
		},
	},
	play: async ({ canvas }) => {
		await userEvent.hover(
			canvas.getByRole("button", { name: "Pricing warning" }),
		);
	},
};

export const Unpriced: Story = {
	args: {
		...Anthropic.args,
		pricedModel: undefined,
		costMicros: 0,
		hasUnpricedUsage: true,
	},
};
