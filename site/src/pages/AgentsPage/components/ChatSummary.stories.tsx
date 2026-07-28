import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ChatSummary } from "./ChatSummary";

const meta: Meta<typeof ChatSummary> = {
	title: "pages/AgentsPage/ChatSummary",
	component: ChatSummary,
	args: {
		summary:
			"Investigated the flaky CI job, traced it to a race in the cache layer, and added a regression test.",
		createdAt: "2024-05-01T12:00:00Z",
		updatedAt: "2024-05-02T15:30:00Z",
		costMicros: 1_250_000,
	},
	decorators: [
		(Story) => (
			<div className="w-[400px] max-w-full p-4">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ChatSummary>;

export const WithSummary: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Created:")).toBeInTheDocument();
		await expect(canvas.getByText("Updated:")).toBeInTheDocument();
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("May 1, 2024")).toBeInTheDocument();
		await expect(canvas.getByText("May 2, 2024")).toBeInTheDocument();
		await expect(canvas.queryByText(/12:00|15:30/)).not.toBeInTheDocument();
		// formatCostMicros is locale-pinned to en-US, so this is deterministic.
		await expect(canvas.getByText("$1.25")).toBeInTheDocument();
	},
};

export const NoSummary: Story = {
	args: { summary: null },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No summary yet.")).toBeInTheDocument();
	},
};

// A subagent's summary is its final report, persisted when it
// completes, so an empty summary means the agent is still working.
export const SubagentSummaryPending: Story = {
	args: { summary: null, isSubagent: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Summary pending agent completion."),
		).toBeInTheDocument();
	},
};

export const CostLoading: Story = {
	args: { isCostLoading: true, costMicros: undefined },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Loading cost")).toBeInTheDocument();
	},
};

export const CostAbsent: Story = {
	args: { costMicros: null },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("-")).toBeInTheDocument();
	},
};

export const SubCentCost: Story = {
	args: { costMicros: 5_000 },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("$0.0050")).toBeInTheDocument();
	},
};

export const CostError: Story = {
	args: { costMicros: undefined, costError: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("Unavailable")).toBeInTheDocument();
	},
};

export const PartialCost: Story = {
	args: { costMicros: 0, unpricedMessagesHavingUsageCount: 3 },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"Excludes 3 messages with usage but without model pricing.",
			),
		).toBeInTheDocument();
	},
};
