import type { Meta, StoryObj } from "@storybook/react";
import {
	MockOrganization,
	MockOrganization2,
	MockPermissions,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { SidebarView } from "./SidebarView";

const meta: Meta<typeof SidebarView> = {
	title: "modules/management/SidebarView",
	component: SidebarView,
	decorators: [withDashboardProvider],
	parameters: { showOrganizations: true },
	args: {
		activeSettings: true,
		activeOrganizationName: undefined,
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
				},
			},
			{
				...MockOrganization2,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
				},
			},
		],
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof SidebarView>;

export const LoadingOrganizations: Story = {
	args: {
		organizations: undefined,
	},
};

export const NoCreateOrg: Story = {
	args: {
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
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
			viewDeploymentValues: false,
			editDeploymentValues: false,
		},
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: {},
	},
};

export const NoSelected: Story = {
	args: {
		activeSettings: false,
	},
};

export const SelectedOrgNoMatch: Story = {
	args: {
		activeOrganizationName: MockOrganization.name,
		organizations: [],
	},
};

export const SelectedOrgAdmin: Story = {
	args: {
		activeOrganizationName: MockOrganization.name,
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
					assignOrgRole: true,
				},
			},
		],
	},
};

export const SelectedOrgAuditor: Story = {
	args: {
		activeOrganizationName: MockOrganization.name,
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: false,
					editMembers: false,
					editGroups: false,
					auditOrganization: true,
				},
			},
		],
	},
};

export const SelectedOrgUserAdmin: Story = {
	args: {
		activeOrganizationName: MockOrganization.name,
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: false,
					editMembers: true,
					editGroups: true,
					auditOrganization: false,
				},
			},
		],
	},
};

export const MultiOrgAdminAndUserAdmin: Story = {
	args: {
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: false,
					editMembers: false,
					editGroups: false,
					auditOrganization: true,
				},
			},
			{
				...MockOrganization2,
				permissions: {
					editOrganization: false,
					editMembers: true,
					editGroups: true,
					auditOrganization: false,
				},
			},
		],
	},
};

export const SelectedMultiOrgAdminAndUserAdmin: Story = {
	args: {
		activeOrganizationName: MockOrganization2.name,
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: false,
					editMembers: false,
					editGroups: false,
					auditOrganization: true,
				},
			},
			{
				...MockOrganization2,
				permissions: {
					editOrganization: false,
					editMembers: true,
					editGroups: true,
					auditOrganization: false,
				},
			},
		],
	},
};

export const OrgsDisabled: Story = {
	parameters: {
		showOrganizations: false,
	},
};
