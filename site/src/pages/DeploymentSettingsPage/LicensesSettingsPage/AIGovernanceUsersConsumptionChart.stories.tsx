import type { Meta, StoryObj } from "@storybook/react-vite";
import { AIGovernanceUsersConsumption } from "./AIGovernanceUsersConsumptionChart";

const meta: Meta<typeof AIGovernanceUsersConsumption> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/AIGovernanceUsersConsumptionChart",
	component: AIGovernanceUsersConsumption,
	args: {
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
		},
	},
};

export default meta;
type Story = StoryObj<typeof AIGovernanceUsersConsumption>;

export const Default: Story = {};

export const Disabled: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: false,
			entitlement: "not_entitled",
		},
	},
};

export const NoFeature: Story = {
	args: {
		aiGovernanceUserFeature: undefined,
	},
};

export const ErrorMissingData: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: undefined,
		},
	},
};

export const ErrorNegativeValues: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: -1000,
		},
	},
};
