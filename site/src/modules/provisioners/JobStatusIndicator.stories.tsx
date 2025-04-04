import type { Meta, StoryObj } from "@storybook/react";
import { MockProvisionerJob } from "testHelpers/entities";
import { JobStatusIndicator } from "./JobStatusIndicator";

const meta: Meta<typeof JobStatusIndicator> = {
	title: "modules/provisioners/JobStatusIndicator",
	component: JobStatusIndicator,
};

export default meta;
type Story = StoryObj<typeof JobStatusIndicator>;

export const Succeeded: Story = {
	args: {
		status: "succeeded",
	},
};

export const Failed: Story = {
	args: {
		status: "failed",
	},
};

export const Pending: Story = {
	args: {
		status: "pending",
		queue: { size: 1, position: 1 },
	},
};

export const Running: Story = {
	args: {
		status: "running",
	},
};

export const Canceling: Story = {
	args: {
		status: "canceling",
	},
};

export const Canceled: Story = {
	args: {
		status: "canceled",
	},
};

export const Unknown: Story = {
	args: {
		status: "unknown",
	},
};
