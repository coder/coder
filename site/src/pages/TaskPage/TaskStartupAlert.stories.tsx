import {
	MockWorkspaceAgentReady,
	MockWorkspaceAgentStartError,
	MockWorkspaceAgentStartTimeout,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskStartupAlert } from "./TaskStartupAlert";

const meta: Meta<typeof TaskStartupAlert> = {
	title: "pages/TaskPage/TaskStartupAlert",
	component: TaskStartupAlert,
	parameters: {
		layout: "padded",
	},
};

export default meta;
type Story = StoryObj<typeof TaskStartupAlert>;

export const StartError: Story = {
	args: {
		agent: MockWorkspaceAgentStartError,
	},
};

export const StartTimeout: Story = {
	args: {
		agent: MockWorkspaceAgentStartTimeout,
	},
};

export const NoAlert: Story = {
	args: {
		agent: MockWorkspaceAgentReady,
	},
};
