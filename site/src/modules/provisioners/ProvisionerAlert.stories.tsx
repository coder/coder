import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { MockTemplateVersion } from "testHelpers/entities";
import { ProvisionerAlert } from "./ProvisionerAlert";

const meta: Meta<typeof ProvisionerAlert> = {
	title: "modules/provisioners/ProvisionerAlert",
	parameters: {
		chromatic,
		layout: "centered",
	},
	component: ProvisionerAlert,
	args: {
		matchingProvisioners: 0,
		availableProvisioners: 0,
		tags: MockTemplateVersion.job.tags,
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerAlert>;

export const HealthyProvisioners: Story = {
	args: {
		matchingProvisioners: 1,
		availableProvisioners: 1,
	}
};

export const NoMatchingProvisioners: Story = {
	args: {
		matchingProvisioners: 0,
	}
};

export const NoAvailableProvisioners: Story = {
	args: {
		matchingProvisioners: 1,
		availableProvisioners: 0,
	}
};
