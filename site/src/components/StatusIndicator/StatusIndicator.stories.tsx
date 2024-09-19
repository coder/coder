import type { Meta, StoryObj } from "@storybook/react";
import { StatusIndicator } from "./StatusIndicator";

const meta: Meta<typeof StatusIndicator> = {
	title: "components/StatusIndicator",
	component: StatusIndicator,
	args: {},
};

export default meta;
type Story = StoryObj<typeof StatusIndicator>;

export const Success: Story = {
	args: {
		color: "success",
	},
};

export const SuccessOutline: Story = {
	args: {
		color: "success",
		variant: "outlined",
	},
};

export const Warning: Story = {
	args: {
		color: "warning",
	},
};

export const WarningOutline: Story = {
	args: {
		color: "warning",
		variant: "outlined",
	},
};

export const Danger: Story = {
	args: {
		color: "danger",
	},
};

export const DangerOutline: Story = {
	args: {
		color: "danger",
		variant: "outlined",
	},
};

export const Inactive: Story = {
	args: {
		color: "inactive",
	},
};

export const InactiveOutline: Story = {
	args: {
		color: "inactive",
		variant: "outlined",
	},
};
