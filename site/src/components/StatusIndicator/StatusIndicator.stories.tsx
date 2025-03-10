import type { Meta, StoryObj } from "@storybook/react";
import { StatusIndicator, StatusIndicatorDot } from "./StatusIndicator";

const meta: Meta<typeof StatusIndicator> = {
	title: "components/StatusIndicator",
	component: StatusIndicator,
	args: {
		children: (
			<>
				<StatusIndicatorDot />
				Status
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof StatusIndicator>;

export const Success: Story = {
	args: {
		variant: "success",
	},
};

export const Failed: Story = {
	args: {
		variant: "failed",
	},
};

export const Inactive: Story = {
	args: {
		variant: "inactive",
	},
};

export const Warning: Story = {
	args: {
		variant: "warning",
	},
};

export const Pending: Story = {
	args: {
		variant: "pending",
	},
};

export const Small: Story = {
	args: {
		variant: "success",
		size: "sm",
	},
};
