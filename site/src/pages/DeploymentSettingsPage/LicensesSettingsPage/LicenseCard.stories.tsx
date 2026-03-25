import { chromatic } from "testHelpers/chromatic";
import { MockLicenseResponse } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, fn, within } from "storybook/test";

import { LicenseCard } from "./LicenseCard";

const FIXED_NOW = dayjs().startOf("day");
const YESTERDAY = FIXED_NOW.subtract(1, "day").unix();
const NEXT_WEEK = FIXED_NOW.add(7, "day").unix();

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

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("#1")).toBeInTheDocument();
		await expect(canvas.getByText("4 / 10")).toBeInTheDocument();
		await expect(canvas.getByText("Enterprise")).toBeInTheDocument();
	},
};

export const UnlimitedUsers: Story = {
	args: {
		userLimitLimit: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("4 / Unlimited")).toBeInTheDocument();
	},
};

export const UsesLicenseUserLimit: Story = {
	args: {
		license: {
			...MockLicenseResponse[0],
			claims: {
				...MockLicenseResponse[0].claims,
				features: {
					...MockLicenseResponse[0].claims.features,
					user_limit: 3,
				},
			},
		},
		userLimitActual: 1,
		userLimitLimit: 100,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1 / 3")).toBeInTheDocument();
	},
};

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
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/add-ons/i)).toBeInTheDocument();
		await expect(canvas.getByText(/ai governance/i)).toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("750 / 1,000");
	},
};

export const PremiumWithoutAIGovernanceAddOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			actual: 100,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Add-ons")).not.toBeInTheDocument();
		await expect(canvas.queryByText("AI governance")).not.toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Add-on exceeded")).toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("1,200 / 1,000");
	},
};

export const ExpiredAIGovernanceOverageShowsExpired: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				license_expires: YESTERDAY,
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Expired")).toBeInTheDocument();
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 1,000");
	},
};

export const ExpiredAIGovernanceInGracePeriodShowsExceeded: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				license_expires: YESTERDAY,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		},
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "grace_period",
			actual: 1200,
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Add-on exceeded")).toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("1,200 / 1,000");
	},
};

export const NotYetValid: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: NEXT_WEEK,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Not started/)).toBeInTheDocument();
	},
};

export const FutureAIGovernanceOverageShowsStartsOn: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: NEXT_WEEK,
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Not started/)).toBeInTheDocument();
		await expect(canvas.queryByText("Add-on exceeded")).not.toBeInTheDocument();
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 1,000");
	},
};

export const FutureAIGovernanceUsageShowsNoCurrentSeats: Story = {
	args: {
		license: {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				nbf: NEXT_WEEK,
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
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 1,000");
		await expect(seatsValue).not.toHaveTextContent("0 / 1,000");
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
		const seatsLabel = canvas.getByText("Seats");
		const seatsValue = seatsLabel.nextElementSibling;
		await expect(seatsValue).toHaveTextContent("—");
		await expect(seatsValue).toHaveTextContent("/ 500");
		await expect(seatsValue).not.toHaveTextContent("750 / 500");
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
			limit: 1000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Add-ons")).not.toBeInTheDocument();
		await expect(canvas.queryByText("AI add-on")).not.toBeInTheDocument();
	},
};
