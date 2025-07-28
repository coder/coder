import type { Meta, StoryObj } from "@storybook/react";
import { ManagedAgentsConsumption } from "./ManagedAgentsConsumption";

const meta: Meta<typeof ManagedAgentsConsumption> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/ManagedAgentsConsumption",
	component: ManagedAgentsConsumption,
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 50000,
			soft_limit: 60000,
			limit: 120000,
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

export const Default: Story = {
	name: "Normal Usage (42% of limit)",
};

export const Warning: Story = {
	name: "Warning (80-99% of limit)",
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 96000, // 80% of limit - should show orange
			soft_limit: 60000,
			limit: 120000,
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
	name: "Near Limit (95% of limit)",
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 114000, // 95% of limit - should show orange
			soft_limit: 60000,
			limit: 120000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const AtLimit: Story = {
	name: "At Limit (100% of limit)",
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 120000, // 100% of limit - should show red
			soft_limit: 60000,
			limit: 120000,
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
	name: "Over Limit (120% of limit)",
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 144000, // 120% of limit - should show red
			soft_limit: 60000,
			limit: 120000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const OverIncluded: Story = {
	name: "Over Included (67% of limit)",
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 80000, // Over included but under 80% of total limit - should still be green
			soft_limit: 60000,
			limit: 120000,
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
	name: "Low Usage (21% of limit)",
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 25000,
			soft_limit: 60000,
			limit: 120000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const IncludedAtLimit: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 25000,
			soft_limit: 30500,
			limit: 30500,
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
			soft_limit: undefined,
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
			soft_limit: undefined,
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
			soft_limit: 60000,
			limit: 120000,
			usage_period: {
				start: "February 27, 2025",
				end: "February 27, 2026",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};

export const ErrorSoftLimitExceedsLimit: Story = {
	args: {
		managedAgentFeature: {
			enabled: true,
			actual: 50000,
			soft_limit: 150000,
			limit: 120000,
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
			soft_limit: 60000,
			limit: 120000,
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
			soft_limit: 60000,
			limit: 120000,
			usage_period: {
				start: "February 27, 2026",
				end: "February 27, 2025",
				issued_at: "February 27, 2025",
			},
			entitlement: "entitled",
		},
	},
};
