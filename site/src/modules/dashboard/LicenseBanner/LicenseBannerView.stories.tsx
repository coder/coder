import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	type Entitlements,
	LicenseAgentRuntimeHoursAllocationReachedWarningText,
	LicenseAgentRuntimeHoursClaimsIgnoredWarningText,
	LicenseAgentRuntimeHoursSoftLimitWarningText,
	LicenseAgentRuntimeUsageUnavailableErrorText,
	LicenseAIGovernance90PercentWarningText,
	LicenseManagedAgentLimitExceededWarningText,
	LicenseManagedAgentUsageUnavailableErrorText,
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
import { formatLicenseMessage, LicenseBanner } from "./LicenseBanner";
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
		await expect(canvas.getByRole("status")).toBeInTheDocument();
		await expect(
			canvas.getByText("Your license limits have been exceeded"),
		).toBeInTheDocument();
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

const renderLicenseBanner = ({
	errors = [],
	warnings = [],
	features = {},
}: {
	errors?: string[];
	warnings?: string[];
	features?: Partial<Entitlements["features"]>;
}) => {
	const mockDashboardValue: DashboardValue = {
		entitlements: {
			...MockEntitlements,
			has_license: true,
			errors,
			warnings,
			features: {
				...MockEntitlements.features,
				...features,
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
		<DashboardContext value={mockDashboardValue}>
			<LicenseBanner />
		</DashboardContext>
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
}) =>
	renderLicenseBanner({
		warnings,
		features: {
			ai_governance_user_limit: {
				enabled: true,
				entitlement,
				actual,
				...(limit !== undefined ? { limit } : {}),
			},
		},
	});

export const AIGovernanceNearLimit: Story = {
	render: () =>
		renderLicenseBannerWithAIGovernance({
			actual: 95,
			limit: 100,
			warnings: [LicenseAIGovernance90PercentWarningText],
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByRole("status");
		await expect(banner).toHaveTextContent(
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
		renderLicenseBanner({
			warnings: [
				formatLicenseMessage(
					LicenseAgentRuntimeHoursSoftLimitWarningText,
					90,
					100,
					80,
				),
			],
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByRole("status");
		await expect(banner).toHaveTextContent(
			"Your deployment is approaching its Coder Agent runtime hours allocation: 90 of the 100 hours included in the current license term are used, at or above the advisory soft limit of 80 hours.",
		);
		// The operator is inside their allocation with nothing owed, so no
		// sales call-to-action is rendered. The advisory renders in the
		// muted variant, which the visual snapshot covers.
		await expect(
			canvas.queryByRole("link", { name: /Contact sales@coder\.com/i }),
		).not.toBeInTheDocument();
	},
};

export const AgentRuntimeHoursAllocationReached: Story = {
	render: () =>
		renderLicenseBanner({
			warnings: [
				formatLicenseMessage(
					LicenseAgentRuntimeHoursAllocationReachedWarningText,
					100,
					100,
				),
			],
		}),
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

// Each diagnostic pins role=status (not alert) and a suppressed sales
// link. The "unavailable" message arrives on the errors channel; see the
// LicenseManagedAgentUsageUnavailableErrorText doc for why. Background
// mutedness is covered by the visual snapshot.
const playMutedDiagnostic =
	(message: string): Story["play"] =>
	async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByRole("status");
		await expect(banner).toHaveTextContent(message);
		await expect(
			canvas.queryByRole("link", { name: /Contact sales@coder\.com/i }),
		).not.toBeInTheDocument();
	};

export const AgentRuntimeUsageUnavailable: Story = {
	render: () =>
		renderLicenseBanner({
			errors: [LicenseAgentRuntimeUsageUnavailableErrorText],
		}),
	play: playMutedDiagnostic(LicenseAgentRuntimeUsageUnavailableErrorText),
};

export const ManagedAgentUsageUnavailable: Story = {
	render: () =>
		renderLicenseBanner({
			errors: [LicenseManagedAgentUsageUnavailableErrorText],
		}),
	play: playMutedDiagnostic(LicenseManagedAgentUsageUnavailableErrorText),
};

export const AgentRuntimeHoursClaimsIgnored: Story = {
	render: () =>
		renderLicenseBanner({
			warnings: [LicenseAgentRuntimeHoursClaimsIgnoredWarningText],
		}),
	play: playMutedDiagnostic(LicenseAgentRuntimeHoursClaimsIgnoredWarningText),
};

// An all-diagnostic banner (e.g. one database blip failing both usage
// queries) must not claim license limits were exceeded.
export const UsageDiagnosticsOnlyHeading: Story = {
	render: () =>
		renderLicenseBanner({
			errors: [
				LicenseManagedAgentUsageUnavailableErrorText,
				LicenseAgentRuntimeUsageUnavailableErrorText,
			],
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toBeInTheDocument();
		await expect(canvas.getByText("License notices")).toBeInTheDocument();
		await expect(
			canvas.queryByText("Your license limits have been exceeded"),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByText("License errors require attention"),
		).not.toBeInTheDocument();
	},
};
