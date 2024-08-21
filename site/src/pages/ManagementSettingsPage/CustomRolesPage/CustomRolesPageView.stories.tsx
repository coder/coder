import type { Meta, StoryObj } from "@storybook/react";
import { MockRoleWithOrgPermissions } from "testHelpers/entities";
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

export const Enabled: Story = {
	args: {
		roles: [MockRoleWithOrgPermissions],
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

export const EmptyRoleWithoutPermission: Story = {
	args: {
		roles: [],
		canAssignOrgRole: false,
		isCustomRolesEnabled: true,
	},
};

export const EmptyRoleWithPermission: Story = {
	args: {
		roles: [],
		canAssignOrgRole: true,
		isCustomRolesEnabled: true,
	},
};
