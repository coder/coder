import type { Meta, StoryObj } from "@storybook/react-vite";
import { TokenBadges } from "./TokenBadges";

const meta: Meta<typeof TokenBadges> = {
	title: "pages/AIBridgePage/TokenBadges",
	component: TokenBadges,
};

export default meta;
type Story = StoryObj<typeof TokenBadges>;

export const Default: Story = {
	args: {
		inputTokens: 1234,
		outputTokens: 567,
	},
};

export const LargeTokenCounts: Story = {
	args: {
		inputTokens: 128000,
		outputTokens: 32000,
	},
};

export const SmallTokenCounts: Story = {
	args: {
		inputTokens: 42,
		outputTokens: 8,
	},
};

export const WithMetadata: Story = {
	args: {
		inputTokens: 5000,
		outputTokens: 2500,
		tokenUsageMetadata: {
			cache_read_input_tokens: 3200,
			cache_creation_input_tokens: 800,
		},
	},
};

export const SizeXs: Story = {
	args: {
		size: "xs",
		inputTokens: 1234,
		outputTokens: 567,
	},
};

export const SizeMd: Story = {
	args: {
		size: "md",
		inputTokens: 1234,
		outputTokens: 567,
	},
};
