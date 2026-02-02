import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	StatusHealthyIndicator,
	StatusIndicator,
	StatusIndicatorDot,
	StatusNotHealthyIndicator,
	StatusNotReachableIndicator,
	StatusNotRegisteredIndicator,
} from "./StatusIndicator";

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

export const Healthy: Story = {
	args: {
		children: <StatusHealthyIndicator />,
	},
};

export const HealthyDERPOnly: Story = {
	args: {
		children: <StatusHealthyIndicator derpOnly />,
	},
};

export const NotHealthy: Story = {
	args: {
		children: <StatusNotHealthyIndicator />,
	},
};

export const NotReachable: Story = {
	args: {
		children: <StatusNotReachableIndicator />,
	},
};

export const NotRegistered: Story = {
	args: {
		children: <StatusNotRegisteredIndicator />,
	},
};
