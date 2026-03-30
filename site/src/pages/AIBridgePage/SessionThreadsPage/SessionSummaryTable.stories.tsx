import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockSession } from "#/testHelpers/entities";
import { SessionSummaryTable } from "./SessionSummaryTable";

const meta: Meta<typeof SessionSummaryTable> = {
	title: "pages/AIBridgePage/SessionSummaryTable",
	component: SessionSummaryTable,
};

export default meta;
type Story = StoryObj<typeof SessionSummaryTable>;

export const Default: Story = {
	args: {
		sessionId: MockSession.id,
		startTime: new Date(MockSession.started_at),
		endTime: new Date(MockSession.ended_at!),
		initiator: MockSession.initiator,
		client: MockSession.client ?? "Unknown",
		providers: MockSession.providers,
		inputTokens: MockSession.token_usage_summary.input_tokens,
		outputTokens: MockSession.token_usage_summary.output_tokens,
		threadCount: MockSession.threads,
		toolCallCount: 12,
	},
};

export const InProgress: Story = {
	args: {
		...Default.args,
		endTime: undefined,
	},
};

export const MultipleProviders: Story = {
	args: {
		...Default.args,
		providers: ["anthropic", "openai", "copilot"],
	},
};

export const WithTokenMetadata: Story = {
	args: {
		...Default.args,
		tokenUsageMetadata: {
			cache_read_input_tokens: 3200,
			cache_creation_input_tokens: 800,
		},
	},
};

export const LargeTokenCounts: Story = {
	args: {
		...Default.args,
		inputTokens: 198_000,
		outputTokens: 32_000,
	},
};
