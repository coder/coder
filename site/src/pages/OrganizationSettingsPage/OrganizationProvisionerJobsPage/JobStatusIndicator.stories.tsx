import type { Meta, StoryObj } from "@storybook/react";
import { JobStatusIndicator } from "./JobStatusIndicator";
import { MockProvisionerJob } from "testHelpers/entities";

const meta: Meta<typeof JobStatusIndicator> = {
	title: "pages/OrganizationProvisionerJobsPage/JobStatusIndicator",
	component: JobStatusIndicator,
};

export default meta;
type Story = StoryObj<typeof JobStatusIndicator>;

export const Succeeded: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "succeeded",
		},
	},
};

export const Failed: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "failed",
		},
	},
};

export const Pending: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "pending",
			queue_position: 1,
			queue_size: 1,
		},
	},
};

export const Running: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "running",
		},
	},
};

export const Canceling: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "canceling",
		},
	},
};

export const Canceled: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "canceled",
		},
	},
};

export const Unknown: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "unknown",
		},
	},
};
