import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	MockBuildInfo,
	MockNoPermissions,
	MockPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

const meta: Meta<typeof DeploymentSidebarView> = {
	title: "modules/management/DeploymentSidebarView",
	component: DeploymentSidebarView,
	decorators: [withDashboardProvider],
	parameters: { showOrganizations: true },
	args: {
		permissions: MockPermissions,
		hidePremiumTab: false,
		buildInfo: MockBuildInfo,
	},
};

export default meta;
type Story = StoryObj<typeof DeploymentSidebarView>;

export const NoViewUsers: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAllUsers: false,
		},
	},
};

export const NoAuditLog: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAnyAuditLog: false,
		},
	},
};

export const NoLicenses: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAllLicenses: false,
		},
	},
};

export const NoDeploymentValues: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewDeploymentConfig: false,
			editDeploymentConfig: false,
		},
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: MockNoPermissions,
	},
};

export const PremiumTabVisible: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("link", { name: "Trial Upgrade" }),
		).toHaveAttribute("href", "/deployment/premium");
	},
};

// A licensed, non-trialing deployment has nothing to upsell.
export const PremiumTabHidden: Story = {
	args: {
		hidePremiumTab: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.queryByRole("link", { name: "Trial Upgrade" }),
		).not.toBeInTheDocument();
		// A neighbouring item must survive the change.
		await expect(
			canvas.getByRole("link", { name: "Licenses" }),
		).toBeInTheDocument();
	},
};

export const OAuth2ProviderEnabled: Story = {};

// The OAuth2 item follows the deployment flag, not the build type, so a
// development build with the flag off still hides it.
export const OAuth2ProviderDisabled: Story = {
	args: {
		buildInfo: {
			...MockBuildInfo,
			version: "v2.99.99-devel+abcdef",
			oauth2_provider: false,
		},
	},
};
