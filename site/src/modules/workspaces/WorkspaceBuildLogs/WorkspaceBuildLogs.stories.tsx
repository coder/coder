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

export const OneSecond: Story = {
	args: {
		logs: [
			{
				...MockWorkspaceBuildLogs[0],
				created_at: "2026-06-01T12:00:00.000Z",
			},
			{
				...MockWorkspaceBuildLogs[0],
				id: 2,
				created_at: "2026-06-01T12:00:01.000Z",
			},
		],
	},
};
