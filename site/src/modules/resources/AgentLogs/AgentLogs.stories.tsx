import type { Meta, StoryObj } from "@storybook/react-vite";
import { AGENT_LOG_LINE_HEIGHT } from "./AgentLogLine";
import { AgentLogs } from "./AgentLogs";
import { MockLogs, MockSources } from "./mocks";

const meta: Meta<typeof AgentLogs> = {
	title: "modules/resources/AgentLogs",
	component: AgentLogs,
	args: {
		sources: MockSources,
		logs: MockLogs,
		height: MockLogs.length * AGENT_LOG_LINE_HEIGHT,
		overflowed: false,
	},
	parameters: {
		layout: "fullscreen",
	},
};

export default meta;
type Story = StoryObj<typeof AgentLogs>;

export const Default: Story = {};

export const Overflowed: Story = {
	args: {
		className: "max-h-[420px]",
		overflowed: true,
	},
};
