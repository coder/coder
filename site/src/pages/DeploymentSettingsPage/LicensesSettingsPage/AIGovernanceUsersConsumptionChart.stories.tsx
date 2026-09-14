import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { GetLicensesResponse } from "#/api/api";
import { AIGovernanceUsersConsumption } from "./AIGovernanceUsersConsumptionChart";

const licenseWithAiGovernanceAddOn: GetLicensesResponse = {
	id: 42,
	uploaded_at: "1660104000",
	expires_at: "3420244800",
	uuid: "license-ai-gov-addon",
	claims: {
		trial: false,
		all_features: true,
		feature_set: "premium",
		version: 1,
		addons: ["ai_governance"],
		features: { ai_governance_user_limit: 750 },
		license_expires: 3420244800,
		nbf: 1660104000,
	},
};

const higherApplicableAiGovernanceLicense: GetLicensesResponse = {
	id: 43,
	uploaded_at: "1660104000",
	expires_at: "3420244800",
	uuid: "license-ai-gov-addon-higher",
	claims: {
		trial: false,
		all_features: true,
		feature_set: "premium",
		version: 1,
		addons: ["ai_governance"],
		features: { ai_governance_user_limit: 900 },
		license_expires: 3420244800,
		nbf: 1660104000,
	},
};

const nonApplicableAiGovernanceLicense: GetLicensesResponse = {
	id: 44,
	uploaded_at: "1660104000",
	expires_at: "3420244800",
	uuid: "license-ai-gov-addon-non-applicable",
	claims: {
		trial: false,
		all_features: true,
		feature_set: "enterprise",
		version: 1,
		addons: ["ai_governance"],
		features: { ai_governance_user_limit: 1200 },
		license_expires: 3420244800,
		nbf: 1660104000,
	},
};

const meta: Meta<typeof AIGovernanceUsersConsumption> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/AIGovernanceUsersConsumptionChart",
	component: AIGovernanceUsersConsumption,
	args: {
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			actual: 512,
		},
		licenses: [],
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
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
	},
};

export const Disabled: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: false,
			entitlement: "not_entitled",
		},
		licenses: [],
	},
};

export const NoFeature: Story = {
	args: {
		aiGovernanceUserFeature: undefined,
		licenses: [],
	},
};

/** Entitlements not enabled, but a Premium license lists the add-on and limit in JWT claims. */
export const UsageBarFromLicenseClaims: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: false,
			entitlement: "not_entitled",
		},
		licenses: [licenseWithAiGovernanceAddOn],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("750")).toBeInTheDocument();
		await expect(
			canvas.getByRole("heading", { name: "AI Governance add-on usage" }),
		).toBeInTheDocument();
	},
};

/** Picks the highest applicable limit when multiple licenses are present. */
export const UsageBarUsesHighestApplicableLicenseLimit: Story = {
	args: {
		aiGovernanceUserFeature: {
			enabled: false,
			entitlement: "not_entitled",
		},
		licenses: [
			licenseWithAiGovernanceAddOn,
			higherApplicableAiGovernanceLicense,
			nonApplicableAiGovernanceLicense,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("900")).toBeInTheDocument();
		await expect(canvas.queryByText("1,200")).not.toBeInTheDocument();
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
