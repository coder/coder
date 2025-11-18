import { MockTask } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskStartupWarningButton } from "./TaskStartupAlert";

const meta: Meta<typeof TaskStartupWarningButton> = {
	title: "pages/TaskPage/TaskStartupWarningButton",
	component: TaskStartupWarningButton,
	parameters: {
		layout: "padded",
	},
};

export default meta;
type Story = StoryObj<typeof TaskStartupWarningButton>;

export const StartError: Story = {
	args: {
		task: {
			...MockTask,
			workspace_agent_lifecycle: "start_error",
		},
	},
};

export const StartTimeout: Story = {
	args: {
		task: {
			...MockTask,
			workspace_agent_lifecycle: "start_timeout",
		},
	},
};

export const NoWarning: Story = {
	args: {
		task: {
			...MockTask,
			workspace_agent_lifecycle: "ready",
		},
	},
};

export const NullLifecycle: Story = {
	args: {
		task: {
			...MockTask,
			workspace_agent_lifecycle: null,
		},
	},
};
