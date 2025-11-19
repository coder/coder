import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskStartupWarningButton } from "./TaskStartupWarningButton";

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
		lifecycleState: "start_error",
	},
};

export const StartTimeout: Story = {
	args: {
		lifecycleState: "start_timeout",
	},
};

export const NoWarning: Story = {
	args: {
		lifecycleState: "ready",
	},
};

export const NullLifecycle: Story = {
	args: {
		lifecycleState: null,
	},
};
