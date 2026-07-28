import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, userEvent, within } from "storybook/test";
import type { AssignableRoles } from "#/api/typesGenerated";
import {
	MockOrganization,
	MockOrganizationAuditorRole,
	MockRoleWithOrgPermissions,
} from "#/testHelpers/entities";
import { CustomRolesPageView } from "./CustomRolesPageView";

const mockOrgRoles: AssignableRoles[] = [
	{
		name: "organization-workspace-access",
		display_name: "Organization Workspace Access",
		organization_id: MockOrganization.id,
		site_permissions: [],
		organization_permissions: [],
		organization_member_permissions: [],
		user_permissions: [],
		assignable: true,
		built_in: true,
	},
	{
		name: "organization-admin",
		display_name: "Organization Admin",
		organization_id: MockOrganization.id,
		site_permissions: [],
		organization_permissions: [],
		organization_member_permissions: [],
		user_permissions: [],
		assignable: true,
		built_in: true,
	},
	{
		name: "agents-access",
		display_name: "Agents Access",
		organization_id: MockOrganization.id,
		site_permissions: [],
		organization_permissions: [],
		organization_member_permissions: [],
		user_permissions: [],
		assignable: true,
		built_in: true,
	},
];

const meta: Meta<typeof CustomRolesPageView> = {
	title: "pages/OrganizationCustomRolesPage",
	component: CustomRolesPageView,
	args: {
		organization: MockOrganization,
		builtInRoles: [MockRoleWithOrgPermissions],
		customRoles: [MockRoleWithOrgPermissions],
		canCreateOrgRole: true,
		canEditDefaultRoles: true,
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
		canCreateOrgRole: true,
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
		canCreateOrgRole: false,
	},
};

export const EmptyTableUserWithPermission: Story = {
	args: {
		customRoles: [],
	},
};

export const DefaultRolesHidden: Story = {
	args: {
		defaultRolesEnabled: false,
		availableOrgRoles: mockOrgRoles,
		onUpdateDefaultRoles: async () => {
			action("onUpdateDefaultRoles")();
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(body.queryByText("Default Roles")).toBeNull();
	},
};

export const DefaultRolesEnabled: Story = {
	args: {
		defaultRolesEnabled: true,
		defaultRolesEntitled: true,
		availableOrgRoles: mockOrgRoles,
		onUpdateDefaultRoles: async () => {
			action("onUpdateDefaultRoles")();
		},
	},
};

export const DefaultRolesNotEntitled: Story = {
	args: {
		defaultRolesEnabled: true,
		defaultRolesEntitled: false,
		availableOrgRoles: mockOrgRoles,
		onUpdateDefaultRoles: async () => {
			action("onUpdateDefaultRoles")();
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const editButton = await body.findByRole("button", {
			name: /edit default roles/i,
		});
		expect(editButton).toBeDisabled();
		await body.findByText(/requires a Premium license/i);
	},
};

export const DefaultRolesEmpty: Story = {
	args: {
		organization: {
			...MockOrganization,
			default_org_member_roles: [],
		},
		defaultRolesEnabled: true,
		defaultRolesEntitled: true,
		availableOrgRoles: mockOrgRoles,
		onUpdateDefaultRoles: async () => {
			action("onUpdateDefaultRoles")();
		},
	},
};

export const DefaultRolesHiddenWithoutEditPermission: Story = {
	args: {
		defaultRolesEnabled: true,
		defaultRolesEntitled: true,
		canEditDefaultRoles: false,
		availableOrgRoles: mockOrgRoles,
		onUpdateDefaultRoles: async () => {
			action("onUpdateDefaultRoles")();
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(body.queryByText("Default Roles")).toBeNull();
	},
};

export const DefaultRolesEditDialog: Story = {
	args: {
		defaultRolesEnabled: true,
		defaultRolesEntitled: true,
		availableOrgRoles: mockOrgRoles,
		onUpdateDefaultRoles: async () => {
			action("onUpdateDefaultRoles")();
		},
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const editButton = await body.findByRole("button", {
			name: /edit default roles/i,
		});
		await user.click(editButton);
		await body.findByRole("heading", { name: /edit default roles/i });
	},
};
