import type { Meta, StoryObj } from "@storybook/react-vite";
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
		experiments: [],
		buildInfo: MockBuildInfo,
		activeSection: "general",
	},
};

export default meta;
type Story = StoryObj<typeof DeploymentSidebarView>;

export const GeneralOpen: Story = {};

export const InfrastructureOpen: Story = {
	args: {
		activeSection: "infrastructure",
	},
};

export const AuthenticationOpen: Story = {
	args: {
		activeSection: "authentication",
	},
};

export const AISettingsOpen: Story = {
	args: {
		activeSection: "ai-settings",
	},
};

export const AIGovernanceOpen: Story = {
	args: {
		activeSection: "ai-governance",
	},
};

export const NoViewUsers: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAllUsers: false,
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
