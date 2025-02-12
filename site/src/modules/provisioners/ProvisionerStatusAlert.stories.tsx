import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { MockTemplateVersion } from "testHelpers/entities";
import { AlertVariant } from "./ProvisionerAlert";
import { ProvisionerStatusAlert } from "./ProvisionerStatusAlert";

const meta: Meta<typeof ProvisionerStatusAlert> = {
	title: "modules/provisioners/ProvisionerStatusAlert",
	parameters: {
		chromatic,
		layout: "centered",
	},
	component: ProvisionerStatusAlert,
	args: {
		matchingProvisioners: 0,
		availableProvisioners: 0,
		tags: MockTemplateVersion.job.tags,
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerStatusAlert>;

export const HealthyProvisioners: Story = {
	args: {
		matchingProvisioners: 1,
		availableProvisioners: 1,
	},
};

export const UndefinedMatchingProvisioners: Story = {
	args: {
		matchingProvisioners: undefined,
		availableProvisioners: undefined,
	},
};

export const UndefinedAvailableProvisioners: Story = {
	args: {
		matchingProvisioners: 1,
		availableProvisioners: undefined,
	},
};

export const NoMatchingProvisioners: Story = {
	args: {
		matchingProvisioners: 0,
	},
};

export const NoMatchingProvisionersInLogs: Story = {
	args: {
		matchingProvisioners: 0,
		variant: AlertVariant.Inline,
	},
};

export const NoAvailableProvisioners: Story = {
	args: {
		matchingProvisioners: 1,
		availableProvisioners: 0,
	},
};

export const NoAvailableProvisionersInLogs: Story = {
	args: {
		matchingProvisioners: 1,
		availableProvisioners: 0,
		variant: AlertVariant.Inline,
	},
};
