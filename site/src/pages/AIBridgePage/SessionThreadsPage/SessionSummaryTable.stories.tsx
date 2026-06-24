import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockSession } from "#/testHelpers/entities";
import { mockNetworkActivity } from "./NetworkActivity/mocks";
import { SessionSummaryTable } from "./SessionSummaryTable";

const meta: Meta<typeof SessionSummaryTable> = {
	title: "pages/AIBridgePage/SessionSummaryTable",
	component: SessionSummaryTable,
};

export default meta;
type Story = StoryObj<typeof SessionSummaryTable>;

const baseArgs = {
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
};

export const Default: Story = {
	args: baseArgs,
};

export const InProgress: Story = {
	args: {
		...baseArgs,
		endTime: undefined,
	},
};

export const MultipleProviders: Story = {
	args: {
		...baseArgs,
		providers: ["anthropic", "openai", "copilot"],
	},
};

export const WithTokenMetadata: Story = {
	args: {
		...baseArgs,
		tokenUsageMetadata: {
			cache_read_input_tokens: 3200,
			cache_creation_input_tokens: 800,
		},
	},
};

export const LargeTokenCounts: Story = {
	args: {
		...baseArgs,
		inputTokens: 198_000,
		outputTokens: 32_000,
	},
};

export const NetworkNoActivity: Story = {
	args: {
		...baseArgs,
		networkActivity: mockNetworkActivity("none"),
	},
};

export const NetworkAllAllowed: Story = {
	args: {
		...baseArgs,
		networkActivity: mockNetworkActivity("all-allowed"),
	},
};

export const NetworkMixed: Story = {
	args: {
		...baseArgs,
		networkActivity: mockNetworkActivity("mixed"),
	},
};

export const NetworkErrorOnly: Story = {
	args: {
		...baseArgs,
		networkActivity: mockNetworkActivity("error-only"),
	},
};

export const NetworkMidSessionFailure: Story = {
	args: {
		...baseArgs,
		networkActivity: mockNetworkActivity("mid-session-failure"),
	},
};
