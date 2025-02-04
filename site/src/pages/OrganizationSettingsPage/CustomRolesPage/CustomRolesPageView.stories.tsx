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
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [MockRoleWithOrgPermissions],
		canAssignOrgRole: true,
		isCustomRolesEnabled: false,
	},
};

export const NotEnabledEmptyTable: Story = {
	args: {
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [],
		canAssignOrgRole: true,
		isCustomRolesEnabled: false,
	},
};

export const Enabled: Story = {
	args: {
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [MockRoleWithOrgPermissions],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export const RoleWithoutPermissions: Story = {
	args: {
		builtInRoles: [MockOrganizationAuditorRole],
		customRoles: [MockOrganizationAuditorRole],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export const EmptyDisplayName: Story = {
	args: {
		customRoles: [
			{
				...MockRoleWithOrgPermissions,
				name: "my-custom-role",
				display_name: "",
			},
		],
		builtInRoles: [MockRoleWithOrgPermissions],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export const EmptyTableUserWithoutPermission: Story = {
	args: {
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [],
		canAssignOrgRole: false,
		isCustomRolesEnabled: true,
	},
};

export const EmptyTableUserWithPermission: Story = {
	args: {
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};
