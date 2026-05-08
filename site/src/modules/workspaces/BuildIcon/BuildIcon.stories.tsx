import type { Meta, StoryObj } from "@storybook/react-vite";
import { BuildIcon } from "./BuildIcon";

const meta: Meta<typeof BuildIcon> = {
	title: "components/BuildIcon",
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

export const Delete: Story = {
	args: {
		transition: "delete",
	},
};

export const FailedDelete: Story = {
	args: {
		transition: "delete",
		jobStatus: "failed",
	},
};
