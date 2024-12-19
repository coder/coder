import type { Meta, StoryObj } from "@storybook/react";
import {
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockPermissions,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { OrganizationSidebarView } from "./OrganizationSidebarView";

const meta: Meta<typeof OrganizationSidebarView> = {
	title: "modules/management/OrganizationSidebarView",
	component: OrganizationSidebarView,
	decorators: [withDashboardProvider],
	parameters: { showOrganizations: true },
	args: {
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
type Story = StoryObj<typeof OrganizationSidebarView>;

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

export const NoPermissions: Story = {
	args: {
		permissions: MockNoPermissions,
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
