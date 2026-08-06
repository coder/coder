import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	LicenseAgentRuntimeHoursAllocationReachedWarningText,
	LicenseAgentRuntimeHoursSoftLimitWarningText,
	LicenseAgentRuntimeUsageUnavailableWarningText,
	LicenseAIGovernance90PercentWarningText,
	LicenseManagedAgentLimitExceededWarningText,
	LicenseTelemetryRequiredErrorText,
} from "#/api/typesGenerated";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockExperiments,
} from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
import { DashboardContext, type DashboardValue } from "../DashboardProvider";
import { LicenseBanner } from "./LicenseBanner";
import { LicenseBannerView } from "./LicenseBannerView";

const meta: Meta<typeof LicenseBannerView> = {
	title: "components/LicenseBannerView",
	component: LicenseBannerView,
};

export default meta;
type Story = StoryObj<typeof LicenseBannerView>;

export const OneWarning: Story = {
	args: {
		messages: [
			{
				message: "You have exceeded the number of seats in your license.",
				variant: "warningProminent",
				link: {
					href: "mailto:sales@coder.com",
					label: "Contact sales@coder.com.",
					showExternalIcon: false,
				},
			},
		],
	},
};

export const TwoWarnings: Story = {
	args: {
		messages: [
			{
				message: "You have exceeded the number of seats in your license.",
				variant: "warningProminent",
			},
			{
				message: "You are flying too close to the sun.",
				variant: "warningProminent",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", { name: "Show more" }),
		).not.toBeInTheDocument();
	},
};

export const ThreeWarnings: Story = {
	args: {
		messages: [
			{
				message: "You have exceeded the number of seats in your license.",
				variant: "warningProminent",
			},
			{
				message: "You are flying too close to the sun.",
				variant: "warningProminent",
			},
			{
				message: "Another warning that should be hidden until expanded.",
				variant: "warningProminent",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "Show more" }),
		).toBeInTheDocument();
	},
};

export const OneError: Story = {
	args: {
		messages: [
			{
				message:
					"You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.",
				variant: "error",
			},
		],
	},
};

