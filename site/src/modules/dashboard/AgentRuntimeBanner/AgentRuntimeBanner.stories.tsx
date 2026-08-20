import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Feature } from "#/api/typesGenerated";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockExperiments,
} from "#/testHelpers/entities";
import { DashboardContext, type DashboardValue } from "../DashboardProvider";
import { AgentRuntimeBanner } from "./AgentRuntimeBanner";

const renderAgentRuntimeBanner = (agentRuntimeHours: Feature) => {
	const mockDashboardValue: DashboardValue = {
		entitlements: {
			...MockEntitlements,
			has_license: true,
			features: {
				...MockEntitlements.features,
				agent_runtime_hours: agentRuntimeHours,
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
			<AgentRuntimeBanner />
		</DashboardContext>
	);
};

const expectNoBanner = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
};

const meta: Meta<typeof AgentRuntimeBanner> = {
	title: "modules/dashboard/AgentRuntimeBanner",
	component: AgentRuntimeBanner,
};

export default meta;
type Story = StoryObj<typeof AgentRuntimeBanner>;

export const BelowAllocation: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: true,
			entitlement: "entitled",
			actual: 99,
			limit: 100,
		}),
	play: async ({ canvasElement }) => {
		await expectNoBanner(canvasElement);
	},
};

export const AtAllocation: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: true,
			entitlement: "entitled",
			actual: 100,
			limit: 100,
			soft_limit: 80,
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// role="alert" is rendered only for the error (red) variant.
		const banner = canvas.getByRole("alert");
		await expect(banner).toHaveTextContent(
			"Your deployment has used 100 of the 100 Coder Agent runtime hours included in the current license term. Contact your deployment administrator.",
		);
		// No dismiss affordance and no sales link.
		await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
		await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
	},
};

export const AtHardLimit: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: true,
			entitlement: "entitled",
			actual: 130,
			limit: 100,
			hard_limit: 120,
		}),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByRole("alert");
		await expect(banner).toHaveTextContent(
			"Your deployment has used 130 of the 100 Coder Agent runtime hours included in the current license term, reaching the hard limit of 120 hours. Contact your deployment administrator.",
		);
		await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
		await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
	},
};

// A zero allocation means the feature is disabled, not exhausted.
export const ZeroAllocation: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: false,
			entitlement: "entitled",
			actual: 50,
			limit: 0,
		}),
	play: async ({ canvasElement }) => {
		await expectNoBanner(canvasElement);
	},
};

// actual is absent when the usage measurement fails.
export const MeasurementUnavailable: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: true,
			entitlement: "entitled",
			limit: 100,
		}),
	play: async ({ canvasElement }) => {
		await expectNoBanner(canvasElement);
	},
};

export const NotEntitled: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: false,
			entitlement: "not_entitled",
			actual: 100,
			limit: 100,
		}),
	play: async ({ canvasElement }) => {
		await expectNoBanner(canvasElement);
	},
};

// A new license that raises the allocation above usage clears the banner.
export const ClearedAfterAllocationRaised: Story = {
	render: () =>
		renderAgentRuntimeBanner({
			enabled: true,
			entitlement: "entitled",
			actual: 100,
			limit: 200,
		}),
	play: async ({ canvasElement }) => {
		await expectNoBanner(canvasElement);
	},
};
