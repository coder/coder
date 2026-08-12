import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockWorkspaceBuildLogs } from "#/testHelpers/entities";
import { WorkspaceBuildLogs } from "./WorkspaceBuildLogs";

const meta: Meta<typeof WorkspaceBuildLogs> = {
	title: "modules/workspaces/WorkspaceBuildLogs",
	component: WorkspaceBuildLogs,
};

export default meta;

type Story = StoryObj<typeof WorkspaceBuildLogs>;

export const InProgress: Story = {
	args: {
		logs: MockWorkspaceBuildLogs.slice(0, 20),
	},
};

export const Completed: Story = {
	args: {
		logs: MockWorkspaceBuildLogs,
	},
};
