import type { Meta, StoryObj } from "@storybook/react-vite";
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
		showCost: true,
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

export const WithSummary: Story = {};

export const NoSummary: Story = {
	args: { summary: null },
};

// A subagent's summary is its final report, persisted when it
// completes, so an empty summary means the agent is still working.
export const SubagentSummaryPending: Story = {
	args: { summary: null, isSubagent: true },
};

export const CostLoading: Story = {
	args: { isCostLoading: true, costMicros: undefined },
};

export const CostAbsent: Story = {
	args: { costMicros: null },
};

export const SubCentCost: Story = {
	args: { costMicros: 5_000 },
};

export const CostError: Story = {
	args: { costMicros: undefined, costError: true },
};

export const PartialCost: Story = {
	args: { costMicros: 0, unpricedRequestCount: 3 },
};

export const SubagentTreeCost: Story = {
	args: { isSubagent: true },
};

export const CostHidden: Story = {
	args: { showCost: false },
};
