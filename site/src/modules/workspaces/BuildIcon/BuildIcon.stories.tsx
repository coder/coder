import type { Meta, StoryObj } from "@storybook/react-vite";
import { BuildIcon } from "./BuildIcon";

const meta: Meta<typeof BuildIcon> = {
	title: "modules/workspaces/BuildIcon",
	component: BuildIcon,
	args: {
		jobStatus: "succeeded",
	},
};

export default meta;
type Story = StoryObj<typeof BuildIcon>;

export const Start: Story = {
	args: {
		transition: "start",
	},
};

export const StartPending: Story = {
	args: {
		transition: "start",
		jobStatus: "pending",
	},
};

export const StartRunning: Story = {
	args: {
		transition: "start",
		jobStatus: "running",
	},
};

export const StartCanceling: Story = {
	args: {
		transition: "start",
		jobStatus: "canceling",
	},
};

export const StartCanceled: Story = {
	args: {
		transition: "start",
		jobStatus: "canceled",
	},
};

export const Stop: Story = {
	args: {
		transition: "stop",
	},
};

export const PendingStop: Story = {
	args: {
		transition: "stop",
		jobStatus: "pending",
	},
};

export const UnknownStop: Story = {
	args: {
		transition: "stop",
		jobStatus: "unknown",
	},
};

export const Delete: Story = {
	args: {
		transition: "delete",
	},
};

export const DeleteFailed: Story = {
	args: {
		transition: "delete",
		jobStatus: "failed",
	},
};
