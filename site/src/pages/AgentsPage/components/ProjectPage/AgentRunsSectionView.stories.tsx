import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	MockChatProjectRuns,
	MockChatProjectTrigger,
	mockApiError,
} from "#/testHelpers/entities";
import { AgentRunsSectionView } from "./AgentRunsSection";

const meta: Meta<typeof AgentRunsSectionView> = {
	title: "pages/AgentsPage/ProjectPage/AgentRunsSectionView",
	component: AgentRunsSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		source: "automation",
		// The section shows only the runs of its source.
		runs: MockChatProjectRuns.filter((run) => run.trigger_id),
		triggers: [MockChatProjectTrigger],
		error: undefined,
		onRetry: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentRunsSectionView>;

export const Default: Story = {};

export const Delegated: Story = {
	args: {
		source: "delegation",
		runs: MockChatProjectRuns.filter(
			(run) => run.parent_chat_id && !run.trigger_id,
		),
	},
};

export const Empty: Story = { args: { runs: [] } };

export const Loading: Story = { args: { runs: undefined } };

export const LoadError: Story = {
	args: {
		runs: undefined,
		error: mockApiError({ message: "Failed to load runs." }),
	},
};

export const RefetchError: Story = {
	args: { error: mockApiError({ message: "Failed to refresh runs." }) },
};
