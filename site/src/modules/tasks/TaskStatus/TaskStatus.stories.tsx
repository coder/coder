import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskStatus } from "./TaskStatus";

const meta: Meta<typeof TaskStatus> = {
	title: "modules/tasks/TaskStatus",
	component: TaskStatus,
};

export default meta;
type Story = StoryObj<typeof TaskStatus>;

export const Active: Story = {
	args: {
		status: "active",
		stateMessage: "Task is running smoothly",
	},
};

export const Failed: Story = {
	args: {
		status: "error",
		stateMessage: "Task encountered an error",
	},
};

export const Initializing: Story = {
	args: {
		status: "initializing",
		stateMessage: "Task is initializing",
	},
};

export const Pending: Story = {
	args: {
		status: "pending",
		stateMessage: "Task is pending",
	},
};

export const Paused: Story = {
	args: {
		status: "paused",
		stateMessage: "Task is paused",
	},
};

export const Unknown: Story = {
	args: {
		status: "unknown",
		stateMessage: "Task status is unknown",
	},
};
