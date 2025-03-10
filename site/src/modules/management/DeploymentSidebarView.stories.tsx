import type { Meta, StoryObj } from "@storybook/react";
import { MockNoPermissions, MockPermissions } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

const meta: Meta<typeof DeploymentSidebarView> = {
	title: "modules/management/DeploymentSidebarView",
	component: DeploymentSidebarView,
	decorators: [withDashboardProvider],
	parameters: { showOrganizations: true },
	args: {
		permissions: MockPermissions,
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
