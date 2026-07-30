import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
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

// Session did not pass through Agent Firewall: monitoring was not active.
export const NetworkDisabled: Story = {
	args: {
		...Default.args,
		networkCalls: undefined,
	},
	play: async ({ canvas }) => {
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
		await expect(canvas.queryByText("Blocked network requests")).toBeNull();
		await expect(canvas.queryByText("Top domains")).toBeNull();
	},
};

// Tabbing to the disabled indicator's info button and pressing Enter reveals
// the reason without a mouse.
export const NetworkDisabledKeyboard: Story = {
	args: {
		...Default.args,
		networkCalls: undefined,
	},
	play: async () => {
		await userEvent.tab();
		await userEvent.keyboard("{Enter}");
		await waitFor(() =>
			expect(screen.getByRole("dialog")).toHaveTextContent(
				"Network request monitoring was not active for this session.",
			),
		);
	},
};

// Firewall active but no egress recorded.
export const NetworkNoActivity: Story = {
	args: {
		...Default.args,
		networkCalls: { total: 0, blocked: 0 },
	},
	play: async ({ canvas }) => {
		await expect(canvas.getByText("No activity")).toBeInTheDocument();
		await expect(canvas.queryByText("Blocked network requests")).toBeNull();
	},
};

// Egress recorded, some blocked, across several domains.
export const NetworkActivity: Story = {
	args: {
		...Default.args,
		networkCalls: { total: 7, blocked: 2 },
		networkDomains: {
			topDomain: { domain: "api.github.com", count: 4 },
			totalCount: 14,
		},
	},
	play: async ({ canvas }) => {
		await expect(canvas.getByText("Network requests")).toBeInTheDocument();
		await expect(canvas.getByText("7")).toBeInTheDocument();
		await expect(
			canvas.getByText("Blocked network requests"),
		).toBeInTheDocument();
		await expect(canvas.getByText("api.github.com")).toBeInTheDocument();
		await expect(canvas.getByText("+13 more")).toBeInTheDocument();
	},
};

// A single domain contacted: no "+N more" overflow.
export const NetworkSingleDomain: Story = {
	args: {
		...Default.args,
		networkCalls: { total: 3, blocked: 0 },
		networkDomains: {
			topDomain: { domain: "api.github.com", count: 3 },
			totalCount: 1,
		},
	},
	play: async ({ canvas }) => {
		await expect(canvas.getByText("api.github.com")).toBeInTheDocument();
		await expect(canvas.queryByText(/more$/)).toBeNull();
	},
};
