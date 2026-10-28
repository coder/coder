import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { Feature } from "#/api/typesGenerated";
import {
	MockAgentRuntimeHoursFeature,
	MockLicenseResponse,
	mockApiError,
} from "#/testHelpers/entities";
import LicensesSettingsPageView from "./LicensesSettingsPageView";

const meta: Meta<typeof LicensesSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPageView",
	component: LicensesSettingsPageView,
	args: {
		showConfetti: false,
		isLoading: false,
		hasUserLimitEntitlementData: true,
		userLimitActual: 1,
		userLimitLimit: 10,
		licenses: MockLicenseResponse,
		canViewLicenses: true,
		isRemovingLicense: false,
		isRefreshing: false,
		removeLicense: fn(),
		refreshEntitlements: fn(),
		activeUsers: [{ date: "2024-01-01", count: 1 }],
		aiGovernanceUserFeature: {
			enabled: false,
			entitlement: "not_entitled",
		} satisfies Feature,
		agentRuntimeHoursFeature: {
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("No licenses yet");
		const addLinks = canvas.getAllByRole("link", { name: /add a license/i });
		for (const link of addLinks) {
			await expect(link).toHaveAttribute("href", "/deployment/licenses/add");
		}
		await expect(
			canvas.getByRole("link", { name: /start a trial/i }),
		).toHaveAttribute("href", "/deployment/premium");
	},
};

export const NoLicensePermission: Story = {
	args: {
		licenses: [],
		canViewLicenses: false,
	},
};

export const LoadError: Story = {
	args: {
		licenses: undefined,
		licensesError: mockApiError({
			message: "Internal error fetching licenses.",
			detail: "database connection refused",
		}),
		onRetryLicenses: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("No licenses yet")).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: /retry/i }));
		await expect(args.onRetryLicenses).toHaveBeenCalledTimes(1);
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

export const TotalAgentHoursUsage: Story = {
	args: {
		agentRuntimeHoursFeature: {
			...MockAgentRuntimeHoursFeature,
			limit: 2000,
			soft_limit: 1700,
			actual: 435,
			// 435 hours and 48 minutes: renders as 435.8.
			actual_ms: 435 * 3_600_000 + 48 * 60_000,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const agentHoursHeading = canvas.getByRole("heading", {
			name: "Total agent hours",
		});
		const agentHoursCard = within(agentHoursHeading.closest("section")!);
		await expect(agentHoursCard.getByText("435.8")).toBeInTheDocument();
		await expect(agentHoursCard.getByText("2,000")).toBeInTheDocument();
	},
};
