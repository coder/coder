import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent } from "storybook/test";
import type { AIBridgePricedModel } from "#/api/typesGenerated";
import { PromptTable } from "./PromptTable";

const mockPricedModel: AIBridgePricedModel = {
	model: "claude-sonnet-4-5",
	input_price: 3_000_000,
	output_price: 15_000_000,
	cache_read_price: 300_000,
	cache_write_price: 3_750_000,
};

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
		pricedModel: mockPricedModel,
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

export const ModelPrices: Story = {
	args: Anthropic.args,
	play: async ({ canvas }) => {
		await userEvent.hover(
			canvas.getByRole("button", { name: "claude-sonnet-4-5" }),
		);
	},
};

export const PriceBelowThreshold: Story = {
	args: {
		...Anthropic.args,
		pricedModel: {
			...mockPricedModel,
			cache_read_price: 50,
		},
	},
	play: ModelPrices.play,
};

// The requested model has no price, so the cost uses the price of the model
// reported by the provider.
export const PricedAsProviderModel: Story = {
	args: {
		...OpenAI.args,
		model: "codex-auto-review",
		pricedModel: {
			model: "gpt-5.6-luna",
			input_price: 200_000,
			output_price: 1_200_000,
			cache_read_price: 20_000,
			cache_write_price: 250_000,
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
