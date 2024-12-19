import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
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
		activeOrganization: undefined,
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
		activeOrganization: {
			...MockOrganization,
			permissions: { createOrganization: false },
		},
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "My Organization" }),
		);
	},
};

export const NoPermissions: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: MockNoPermissions,
		},
		permissions: MockNoPermissions,
	},
};

export const AllPermissions: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: true,
				editMembers: true,
				editGroups: true,
				auditOrganization: true,
				assignOrgRole: true,
				viewProvisioners: true,
				viewIdpSyncSettings: true,
			},
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
					assignOrgRole: true,
					viewProvisioners: true,
					viewIdpSyncSettings: true,
				},
			},
		],
	},
};

export const SelectedOrgAdmin: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: true,
				editMembers: true,
				editGroups: true,
				auditOrganization: true,
				assignOrgRole: true,
			},
		},
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
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: false,
				editMembers: false,
				editGroups: false,
				auditOrganization: true,
			},
		},
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
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: false,
				editMembers: true,
				editGroups: true,
				auditOrganization: false,
			},
		},
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

export const OrgsDisabled: Story = {
	parameters: {
		showOrganizations: false,
	},
};
