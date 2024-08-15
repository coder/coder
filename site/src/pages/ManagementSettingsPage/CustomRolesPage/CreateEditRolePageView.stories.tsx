import type { Meta, StoryObj } from "@storybook/react";
import {
	MockRoleWithOrgPermissions,
	assignableRole,
	mockApiError,
} from "testHelpers/entities";
import { CreateEditRolePageView } from "./CreateEditRolePageView";

const meta: Meta<typeof CreateEditRolePageView> = {
	title: "pages/OrganizationCreateEditRolePage",
	component: CreateEditRolePageView,
};

export default meta;
type Story = StoryObj<typeof CreateEditRolePageView>;

export const Default: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: undefined,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
	},
};

export const WithError: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: mockApiError({
			message: "A role named new-role already exists.",
			validations: [{ field: "name", detail: "Role names must be unique" }],
		}),
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
	},
};

export const CannotEdit: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: undefined,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: false,
	},
};

export const ShowAllResources: Story = {
	args: {
		role: assignableRole(MockRoleWithOrgPermissions, true),
		onSubmit: () => null,
		error: undefined,
		isLoading: false,
		organizationName: "my-org",
		canAssignOrgRole: true,
		allResources: true,
	},
};
