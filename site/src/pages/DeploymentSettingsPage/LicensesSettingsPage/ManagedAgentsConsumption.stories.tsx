import type { Meta, StoryObj } from "@storybook/react";
import { ManagedAgentsConsumption } from "./ManagedAgentsConsumption";

const meta: Meta<typeof ManagedAgentsConsumption> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/ManagedAgentsConsumption",
	component: ManagedAgentsConsumption,
	args: {
		usage: 50000,
		included: 60000,
		limit: 120000,
		startDate: "February 27, 2025",
		endDate: "February 27, 2026",
	},
};

export default meta;
type Story = StoryObj<typeof ManagedAgentsConsumption>;

export const Default: Story = {};

export const NearLimit: Story = {
	args: {
		usage: 115000,
		included: 60000,
		limit: 120000,
	},
};

export const OverIncluded: Story = {
	args: {
		usage: 80000,
		included: 60000,
		limit: 120000,
	},
};

export const LowUsage: Story = {
	args: {
		usage: 25000,
		included: 60000,
		limit: 120000,
	},
};

export const IncludedAtLimit: Story = {
	args: {
		usage: 25000,
		included: 30500,
		limit: 30500,
	},
};

export const Disabled: Story = {
	args: {
		enabled: false,
		usage: Number.NaN,
		included: Number.NaN,
		limit: Number.NaN,
	},
};
