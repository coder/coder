import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { buildInfoKey } from "#/api/queries/buildInfo";
import { deploymentStatsQueryKey } from "#/api/queries/deployment";
import { updateCheckQueryKey } from "#/api/queries/updateCheck";
import {
	LicenseAgentRuntimeHoursAllocationReachedWarningText,
	type UpdateCheckResponse,
} from "#/api/typesGenerated";
import { formatLicenseMessage } from "#/modules/dashboard/LicenseBanner/LicenseBanner";
import {
	MockBuildInfo,
	MockDeploymentStats,
	MockNoPermissions,
	MockPermissions,
	MockUpdateCheck,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithTablet } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import { DashboardFullPage, DashboardLayout } from "./DashboardLayout";

const outdatedUpdateCheck: UpdateCheckResponse = {
	...MockUpdateCheck,
	current: false,
	version: "v0.12.9",
	url: "https://github.com/coder/coder/releases/tag/v0.12.9",
};

const pageContent = (
	<DashboardFullPage className="p-6">
		<h1>Workspaces</h1>
		<p>Page content rendered in the dashboard outlet.</p>
	</DashboardFullPage>
);

const meta: Meta<typeof DashboardLayout> = {
	title: "modules/dashboard/DashboardLayout",
	component: DashboardLayout,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		pixel: { matrix: pixelWithTablet },
		user: MockUserOwner,
		permissions: MockPermissions,
		reactRouter: reactRouterParameters({
			location: { path: "/" },
			routing: reactRouterOutlet({ path: "/" }, pageContent),
		}),
		queries: [
			{ key: buildInfoKey, data: MockBuildInfo },
			{ key: updateCheckQueryKey, data: MockUpdateCheck },
			{ key: deploymentStatsQueryKey, data: MockDeploymentStats },
		],
	},
};

export default meta;
type Story = StoryObj<typeof DashboardLayout>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Workspaces" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Skip to main content" }),
		).toBeInTheDocument();
	},
};

export const ForMember: Story = {
	parameters: {
		user: MockUserMember,
		permissions: MockNoPermissions,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Workspaces" }),
		).toBeVisible();
		await expect(
			screen.queryByTestId("update-check-notice"),
		).not.toBeInTheDocument();
	},
};

export const UpdateAvailable: Story = {
	parameters: {
		queries: [
			{ key: buildInfoKey, data: MockBuildInfo },
			{ key: updateCheckQueryKey, data: outdatedUpdateCheck },
			{ key: deploymentStatsQueryKey, data: MockDeploymentStats },
		],
	},
	beforeEach: () => {
		localStorage.removeItem("dismissedVersion");
	},
	play: async () => {
		const notice = await screen.findByTestId("update-check-notice");
		await expect(notice).toBeVisible();
		await expect(
			screen.getByText(/Coder v0\.12\.9 is now available/),
		).toBeVisible();
	},
};

export const RuntimeHoursBannerForMember: Story = {
	parameters: {
		user: MockUserMember,
		permissions: MockNoPermissions,
		features: [{ name: "agent_runtime_hours", actual: 100, limit: 100 }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = await canvas.findByRole("alert");
		await expect(banner).toHaveTextContent(
			/used 100 of the 100 Coder Agent runtime hours .* Contact your deployment administrator/,
		);
	},
};

// Admins keep the richer LicenseBanner warning for the same threshold, and
// the member banner stays unmounted so the message never doubles up.
export const RuntimeHoursWarningForAdmin: Story = {
	parameters: {
		features: [{ name: "agent_runtime_hours", actual: 100, limit: 100 }],
		entitlements: {
			warnings: [
				formatLicenseMessage(
					LicenseAgentRuntimeHoursAllocationReachedWarningText,
					100,
					100,
				),
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = await canvas.findByRole("status");
		await expect(banner).toHaveTextContent(
			/used 100 of the 100 Coder Agent runtime hours/,
		);
		await expect(
			canvas.queryByText(/Contact your deployment administrator/),
		).not.toBeInTheDocument();
	},
};

export const SkipToMainContent: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const skipLink = canvas.getByRole("link", {
			name: "Skip to main content",
		});
		const main = canvas.getByRole("main");

		await userEvent.click(skipLink);
		await expect(main).toHaveFocus();
	},
};
