import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgenticLoopTable } from "./AgenticLoopTable";

const meta: Meta<typeof AgenticLoopTable> = {
	title: "pages/AIBridgePage/SessionTimeline/AgenticLoopTable",
	component: AgenticLoopTable,
};

export default meta;
type Story = StoryObj<typeof AgenticLoopTable>;

export const Short: Story = {
	args: {
		duration: 4_200,
		toolCalls: 3,
		inputTokens: 1234,
		outputTokens: 567,
	},
};

export const Long: Story = {
	args: {
		duration: 125_000,
		toolCalls: 42,
		inputTokens: 150_000,
		outputTokens: 12_000,
	},
};

export const SingleToolCall: Story = {
	args: {
		duration: 980,
		toolCalls: 1,
		inputTokens: 320,
		outputTokens: 88,
	},
};

export const NoToolCalls: Story = {
	args: {
		duration: 500,
		toolCalls: 0,
		inputTokens: 100,
		outputTokens: 50,
	},
};
