import type { Meta, StoryObj } from "@storybook/react-vite";
import { ManagedAgentsConsumption } from "./ManagedAgentsConsumption";

const meta: Meta<typeof ManagedAgentsConsumption> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/ManagedAgentsConsumption",
	component: ManagedAgentsConsumption,
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 50000,
			limit: 60000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export default meta;
type Story = StoryObj<typeof ManagedAgentsConsumption>;

export const Default: Story = {};

export const ZeroUsage: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 0,
			limit: 60000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const NearLimit: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 55000,
			limit: 60000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const OverLimit: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 80000,
			limit: 60000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const LowUsage: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 25000,
			limit: 60000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const Disabled: Story = {
	args: {
		managedAgentFeature: {
			enabled: false,
			actual: undefined,
			limit: undefined,
			usage_period: undefined,
			entitlement: "not_entitled",
		},
	},
};

export const NoFeature: Story = {
	args: {
		managedAgentFeature: undefined,
	},
};

// Error States for Validation
export const ErrorMissingData: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: undefined,
			limit: undefined,
			usage_period: undefined,
			entitlement: "entitled",
		},
	},
};

export const ErrorNegativeValues: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: -100,
			limit: 60000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const ErrorInvalidDates: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 50000,
			limit: 60000,
			usage_period: {
				start: "invalid-date",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const ErrorEndBeforeStart: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 50000,
			limit: 60000,
			usage_period: {
				start: "February 27, 2026",
				end: "February 27, 2025",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};
