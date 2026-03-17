import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
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
			actual: 750,
		},
	},
};

export default meta;
type Story = StoryObj<typeof AIGovernanceUsersConsumption>;

export const Default: Story = {};

export const Exceeded: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			actual: 1200,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1,200")).toBeInTheDocument();
		await expect(
			canvas.getByText(/of 1,000 Users Entitled/i),
		).toBeInTheDocument();
		await expect(canvas.getByText("Add-on exceeded")).toBeInTheDocument();
	},
};

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
