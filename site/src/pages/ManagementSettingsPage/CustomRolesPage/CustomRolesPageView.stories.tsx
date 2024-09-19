import type { Meta, StoryObj } from "@storybook/react";
import {
	MockOrganizationAuditorRole,
	MockRoleWithOrgPermissions,
} from "testHelpers/entities";
import { CustomRolesPageView } from "./CustomRolesPageView";

const meta: Meta<typeof CustomRolesPageView> = {
	title: "pages/OrganizationCustomRolesPage",
	component: CustomRolesPageView,
};

export default meta;
type Story = StoryObj<typeof CustomRolesPageView>;

export const NotEnabled: Story = {
	args: {
		roles: [MockRoleWithOrgPermissions],
		canAssignOrgRole: true,
		isCustomRolesEnabled: false,
	},
};

export const NotEnabledEmptyTable: Story = {
	args: {
		roles: [],
		canAssignOrgRole: true,
		isCustomRolesEnabled: false,
	},
};

export const Enabled: Story = {
	args: {
		roles: [MockRoleWithOrgPermissions],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export const RoleWithoutPermissions: Story = {
	args: {
		roles: [MockOrganizationAuditorRole],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export const EmptyDisplayName: Story = {
	args: {
		roles: [
			{
				...MockRoleWithOrgPermissions,
				name: "my-custom-role",
				display_name: "",
			},
		],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export const EmptyTableUserWithoutPermission: Story = {
	args: {
		roles: [],
		canAssignOrgRole: false,
		isCustomRolesEnabled: true,
	},
};

export const EmptyTableUserWithPermission: Story = {
	args: {
		roles: [],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};
