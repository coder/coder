import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { chromatic } from "#/testHelpers/chromatic";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockExperiments,
} from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
import { DashboardContext, type DashboardValue } from "../DashboardProvider";
import {
	AIProviderEnvDriftBanner,
	AIProviderEnvDriftBannerView,
} from "./AIProviderEnvDriftBanner";

const meta: Meta<typeof AIProviderEnvDriftBannerView> = {
	title: "modules/dashboard/AIProviderEnvDriftBanner",
	parameters: { chromatic },
	component: AIProviderEnvDriftBannerView,
};

export default meta;
type Story = StoryObj<typeof AIProviderEnvDriftBannerView>;

export const Default: Story = {
	args: {
		docsHref: docs("/ai-coder/ai-gateway/setup"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /View setup docs/i }),
		).toHaveAttribute("href", docs("/ai-coder/ai-gateway/setup"));
	},
};

const renderBannerWithDrift = (driftDetected: boolean) => {
	const value: DashboardValue = {
		entitlements: MockEntitlements,
		experiments: MockExperiments,
		appearance: {
			...MockAppearanceConfig,
			ai_providers_env_drift_detected: driftDetected,
		},
		buildInfo: MockBuildInfo,
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
		canViewOrganizationSettings: false,
	};
	return (
		<DashboardContext.Provider value={value}>
			<AIProviderEnvDriftBanner />
		</DashboardContext.Provider>
	);
};

export const DriftDetected: Story = {
	render: () => renderBannerWithDrift(true),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /View setup docs/i }),
		).toHaveAttribute("href", docs("/ai-coder/ai-gateway/setup"));
	},
};

export const NoDrift: Story = {
	render: () => renderBannerWithDrift(false),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByRole("status")).not.toBeInTheDocument();
	},
};
