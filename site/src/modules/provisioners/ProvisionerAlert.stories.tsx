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

export const InfoInline: Story = {
	args: {
		variant: AlertVariant.Inline,
	},
};

export const Warning: Story = {
	args: {
		severity: "warning",
	},
};

export const WarningInline: Story = {
	args: {
		severity: "warning",
		variant: AlertVariant.Inline,
	},
};

export const NullTags: Story = {
	args: {
		tags: undefined,
	},
};
