import { chromatic } from "testHelpers/chromatic";
import { MockLicenseResponse } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, fn, within } from "storybook/test";

import { LicenseCard } from "./LicenseCard";

const meta: Meta<typeof LicenseCard> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPage/LicenseCard",
	component: LicenseCard,
	parameters: { chromatic },
	args: {
		license: MockLicenseResponse[0],
		userLimitActual: 4,
		userLimitLimit: 10,
		onRemove: fn(),
		isRemoving: false,
	},
};

export default meta;
type Story = StoryObj<typeof LicenseCard>;

export const Default: Story = {};

export const Premium: Story = {
	args: {
		license: MockLicenseResponse[1],
	},
};

export const PremiumWithAIGovernance: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 750,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/add-ons/i)).toBeInTheDocument();
		await expect(canvas.getByText(/ai governance/i)).toBeInTheDocument();
	},
};

export const Expired: Story = {
	args: {
		license: MockLicenseResponse[3],
	},
};

export const ExceededUserLimit: Story = {
	args: {
		userLimitActual: 15,
		userLimitLimit: 10,
	},
};

export const ExceededAIGovernance: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
			limit: 1000,
		},
	},
};

export const ExpiredAIGovernanceOverageShowsExpired: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				license_expires: dayjs().subtract(1, "day").unix(),
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Expired")).toBeInTheDocument();
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
	},
};

export const NotYetValid: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: dayjs().add(7, "day").unix(),
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Starts on/)).toBeInTheDocument();
	},
};

export const FutureAIGovernanceOverageShowsStartsOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: dayjs().add(7, "day").unix(),
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 1200,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Starts on/)).toBeInTheDocument();
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
	},
};

export const LowerLimitCardUsesMergedEntitlement: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 500,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 750,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
	},
};

export const EnterpriseDoesNotShowAIGovernanceAddOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				feature_set: "enterprise",
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 750,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Add-ons")).not.toBeInTheDocument();
		await expect(canvas.queryByText("AI add-on")).not.toBeInTheDocument();
	},
};
