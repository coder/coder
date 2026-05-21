import type { Meta, StoryObj } from "@storybook/react-vite";
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
		inputTokens: 1234,
		outputTokens: 567,
	},
};

export const OpenAI: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		model: "gpt-4o",
		inputTokens: 8192,
		outputTokens: 1024,
	},
};

export const WithTokenMetadata: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		model: "claude-sonnet-4-5",
		inputTokens: 5000,
		outputTokens: 2500,
		tokenUsageMetadata: {
			cache_read_input_tokens: 3200,
			cache_creation_input_tokens: 800,
		},
	},
};

export const LargeTokenCounts: Story = {
	args: {
		timestamp: new Date("2025-03-19T14:22:00Z"),
		model: "claude-opus-4-5",
		inputTokens: 198_000,
		outputTokens: 8_000,
	},
};