export const TwoErrors: Story = {
	args: {
		messages: [
			{
				message:
					"You have multiple replicas but high availability is an Enterprise feature.",
				variant: "error",
			},
			{
				message: "Telemetry is required for this deployment.",
				variant: "error",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("License errors require attention"),
		).toBeInTheDocument();
	},
};

export const TelemetryRequiredError: Story = {
	args: {
		messages: [
			{
				message: LicenseTelemetryRequiredErrorText,
				variant: "error",
				link: {
					href: "mailto:sales@coder.com",
					label: "Contact sales@coder.com if you need an exception.",
					showExternalIcon: false,
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			LicenseTelemetryRequiredErrorText,
		);
		await expect(
			canvas.getByRole("link", {
				name: /Contact sales@coder\.com if you need an exception\./i,
			}),
		).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

export const ManagedAgentLimitExceeded: Story = {
	args: {
		messages: [
			{
				message: LicenseManagedAgentLimitExceededWarningText,
				variant: "warningProminent",
				link: {
					href: docs("/ai-coder/ai-governance"),
					label: "View AI Governance",
					showExternalIcon: true,
					target: "_blank",
				},
			},
		],
	},
};

export const ManagedAgentLimitExceededWithOtherWarnings: Story = {
	args: {
		messages: [
			{
				message: LicenseManagedAgentLimitExceededWarningText,
				variant: "warningProminent",
			},
			{
				message: "You have exceeded the number of seats in your license.",
				variant: "warningProminent",
			},
		],
	},
};

// Renders a warning template the way the backend does, substituting each
// %d placeholder in order.
const formatWarning = (template: string, ...values: number[]): string =>
	values.reduce<string>(
		(message, value) => message.replace("%d", `${value}`),
		template,
	);

const renderLicenseBannerWithWarnings = (warnings: string[]) => {
	const mockDashboardValue: DashboardValue = {
		entitlements: {
			...MockEntitlements,
			has_license: true,
			warnings,
		},
		experiments: MockExperiments,
		appearance: MockAppearanceConfig,
		buildInfo: MockBuildInfo,
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
		canViewOrganizationSettings: false,
	};

	return (
		<DashboardContext.Provider value={mockDashboardValue}>
			<LicenseBanner />
		</DashboardContext.Provider>
	);
};

const renderLicenseBannerWithAIGovernance = ({
	actual,
	entitlement = "entitled",
	limit,
	warnings = [],
}: {
	actual: number;
	entitlement?: "entitled" | "grace_period" | "not_entitled";
	limit?: number;
	warnings?: string[];
}) => {
	const mockDashboardValue: DashboardValue = {
		entitlements: {
			...MockEntitlements,
			has_license: true,
			warnings,
			features: {
				...MockEntitlements.features,
				ai_governance_user_limit: {
					enabled: true,
					entitlement,
					actual,
					...(limit !== undefined ? { limit } : {}),
				},
			},
		},
		experiments: MockExperiments,
		appearance: MockAppearanceConfig,
		buildInfo: MockBuildInfo,
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
		canViewOrganizationSettings: false,
	};

	return (
		<DashboardContext.Provider value={mockDashboardValue}>
			<LicenseBanner />
		</DashboardContext.Provider>
	);
};

export const AIGovernanceNearLimit: Story = {
	render: () =>
		renderLicenseBannerWithAIGovernance({
			actual: 95,
			limit: 100,
			warnings: [LicenseAIGovernance90PercentWarningText],
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toHaveTextContent(
			"You have used 95% of your AI Governance add-on seats.",
		);
		await expect(
			canvas.getByRole("link", { name: /Contact sales@coder\.com/i }),
		).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

export const AIGovernanceOverLimitFromFeature: Story = {
	render: () =>
		renderLicenseBannerWithAIGovernance({
			actual: 110,
			limit: 100,
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toHaveTextContent(
			/110 of 100 AI Governance add-on seats \(10 over the limit\)/,
		);
	},
};

export const AIGovernanceOverLimitGracePeriod: Story = {
	render: () =>
		renderLicenseBannerWithAIGovernance({
			actual: 110,
			entitlement: "grace_period",
			limit: 100,
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toHaveTextContent(
			/110 of 100 AI Governance add-on seats \(10 over the limit\)/,
		);
	},
};

export const AgentRuntimeHoursSoftLimit: Story = {
	render: () =>
		renderLicenseBannerWithWarnings([
			formatWarning(LicenseAgentRuntimeHoursSoftLimitWarningText, 90, 100, 80),
		]),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The advisory soft-limit warning renders in the muted variant,
		// unlike the allocation-reached warning below.
		const banner = canvas.getByRole("status");
		await expect(banner).toHaveTextContent(
			"Your deployment is approaching its Coder Agent runtime hours allocation: 90 of the 100 hours included in the current license term are used, reaching the advisory soft limit of 80 hours.",
		);
		await expect(
			canvas.getByRole("link", { name: /Contact sales@coder\.com/i }),
		).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

export const AgentRuntimeHoursAllocationReached: Story = {
	render: () =>
		renderLicenseBannerWithWarnings([
			formatWarning(
				LicenseAgentRuntimeHoursAllocationReachedWarningText,
				100,
				100,
			),
		]),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByRole("status");
		await expect(banner).toHaveTextContent(
			"Your deployment has used 100 of the 100 Coder Agent runtime hours included in the current license term.",
		);
		await expect(
			canvas.getByRole("link", { name: /Contact sales@coder\.com/i }),
		).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

export const AgentRuntimeUsageUnavailable: Story = {
	render: () =>
		renderLicenseBannerWithWarnings([
			LicenseAgentRuntimeUsageUnavailableWarningText,
		]),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A usage-measurement diagnostic points at the coderd logs, so no
		// sales link is rendered under it.
		await expect(canvas.getByRole("status")).toHaveTextContent(
			LicenseAgentRuntimeUsageUnavailableWarningText,
		);
		await expect(
			canvas.queryByRole("link", { name: /Contact sales@coder\.com/i }),
		).not.toBeInTheDocument();
	},
};
