import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { Feature } from "#/api/typesGenerated";
import { chromatic } from "#/testHelpers/chromatic";
import { MockLicenseResponse } from "#/testHelpers/entities";
import LicensesSettingsPageView from "./LicensesSettingsPageView";

const meta: Meta<typeof LicensesSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPageView",
	parameters: { chromatic },
	component: LicensesSettingsPageView,
	args: {
		showConfetti: false,
		isLoading: false,
		hasUserLimitEntitlementData: true,
		userLimitActual: 1,
		userLimitLimit: 10,
		licenses: MockLicenseResponse,
		isRemovingLicense: false,
		isRefreshing: false,
		removeLicense: fn(),
		refreshEntitlements: fn(),
		activeUsers: [{ date: "2024-01-01", count: 1 }],
		managedAgentFeature: {
			enabled: false,
			entitlement: "not_entitled",
		} satisfies Feature,
		aiGovernanceUserFeature: {
			enabled: false,
			entitlement: "not_entitled",
		} satisfies Feature,
	},
};

export default meta;
type Story = StoryObj<typeof LicensesSettingsPageView>;

export const Default: Story = {};

export const Empty: Story = {
	args: {
		licenses: [],
	},
};

/** Premium + AI Governance usage bars; AI Governance shows `SeatUsageBarCard` (not the not-entitled placeholder). */
export const ActiveAIGovernanceAddOnUsage: Story = {
	args: {
		userLimitActual: 1923,
		userLimitLimit: 2500,
		activeUsers: [
			{ date: "2024-01-01", count: 100 },
			{ date: "2024-02-01", count: 120 },
		],
		aiGovernanceUserFeature: {
			enabled: true,
			entitlement: "entitled",
			limit: 1000,
			actual: 512,
		} satisfies Feature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Seat usage" }),
		).toBeInTheDocument();
		await expect(canvas.getByText("1,923")).toBeInTheDocument();
		await expect(canvas.getByText("2,500")).toBeInTheDocument();
		await expect(
			canvas.getByRole("heading", { name: "AI Governance add-on usage" }),
		).toBeInTheDocument();
		await expect(canvas.getByText("512")).toBeInTheDocument();
		await expect(canvas.getByText("1,000")).toBeInTheDocument();
	},
};
