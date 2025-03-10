import type { Meta, StoryObj } from "@storybook/react";
import {
	MockOrganizationAuditorRole,
	MockRoleWithOrgPermissions,
} from "testHelpers/entities";
import { CustomRolesPageView } from "./CustomRolesPageView";

const meta: Meta<typeof CustomRolesPageView> = {
	title: "pages/OrganizationCustomRolesPage",
	component: CustomRolesPageView,
	args: {
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [MockRoleWithOrgPermissions],
		canAssignOrgRole: true,
		canCreateOrgRole: true,
		isCustomRolesEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof CustomRolesPageView>;

export const Enabled: Story = {};

export const NotEnabled: Story = {
	args: {
		isCustomRolesEnabled: false,
	},
};

export const NotEnabledEmptyTable: Story = {
	args: {
		customRoles: [],
		canAssignOrgRole: true,
		isCustomRolesEnabled: false,
	},
};

export const RoleWithoutPermissions: Story = {
	args: {
		builtInRoles: [MockOrganizationAuditorRole],
		customRoles: [MockOrganizationAuditorRole],
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
	},
};

export const EmptyTableUserWithoutPermission: Story = {
	args: {
		customRoles: [],
		canAssignOrgRole: false,
		canCreateOrgRole: false,
	},
};

export const EmptyTableUserWithPermission: Story = {
	args: {
		customRoles: [],
	},
};
