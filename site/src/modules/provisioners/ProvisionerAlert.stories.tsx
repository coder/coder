import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { AlertVariant, ProvisionerAlert } from "./ProvisionerAlert";

const meta: Meta<typeof ProvisionerAlert> = {
	title: "modules/provisioners/ProvisionerAlert",
	parameters: {
		chromatic,
		layout: "centered",
	},
	component: ProvisionerAlert,
	args: {
		title: "Title",
		detail: "Detail",
		severity: "info",
		tags: { tag: "tagValue" },
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerAlert>;

export const Info: Story = {};

export const InfoStyledForLogs: Story = {
	args: {
		variant: AlertVariant.InLogs,
	},
};

export const Warning: Story = {
	args: {
		severity: "warning",
	},
};

export const WarningStyledForLogs: Story = {
	args: {
		severity: "warning",
		variant: AlertVariant.InLogs,
	},
};

export const NullTags: Story = {
	args: {
		tags: undefined,
	},
};
